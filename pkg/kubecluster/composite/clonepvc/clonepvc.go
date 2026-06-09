package clonepvc

import (
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
)

type ClonePVCOptions struct {
	WaitForSnapshotTimeout helpers.MaxWaitTime
	SnapshotClass          string // Override the VolumeSnapshotClass used to snapshot the source volume. Defaults to the cluster default when empty.
	DestStorageClassName   string // Override the storage class used for the created volume. Must be compatible with the snapshot.
	DestPvcNamePrefix      string // Override the prefix used for the created volume name
	ForceBind              bool   // Force the PVC to be bound immediately. This should be set if the storage class does not have `volumeBindingMode: Immediate` set, because the snapshot will be deleted after the PVC is created.
	ForceBindTimeout       helpers.MaxWaitTime
	CleanupTimeout         helpers.MaxWaitTime
}

// Snapshots a given volume and clones it. Callers are responsible for ensuring consistency.
func (p *Provider) ClonePVC(ctx *contexts.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error) {
	ctx.Log.With("existingPVC", pvcName).Info("Cloning PVC")
	defer ctx.Log.Info("Finished cloning PVC", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	ctx.Log.Step().Info("Creating snapshot of PVC")
	snapshot, err := p.esClient.SnapshotVolume(ctx.Child(), namespace, pvcName, externalsnapshotter.SnapshotVolumeOptions{SnapshotClass: opts.SnapshotClass})
	if err != nil {
		err = trace.Wrap(err, "failed to snapshot %q", helpers.FullNameStr(namespace, pvcName))
		return
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		return p.esClient.DeleteSnapshot(ctx, namespace, snapshot.Name)
	}).WithErrMessage("failed to delete created snapshot for PVC %q", helpers.FullNameStr(namespace, pvcName)).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	readySnapshot, err := p.esClient.WaitForReadySnapshot(ctx.Child(), namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: opts.WaitForSnapshotTimeout})
	if err != nil {
		err = trace.Wrap(err, "failed to wait for snapshot %q to become ready", helpers.FullName(snapshot))
		return
	}

	pvcNamePrefix := pvcName
	if opts.DestPvcNamePrefix != "" {
		pvcNamePrefix = opts.DestPvcNamePrefix
	}
	ctx.Log.With("newPVC", pvcNamePrefix).Step().Info("Creating PVC from snapshot", "snapshot", readySnapshot.Name)

	clonedPvc, err = p.createPVCFromSnapshot(ctx.Child(), namespace, pvcNamePrefix, pvcName, readySnapshot, opts.DestStorageClassName)
	if err != nil {
		err = trace.Wrap(err, "failed to create volume from created snapshot %q", helpers.FullName(readySnapshot))
		return
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		if err == nil {
			return nil
		}
		cleanupErr := p.coreClient.DeletePVC(ctx, namespace, clonedPvc.Name)
		clonedPvc = nil
		return cleanupErr
	}).WithErrMessage("failed to delete created volume for PVC %q", helpers.FullNameStr(namespace, pvcName)).WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	if opts.ForceBind {
		ctx.Log.Step().Info("Forcing immediate bind of PVC")
		err = p.forceBindVolumes(ctx.Child(), namespace, []string{clonedPvc.Name}, forceBindVolumesOptions{
			WaitForReadyTimeout: opts.ForceBindTimeout,
			CleanupTimeout:      opts.CleanupTimeout,
		})
		if err != nil {
			err = trace.Wrap(err, "failed to force bind cloned PVC %q", helpers.FullName(clonedPvc))
			return
		}
	}

	return
}
