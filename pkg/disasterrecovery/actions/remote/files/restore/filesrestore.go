package restore

import (
	"path/filepath"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
)

type FilesRestoreOptions struct{}

// FilesRestoreInterface is a RemoteStage action that restores a data-directory capture from the DR
// volume back onto a target PVC. The target PVC must already exist and not be in use (a restore
// precondition), so unlike the backup direction it is mounted directly and no clone is taken; the action
// simply syncs the DR volume subdirectory onto it. It creates no resources of its own, so it is a plain
// RemoteAction with no Cleanup.
type FilesRestoreInterface interface {
	remote.RemoteAction
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, targetPVCName, drVolName, backupDirRelPath string, opts FilesRestoreOptions) error
}

type configureState struct {
	uid               string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured      bool
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	targetPVCName     string
	drVolName         string
	backupDirRelPath  string
	opts              FilesRestoreOptions
}

func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, targetPVCName, drVolName, backupDirRelPath string, opts FilesRestoreOptions) error {
	if cs.isConfigured {
		return trace.Errorf("attempted to configure multiple times")
	}

	cs.uid = uuid.NewString()
	cs.kubeClusterClient = kubeClusterClient
	cs.namespace = namespace
	cs.targetPVCName = targetPVCName
	cs.drVolName = drVolName
	cs.backupDirRelPath = backupDirRelPath
	cs.opts = opts

	cs.isConfigured = true
	return nil
}

func (cs *configureState) ctxLogWith(ctx *contexts.Context) *contexts.LoggerContext {
	return ctx.Log.With("targetPVC", cs.targetPVCName, "uid", cs.uid)
}

type validateState struct {
	configureState
	isValidated bool
}

func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for files restore")
	defer ctx.Log.Info("Completed files restore configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !vs.isConfigured {
		return trace.Errorf("attempted to validate without configuring")
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.targetPVCName); err != nil {
		return trace.Wrap(err, "failed to get target data PVC %q", vs.targetPVCName)
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.drVolName); err != nil {
		return trace.Wrap(err, "failed to get DR PVC %q", vs.drVolName)
	}

	vs.isValidated = true
	return nil
}

type setupStateMountPaths struct {
	drVolume string
	data     string
}

type setupState struct {
	validateState
	mountPaths setupStateMountPaths
	isSetup    bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for files restore")
	defer ctx.Log.Info("Files restore setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isValidated {
		return trace.Errorf("attempted to setup without validating")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	baseMountPath := filepath.Join("/mnt", "filesrestore", ss.uid)
	ss.mountPaths = setupStateMountPaths{
		drVolume: filepath.Join(baseMountPath, "dr"),
		data:     filepath.Join(baseMountPath, "data"),
	}

	btiOpts.Volumes = append(btiOpts.Volumes,
		core.NewSingleContainerPVC(ss.drVolName, ss.mountPaths.drVolume),
		core.NewSingleContainerPVC(ss.targetPVCName, ss.mountPaths.data),
	)

	ss.isSetup = true
	return nil
}

type executeState struct {
	setupState
}

func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing files restore")
	defer ctx.Log.Info("Files restore complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	drDataPath := filepath.Join(es.mountPaths.drVolume, es.backupDirRelPath)
	err = backupToolClient.Files().SyncFiles(ctx.Child(), drDataPath, es.mountPaths.data, files.SyncFilesOptions{})
	return trace.Wrap(err, "failed to sync data directory files at %q to the data PVC at %q", drDataPath, es.mountPaths.data)
}

type FilesRestore struct {
	executeState
}

func NewFilesRestore() FilesRestoreInterface {
	return &FilesRestore{}
}
