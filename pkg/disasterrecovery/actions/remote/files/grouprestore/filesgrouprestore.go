package grouprestore

import (
	"path/filepath"
	"sort"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/layout"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FilesGroupRestoreOptions struct{}

// FilesGroupRestoreInterface is a RemoteStage action that restores a file-group capture from the DR
// volume back onto its target PVCs. Membership is supplied the same way it was at backup time - a label
// selector - resolved against the live cluster, so the target PVCs are known up front (no on-disk
// manifest needs to be read to discover them) and are mounted directly like files/restore. The action
// creates no resources of its own, so it is a plain RemoteAction with no Cleanup.
//
// Before syncing anything, Execute enforces an exact 1:1 mapping between the captured member
// directories (layout.FileGroupsDirName/<group>/<pvc> on the DR volume) and the selector-resolved target
// PVCs: it errors if a target PVC has no captured data, or if a captured member has no target PVC. This
// guards against a half-applied restore leaving the application in a corrupted state.
type FilesGroupRestoreInterface interface {
	remote.RemoteAction
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace string, selector metav1.LabelSelector, drVolName, groupName string, opts FilesGroupRestoreOptions) error
}

type configureState struct {
	uid               string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured      bool
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	selector          metav1.LabelSelector
	drVolName         string
	groupName         string
	opts              FilesGroupRestoreOptions
}

func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace string, selector metav1.LabelSelector, drVolName, groupName string, opts FilesGroupRestoreOptions) error {
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
	// targetPVCNames are the live PVCs the selector resolved to - the restore destinations.
	targetPVCNames []string
	isValidated    bool
}

func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for files group restore")
	defer ctx.Log.Info("Completed files group restore configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !vs.isConfigured {
		return trace.Errorf("attempted to validate without configuring")
	}

	// Resolve the restore destinations from the live cluster - the PVCs currently matching the selector.
	// These must exist (they are the in-place restore targets), so an empty match is a misconfiguration.
	targetPVCs, err := vs.kubeClusterClient.Core().ListPVCs(ctx.Child(), vs.namespace, core.ListPVCsOptions{LabelSelector: vs.selector})
	if err != nil {
		return trace.Wrap(err, "failed to list target PVCs matching the group selector")
	}
	if len(targetPVCs) == 0 {
		return trace.Errorf("group selector matched no PVCs in namespace %q", vs.namespace)
	}
	vs.targetPVCNames = make([]string, 0, len(targetPVCs))
	for i := range targetPVCs {
		vs.targetPVCNames = append(vs.targetPVCNames, targetPVCs[i].Name)
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.drVolName); err != nil {
		return trace.Wrap(err, "failed to get DR PVC %q", vs.drVolName)
	}

	vs.isValidated = true
	return nil
}

type setupState struct {
	validateState
	drVolumeMountPath string
	// targetMountPaths maps each target PVC name to its mount path in the tool pod.
	targetMountPaths map[string]string
	isSetup          bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for files group restore")
	defer ctx.Log.Info("Files group restore setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isValidated {
		return trace.Errorf("attempted to setup without validating")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	baseMountPath := filepath.Join("/mnt", "filesgrouprestore", ss.uid)
	ss.drVolumeMountPath = filepath.Join(baseMountPath, "dr")
	btiOpts.Volumes = append(btiOpts.Volumes, core.NewSingleContainerPVC(ss.drVolName, ss.drVolumeMountPath))

	ss.targetMountPaths = make(map[string]string, len(ss.targetPVCNames))
	for _, targetPVCName := range ss.targetPVCNames {
		mountPath := filepath.Join(baseMountPath, "targets", targetPVCName)
		btiOpts.Volumes = append(btiOpts.Volumes, core.NewSingleContainerPVC(targetPVCName, mountPath))
		ss.targetMountPaths[targetPVCName] = mountPath
	}

	ss.isSetup = true
	return nil
}

type executeState struct {
	setupState
}

func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing files group restore")
	defer ctx.Log.Info("Files group restore complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	groupDirPath := filepath.Join(es.drVolumeMountPath, layout.FileGroupsDirName, es.groupName)

	// List the member directories the backup captured for this group.
	capturedMembers, err := backupToolClient.Files().ListDirectory(ctx.Child(), groupDirPath)
	if err != nil {
		return trace.Wrap(err, "failed to list captured members for group %q at %q", es.groupName, groupDirPath)
	}

	// Enforce an exact 1:1 mapping between captured members and target PVCs before syncing anything, so a
	// mismatch can't leave the application in a partially-restored (corrupted) state.
	if err := es.verifyOneToOneMapping(capturedMembers); err != nil {
		return trace.Wrap(err, "failed to verify one to one mapping of backup directories to recovery directories for group %q", es.groupName)
	}

	// 1:1 confirmed - restore each captured member into its identically-named target PVC.
	for targetPVCName, mountPath := range es.targetMountPaths {
		srcPath := filepath.Join(groupDirPath, targetPVCName)
		if err := backupToolClient.Files().SyncFiles(ctx.Child(), srcPath, mountPath); err != nil {
			return trace.Wrap(err, "failed to sync captured member %q at %q onto target PVC at %q", targetPVCName, srcPath, mountPath)
		}
	}

	return nil
}

// verifyOneToOneMapping errors unless the set of captured member directories exactly equals the set of
// target PVCs: every target must have captured data to restore, and every captured member must have a
// target to restore into. This is checked before any sync runs.
func (es *executeState) verifyOneToOneMapping(capturedMembers []string) error {
	capturedSet := make(map[string]struct{}, len(capturedMembers))
	for _, member := range capturedMembers {
		capturedSet[member] = struct{}{}
	}

	var targetsWithoutCapture []string // a target PVC with no captured data
	for targetPVCName := range es.targetMountPaths {
		if _, ok := capturedSet[targetPVCName]; !ok {
			targetsWithoutCapture = append(targetsWithoutCapture, targetPVCName)
		}
	}

	var capturesWithoutTarget []string // captured data with no target PVC
	for _, member := range capturedMembers {
		if _, ok := es.targetMountPaths[member]; !ok {
			capturesWithoutTarget = append(capturesWithoutTarget, member)
		}
	}

	if len(targetsWithoutCapture) == 0 && len(capturesWithoutTarget) == 0 {
		return nil
	}

	// Sort for a deterministic, readable error.
	sort.Strings(targetsWithoutCapture)
	sort.Strings(capturesWithoutTarget)
	return trace.Errorf("refusing to restore group %q: backup/restore membership is not 1:1 (target PVCs without captured data: %v; captured members without a target PVC: %v)",
		es.groupName, targetsWithoutCapture, capturesWithoutTarget)
}

type FilesGroupRestore struct {
	executeState
}

func NewFilesGroupRestore() FilesGroupRestoreInterface {
	return &FilesGroupRestore{}
}
