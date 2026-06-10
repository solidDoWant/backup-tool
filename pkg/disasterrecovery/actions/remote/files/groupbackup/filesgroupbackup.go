package groupbackup

import (
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/layout"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FilesGroupBackupOptions struct {
	SnapshotClass  string              `yaml:"snapshotClass,omitempty"` // VolumeGroupSnapshotClass used to snapshot the source PVCs; cluster default when empty.
	CleanupTimeout helpers.MaxWaitTime `yaml:"cleanupTimeout,omitempty"`
}

// FilesGroupBackupInterface is a RemoteStage action that captures a label-selected group of live
// data-directory PVCs into the DR volume as a single atomic unit. Unlike files/backup (one PVC via an
// individual VolumeSnapshot), it snapshots the whole group with one VolumeGroupSnapshot so every member
// is frozen at the same CSI-consistent instant, clones each member, and syncs each clone's contents into
// layout.FileGroupsDirName/<group>/<pvc> on the DR volume. The clones (and the group snapshot) are torn down
// afterwards, so it is a CleanupAction. The group snapshot exists only at the moment it is taken and
// cannot be reconstructed for an arbitrary instant, so as a remote.PreConsistencyPointAction it takes the
// snapshot before the consistency point is fixed and pins the point to the group's freeze instant - one
// atomic instant for the whole group, which the other captures then align to.
type FilesGroupBackupInterface interface {
	remote.CleanupAction
	remote.PreConsistencyPointAction
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace string, selector metav1.LabelSelector, drVolName, groupName string, opts FilesGroupBackupOptions) error
}

type configureState struct {
	uid               string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured      bool
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	selector          metav1.LabelSelector
	drVolName         string
	groupName         string
	opts              FilesGroupBackupOptions
}

func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace string, selector metav1.LabelSelector, drVolName, groupName string, opts FilesGroupBackupOptions) error {
	if cs.isConfigured {
		return trace.Errorf("attempted to configure multiple times")
	}

	cs.uid = uuid.NewString()
	cs.kubeClusterClient = kubeClusterClient
	cs.namespace = namespace
	cs.selector = selector
	cs.drVolName = drVolName
	cs.groupName = groupName
	cs.opts = opts

	cs.isConfigured = true
	return nil
}

func (cs *configureState) ctxLogWith(ctx *contexts.Context) *contexts.LoggerContext {
	return ctx.Log.With("group", cs.groupName, "uid", cs.uid)
}

type validateState struct {
	configureState
	isValidated bool
}

func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for files group backup")
	defer ctx.Log.Info("Completed files group backup configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !vs.isConfigured {
		return trace.Errorf("attempted to validate without configuring")
	}

	// The selector must match at least one PVC - a group that captures nothing is a misconfiguration.
	// This is the snapshot controller's own definition of membership (it selects PVCs by this selector).
	sourcePVCs, err := vs.kubeClusterClient.Core().ListPVCs(ctx.Child(), vs.namespace, core.ListPVCsOptions{LabelSelector: vs.selector})
	if err != nil {
		return trace.Wrap(err, "failed to list source PVCs matching the group selector")
	}
	if len(sourcePVCs) == 0 {
		return trace.Errorf("group selector matched no PVCs in namespace %q", vs.namespace)
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.drVolName); err != nil {
		return trace.Wrap(err, "failed to get DR PVC %q", vs.drVolName)
	}

	vs.isValidated = true
	return nil
}

// cloneState holds the group snapshot + member clones taken before the consistency point is established.
type cloneState struct {
	validateState
	cloneResult *clonepvc.ClonePVCGroupResult
	isCloned    bool
}

// BeforeConsistencyPoint atomically snapshots the whole group and clones each member, then returns the
// group snapshot's freeze instant, pinning the event's consistency point to the moment the group was
// frozen so the other captures align to exactly that state. Implements remote.PreConsistencyPointAction.
func (cs *cloneState) BeforeConsistencyPoint(ctx *contexts.Context) (_ time.Time, err error) {
	cs.ctxLogWith(ctx).Info("Cloning data directory group for files group backup")
	defer ctx.Log.Info("Files group backup clone complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !cs.isValidated {
		return time.Time{}, trace.Errorf("attempted to clone without validating")
	}

	if cs.isCloned {
		return time.Time{}, trace.Errorf("attempted to clone multiple times")
	}

	// ForceBind is required because deleting the group snapshot deletes the member snapshots, so the
	// clones must bind immediately.
	cloneResult, err := cs.kubeClusterClient.ClonePVCGroup(ctx.Child(), cs.namespace, cs.selector, clonepvc.ClonePVCGroupOptions{
		SnapshotClass:  cs.opts.SnapshotClass,
		ForceBind:      true,
		CleanupTimeout: cs.opts.CleanupTimeout,
	})
	if err != nil {
		return time.Time{}, trace.Wrap(err, "failed to clone source data PVC group")
	}
	cs.cloneResult = cloneResult
	cs.isCloned = true

	// Prefer the group snapshot's physical freeze instant (status.creationTime, the CSI-reported moment),
	// falling back to the object's creation timestamp when the status isn't populated.
	groupSnapshot := cloneResult.GroupSnapshot
	instant := groupSnapshot.CreationTimestamp.Time
	if groupSnapshot.Status != nil && groupSnapshot.Status.CreationTime != nil {
		instant = groupSnapshot.Status.CreationTime.Time
	}

	return instant, nil
}

