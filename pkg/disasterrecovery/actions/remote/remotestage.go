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
// recoverable to. There are two kinds of pre-step work, distinguished by the instant it returns:
//
//   - A capture that *pins* an instant freezes state at a wall-clock moment it cannot choose
//     retroactively — e.g. a filesystem PVC snapshot, which exists only at the moment it is taken. It
//     returns that moment, making it a candidate for the consistency point.
//   - A capture that only needs to *precede* the point pins nothing and returns the zero time — e.g. a
//     database base backup, from which the clone later recovers forward to the point.
//
// The stage runs BeforeConsistencyPoint on every such action first, then sets the consistency point to the
// earliest non-zero instant any of them pinned (or the current time if none did) and hands it to each
// ConsistencyPointConsumer. Choosing the earliest keeps every forward-recoverable capture (a DB clone) and
// as-of capture (S3) from landing later than a capture already frozen at a fixed instant. Any future
// action needing a pre-consistency-point step can implement this without changing the stage.
type PreConsistencyPointAction interface {
	RemoteAction
	BeforeConsistencyPoint(ctx *contexts.Context) (time.Time, error)
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
	cleanupFuncs := []func(){}

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
			cleanupFunc()
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

	// Phase 1: every action with pre-consistency-point work (e.g. a CNPG base backup, or a filesystem PVC
	// clone) runs it now, before any clone of a recovering cluster is created and before the shared instant
	// is fixed. Each returns the instant its capture pinned, or the zero time if it pins none (it only needs
	// to precede the point).
	var consistencyPoint time.Time
	for _, action := range rs.actions {
		preAction, ok := action.remoteAction.(PreConsistencyPointAction)
		if !ok {
			continue
		}

		pinnedTime, err := preAction.BeforeConsistencyPoint(ctx.Child())
		if err != nil {
			return bti.CreateBackupToolInstanceOptions{}, trace.Wrap(err, fmt.Sprintf("failed to run pre-consistency-point step for %s", action.name))
		}

		// Track the earliest instant any capture pinned (see PreConsistencyPointAction for why earliest).
		if !pinnedTime.IsZero() && (consistencyPoint.IsZero() || pinnedTime.Before(consistencyPoint)) {
			consistencyPoint = pinnedTime
		}
	}

	// When no capture pinned an instant, the point is simply the moment after all pre-step work completed.
	if consistencyPoint.IsZero() {
		consistencyPoint = time.Now()
	}

	// Phase 2: every capture in the event is made recoverable to the consistency point. Hand it to each
	// action that aligns to it (cloned clusters recover forward to it; non-DB captures are taken as of it).
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
