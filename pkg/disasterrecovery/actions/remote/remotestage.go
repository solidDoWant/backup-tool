package remote

import (
	"fmt"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	bti "github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

type RemoteAction interface {
	Validate(ctx *contexts.Context) error
	Setup(ctx *contexts.Context, btiOpts *bti.CreateBackupToolInstanceOptions) error
	Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) error
}

type CleanupAction interface {
	RemoteAction
	Cleanup(ctx *contexts.Context) error
}

// PreConsistencyPointAction is an optional capability of a RemoteAction that has work to do before the
// stage establishes the event's shared consistency point — the single instant the whole DR event is made
// recoverable to. The canonical example is a database capture taking its base backup, which must complete
// before the non-database captures so the database's recoverable state precedes them. The stage runs
// BeforeConsistencyPoint on every such action first; once they all finish it stamps the consistency point
// (the current time) and hands it to each ConsistencyPointConsumer. Any future action needing a
// pre-consistency-point step can implement this without changing the stage.
type PreConsistencyPointAction interface {
	RemoteAction
	BeforeConsistencyPoint(ctx *contexts.Context) error
}

// ConsistencyPointConsumer is an optional capability of a RemoteAction that needs the event's shared
// consistency point in order to align to it (a cloned cluster recovers forward to it; a non-DB capture is
// taken as of it). The stage calls SetConsistencyPoint after every PreConsistencyPointAction has run and
// before Setup. Unlike PreConsistencyPointAction this is a value sink rather than a lifecycle step the
// stage runs, so it is a plain capability and does not embed RemoteAction.
type ConsistencyPointConsumer interface {
	SetConsistencyPoint(c time.Time)
}

type namedRemoteAction struct {
	name         string
	remoteAction RemoteAction
}

func newNamedRemoteAction(name string, action RemoteAction) namedRemoteAction {
	return namedRemoteAction{
		name:         name,
		remoteAction: action,
	}
}

type RemoteStageOptions struct {
	CleanupTimeout helpers.MaxWaitTime `yaml:"cleanupTimeout,omitempty"`
}

type RemoteStageInterface interface {
	WithAction(friendlyName string, action RemoteAction) RemoteStageInterface
	Run(ctx *contexts.Context) error
}

type RemoteStage struct {
	actions           []namedRemoteAction
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	eventName         string
	opts              RemoteStageOptions
}

func NewRemoteStage(kubeClusterClient kubecluster.ClientInterface, namespace, eventName string, opts RemoteStageOptions) RemoteStageInterface {
	return &RemoteStage{
		kubeClusterClient: kubeClusterClient,
		namespace:         namespace,
		eventName:         eventName,
		opts:              opts,
	}
}

func (rs *RemoteStage) WithAction(friendlyName string, action RemoteAction) RemoteStageInterface {
	rs.actions = append(rs.actions, newNamedRemoteAction(friendlyName, action))
	return rs
}

func (rs *RemoteStage) cleanupFunc(ctx *contexts.Context, outerErr *error) func() {
	cleanupFuncs := []func() error{}

	for _, action := range rs.actions {
		if cleanupAction, ok := action.remoteAction.(CleanupAction); ok {
			cleanupFunc := cleanup.To(cleanupAction.Cleanup).
				WithErrMessage(fmt.Sprintf("failed to cleanup %s resources", action.name)).WithOriginalErr(outerErr).
				WithParentCtx(ctx).WithTimeout(rs.opts.CleanupTimeout.MaxWait(time.Minute)).
				Run
			cleanupFuncs = append(cleanupFuncs, cleanupFunc)
		}
	}

	return func() {
		for _, cleanupFunc := range cleanupFuncs {
			_ = cleanupFunc()
		}
	}
}

func (rs *RemoteStage) validate(ctx *contexts.Context) error {
	for _, action := range rs.actions {
		if err := action.remoteAction.Validate(ctx.Child()); err != nil {
			return trace.Wrap(err, fmt.Sprintf("failed to validate %s resources", action.name))
		}
	}

	return nil
}

func (rs *RemoteStage) setup(ctx *contexts.Context) (bti.CreateBackupToolInstanceOptions, error) {
	btiOpts := bti.CreateBackupToolInstanceOptions{
		NamePrefix:     fmt.Sprintf("%s-%s", constants.ToolName, rs.eventName),
		CleanupTimeout: rs.opts.CleanupTimeout,
	}

	// Phase 1: every action with pre-consistency-point work (e.g. a CNPG base backup) runs it now,
	// before any clone is created and before the shared instant is fixed.
	for _, action := range rs.actions {
		preAction, ok := action.remoteAction.(PreConsistencyPointAction)
		if !ok {
			continue
		}

		if err := preAction.BeforeConsistencyPoint(ctx.Child()); err != nil {
			return bti.CreateBackupToolInstanceOptions{}, trace.Wrap(err, fmt.Sprintf("failed to run pre-consistency-point step for %s", action.name))
		}
	}

	// Phase 2: the consistency point is the instant after all pre-step work completed — every capture in
	// the event is made recoverable to it. Hand it to each action that aligns to it (cloned clusters
	// recover forward to it; non-DB captures are taken as of it). Today every consumer is also a
	// pre-step action, so when nothing produced a consistency point this loop has nothing to hand it to.
	consistencyPoint := time.Now()
	for _, action := range rs.actions {
		if consumer, ok := action.remoteAction.(ConsistencyPointConsumer); ok {
			consumer.SetConsistencyPoint(consistencyPoint)
		}
	}

	// Phase 3: set up each action — CNPG actions create their clones (recovering forward to C), and
	// every action contributes its volumes/secrets to the tool pod.
	for _, action := range rs.actions {
		if err := action.remoteAction.Setup(ctx.Child(), &btiOpts); err != nil {
			return bti.CreateBackupToolInstanceOptions{}, trace.Wrap(err, fmt.Sprintf("failed to setup %s resources", action.name))
		}
	}

	return btiOpts, nil
}

func (rs *RemoteStage) execute(ctx *contexts.Context, btiOpts bti.CreateBackupToolInstanceOptions) error {
	btInstance, err := rs.kubeClusterClient.CreateBackupToolInstance(ctx.Child(), rs.namespace, rs.eventName, btiOpts)
	if err != nil {
		return trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", rs.eventName).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(rs.opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child())
	if err != nil {
		return trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	defer cleanup.To(func(ctx *contexts.Context) error {
		return backupToolClient.Close()
	}).WithErrMessage("failed to close backup tool client").WithParentCtx(ctx).
		WithOriginalErr(&err).WithTimeout(rs.opts.CleanupTimeout.MaxWait(time.Minute)).
		Run()

	for _, action := range rs.actions {
		if err := action.remoteAction.Execute(ctx.Child(), backupToolClient); err != nil {
			return trace.Wrap(err, fmt.Sprintf("failed to execute %s resources", action.name))
		}
	}

	return nil
}

// Runs each part of each action in the configured stage. Handles all cleanup.
func (rs *RemoteStage) Run(ctx *contexts.Context) (err error) {
	// Defer cleanups
	defer rs.cleanupFunc(ctx, &err)()

	// 1. Validate
	ctx.Log.Step().Info("Validating")
	if err := rs.validate(ctx); err != nil {
		return err
	}

	// 2. Setup
	ctx.Log.Step().Info("Setting up")
	btiOpts, err := rs.setup(ctx)
	if err != nil {
		return err
	}

	// 3. Execute
	ctx.Log.Step().Info("Executing")
	if err := rs.execute(ctx, btiOpts); err != nil {
		return err
	}

	return nil
}