type setupState struct {
	cloneState
	drVolumeMountPath string
	// memberMountPaths maps each source PVC name to the mount path of its clone in the tool pod.
	memberMountPaths map[string]string
	isSetup          bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for files group backup")
	defer ctx.Log.Info("Files group backup setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isCloned {
		return trace.Errorf("attempted to setup without cloning")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	baseMountPath := filepath.Join("/mnt", "filesgroupbackup", ss.uid)
	ss.drVolumeMountPath = filepath.Join(baseMountPath, "dr")
	btiOpts.Volumes = append(btiOpts.Volumes, core.NewSingleContainerPVC(ss.drVolName, ss.drVolumeMountPath))

	ss.memberMountPaths = make(map[string]string, len(ss.cloneResult.ClonedPVCs))
	for sourcePVCName, clonedPVC := range ss.cloneResult.ClonedPVCs {
		mountPath := filepath.Join(baseMountPath, "members", sourcePVCName)
		btiOpts.Volumes = append(btiOpts.Volumes, core.NewSingleContainerPVC(clonedPVC.Name, mountPath))
		ss.memberMountPaths[sourcePVCName] = mountPath
	}

	ss.isSetup = true
	return nil
}

// Cleanup tears down the member clones and the group snapshot. It tolerates partial state (e.g. the
// action never reached BeforeConsistencyPoint because another action failed first), deleting nothing in
// that case. ClonePVCGroup cleans up after itself on failure, so cloneResult is only non-nil when the
// whole group cloned successfully.
func (ss *setupState) Cleanup(ctx *contexts.Context) error {
	if ss.cloneResult == nil {
		return nil
	}

	err := cleanup.To(func(ctx *contexts.Context) error {
		var errs []error
		for sourcePVCName, clonedPVC := range ss.cloneResult.ClonedPVCs {
			if clonedPVC == nil {
				continue
			}
			if deleteErr := ss.kubeClusterClient.Core().DeletePVC(ctx.Child(), ss.namespace, clonedPVC.Name); deleteErr != nil {
				errs = append(errs, trace.Wrap(deleteErr, "failed to delete cloned PVC %q for source %q", clonedPVC.Name, sourcePVCName))
			}
		}

		if ss.cloneResult.GroupSnapshot != nil {
			if deleteErr := ss.kubeClusterClient.ES().DeleteGroupSnapshot(ctx.Child(), ss.namespace, ss.cloneResult.GroupSnapshot.Name); deleteErr != nil {
				errs = append(errs, trace.Wrap(deleteErr, "failed to delete group snapshot %q", ss.cloneResult.GroupSnapshot.Name))
			}
		}

		return trace.NewAggregate(errs...)
	}).WithErrMessage("failed to cleanup files group backup resources").
		WithParentCtx(ctx).WithTimeout(ss.opts.CleanupTimeout.MaxWait(time.Minute)).
		RunError()
	return trace.Wrap(err, "failed to cleanup files group backup resources")
}

type executeState struct {
	setupState
}

func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing files group backup")
	defer ctx.Log.Info("Files group backup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	for sourcePVCName, mountPath := range es.memberMountPaths {
		drDataPath := filepath.Join(es.drVolumeMountPath, layout.FileGroupsDirName, es.groupName, sourcePVCName)
		if err := backupToolClient.Files().SyncFiles(ctx.Child(), mountPath, drDataPath, files.SyncFilesOptions{}); err != nil {
			return trace.Wrap(err, "failed to sync member %q files at %q to the disaster recovery volume at %q", sourcePVCName, mountPath, drDataPath)
		}
	}

	return nil
}

type FilesGroupBackup struct {
	executeState
}

func NewFilesGroupBackup() FilesGroupBackupInterface {
	return &FilesGroupBackup{}
}
