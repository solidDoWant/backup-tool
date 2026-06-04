package backup

import (
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	corev1 "k8s.io/api/core/v1"
)

type FilesBackupOptions struct {
	CleanupTimeout helpers.MaxWaitTime `yaml:"cleanupTimeout,omitempty"`
}

// FilesBackupInterface is a RemoteStage action that captures a live data-directory PVC into the DR
// volume. The source PVC is in use, so it cannot be read directly; the action snapshots/clones it for a
// consistent point-in-time view, syncs the clone's contents into a subdirectory of the DR volume, and
// tears the clone down afterwards (so it is a CleanupAction). A volume snapshot exists only at the moment
// it is taken and cannot be reconstructed for an arbitrary instant, so as a remote.PreConsistencyPointAction
// it takes the clone before the consistency point is fixed and pins the point to the clone's creation time;
// the other captures then align to that filesystem freeze (a database clone recovers forward to it).
type FilesBackupInterface interface {
	remote.CleanupAction
	remote.PreConsistencyPointAction
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, sourcePVCName, drVolName, backupDirRelPath string, opts FilesBackupOptions) error
}

type configureState struct {
	uid               string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured      bool
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	sourcePVCName     string
	drVolName         string
	backupDirRelPath  string
	opts              FilesBackupOptions
}

func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, sourcePVCName, drVolName, backupDirRelPath string, opts FilesBackupOptions) error {
	if cs.isConfigured {
		return trace.Errorf("attempted to configure multiple times")
	}

	cs.uid = uuid.NewString()
	cs.kubeClusterClient = kubeClusterClient
	cs.namespace = namespace
	cs.sourcePVCName = sourcePVCName
	cs.drVolName = drVolName
	cs.backupDirRelPath = backupDirRelPath
	cs.opts = opts

	cs.isConfigured = true
	return nil
}

func (cs *configureState) ctxLogWith(ctx *contexts.Context) *contexts.LoggerContext {
	return ctx.Log.With("sourcePVC", cs.sourcePVCName, "uid", cs.uid)
}

type validateState struct {
	configureState
	isValidated bool
}

func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for files backup")
	defer ctx.Log.Info("Completed files backup configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !vs.isConfigured {
		return trace.Errorf("attempted to validate without configuring")
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.sourcePVCName); err != nil {
		return trace.Wrap(err, "failed to get source data PVC %q", vs.sourcePVCName)
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.drVolName); err != nil {
		return trace.Wrap(err, "failed to get DR PVC %q", vs.drVolName)
	}

	vs.isValidated = true
	return nil
}

// cloneState holds the PVC clone taken before the consistency point is established.
type cloneState struct {
	validateState
	clonedPVC *corev1.PersistentVolumeClaim
	isCloned  bool
}

// BeforeConsistencyPoint clones the source data PVC and returns the clone's creation time, pinning the
// event's consistency point to the moment the filesystem was frozen so the database recovers forward to
// exactly that state. Implements remote.PreConsistencyPointAction.
func (cs *cloneState) BeforeConsistencyPoint(ctx *contexts.Context) (_ time.Time, err error) {
	cs.ctxLogWith(ctx).Info("Cloning data directory for files backup")
	defer ctx.Log.Info("Files backup clone complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !cs.isValidated {
		return time.Time{}, trace.Errorf("attempted to clone without validating")
	}

	if cs.isCloned {
		return time.Time{}, trace.Errorf("attempted to clone multiple times")
	}

	// ForceBind is required because the snapshot is deleted once the clone exists, so the clone must bind
	// immediately.
	clonedPVC, err := cs.kubeClusterClient.ClonePVC(ctx.Child(), cs.namespace, cs.sourcePVCName, clonepvc.ClonePVCOptions{
		DestPvcNamePrefix: cs.drVolName,
		ForceBind:         true,
		CleanupTimeout:    cs.opts.CleanupTimeout,
	})
	if err != nil {
		return time.Time{}, trace.Wrap(err, "failed to clone source data PVC %q", cs.sourcePVCName)
	}
	cs.clonedPVC = clonedPVC
	cs.isCloned = true

	return clonedPVC.CreationTimestamp.Time, nil
}

type setupStateMountPaths struct {
	drVolume string
	data     string
}

type setupState struct {
	cloneState
	mountPaths setupStateMountPaths
	isSetup    bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for files backup")
	defer ctx.Log.Info("Files backup setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isCloned {
		return trace.Errorf("attempted to setup without cloning")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	baseMountPath := filepath.Join("/mnt", "filesbackup", ss.uid)
	ss.mountPaths = setupStateMountPaths{
		drVolume: filepath.Join(baseMountPath, "dr"),
		data:     filepath.Join(baseMountPath, "data"),
	}

	btiOpts.Volumes = append(btiOpts.Volumes,
		core.NewSingleContainerPVC(ss.drVolName, ss.mountPaths.drVolume),
		core.NewSingleContainerPVC(ss.clonedPVC.Name, ss.mountPaths.data),
	)

	ss.isSetup = true
	return nil
}

// Cleanup tears down the cloned PVC. It tolerates partial state (e.g. the action never reached Setup
// because another action failed first), deleting nothing in that case.
func (ss *setupState) Cleanup(ctx *contexts.Context) error {
	if ss.clonedPVC == nil {
		return nil
	}

	err := cleanup.To(func(ctx *contexts.Context) error {
		return ss.kubeClusterClient.Core().DeletePVC(ctx, ss.namespace, ss.clonedPVC.Name)
	}).WithErrMessage("failed to cleanup cloned data PVC %q", helpers.FullName(ss.clonedPVC)).
		WithParentCtx(ctx).WithTimeout(ss.opts.CleanupTimeout.MaxWait(time.Minute)).
		RunError()
	return trace.Wrap(err, "failed to cleanup files backup resources")
}

type executeState struct {
	setupState
}

func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing files backup")
	defer ctx.Log.Info("Files backup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	drDataPath := filepath.Join(es.mountPaths.drVolume, es.backupDirRelPath)
	err = backupToolClient.Files().SyncFiles(ctx.Child(), es.mountPaths.data, drDataPath)
	return trace.Wrap(err, "failed to sync data directory files at %q to the disaster recovery volume at %q", es.mountPaths.data, drDataPath)
}

type FilesBackup struct {
	executeState
}

func NewFilesBackup() FilesBackupInterface {
	return &FilesBackup{}
}
