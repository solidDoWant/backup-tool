package kubecluster

import (
	context "context"
	"time"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

type ClonePVCOptions struct {
	WaitForSnapshotTimeout helpers.MaxWaitTime
	DestStorageClassName   string // Override the storage class used for the created volume. Must be compatible with the snapshot.
	DestPvcNamePrefix      string // Override the prefix used for the created volume name
	CleanupTimeout         helpers.MaxWaitTime
}

// Snapshots a given volume and clones it. Callers are responsible for ensuring consistency.
func (c *Client) ClonePVC(ctx context.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error) {
	snapshot, err := c.ES().SnapshotVolume(ctx, namespace, pvcName, externalsnapshotter.SnapshotVolumeOptions{})
	if err != nil {
		err = trace.Wrap(err, "failed to snapshot %q", helpers.FullNameStr(namespace, pvcName))
		return
	}
	defer cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(time.Minute), func(ctx context.Context) error {
		return c.ES().DeleteSnapshot(ctx, namespace, snapshot.Name)
	}).WithErrMessage("failed to delete created snapshot for PVC %q", helpers.FullNameStr(namespace, pvcName)).WithOriginalErr(&err).Run()

	err = c.ES().WaitForReadySnapshot(ctx, namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: helpers.ShortWaitTime})
	if err != nil {
		err = trace.Wrap(err, "failed to wait for snapshot %q to become ready", helpers.FullName(snapshot))
		return
	}

	pvcNamePrefix := pvcName
	if opts.DestPvcNamePrefix != "" {
		pvcNamePrefix = opts.DestPvcNamePrefix
	}

	var storageClassName string
	if opts.DestStorageClassName != "" {
		storageClassName = opts.DestStorageClassName
	} else {
		// Default to the original PVC's storage class if none is specified
		var srcPvc *corev1.PersistentVolumeClaim
		srcPvc, err = c.Core().GetPVC(ctx, namespace, pvcName)
		if err != nil {
			err = trace.Wrap(err, "failed to get existing PVC %q", helpers.FullNameStr(namespace, pvcName))
			return
		}

		if srcPvc.Spec.StorageClassName != nil {
			storageClassName = *srcPvc.Spec.StorageClassName
		}
	}

	// TODO add an override option for this
	var size resource.Quantity
	if snapshot.Status != nil && snapshot.Status.RestoreSize != nil {
		size = *snapshot.Status.RestoreSize
	} else {
		err = trace.Errorf("snapshot %q does not have a restore size", helpers.FullName(snapshot))
		return
	}

	clonedPvc, err = c.Core().CreatePVC(ctx, namespace, pvcNamePrefix, size, core.CreatePVCOptions{
		GenerateName:     true,
		StorageClassName: storageClassName,
		Source: &corev1.TypedObjectReference{
			APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Identifier()),
			Kind:     externalsnapshotter.VolumeSnapshotKind,
			Name:     snapshot.Name,
		},
	})
	if err != nil {
		err = trace.Wrap(err, "failed to create volume from created snapshot %q", helpers.FullName(snapshot))
		return
	}

	return
}
