package clonepvc

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
func (p *Provider) ClonePVC(ctx context.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error) {
	snapshot, err := p.esClient.SnapshotVolume(ctx, namespace, pvcName, externalsnapshotter.SnapshotVolumeOptions{})
	if err != nil {
		err = trace.Wrap(err, "failed to snapshot %q", helpers.FullNameStr(namespace, pvcName))
		return
	}
	defer cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(time.Minute), func(ctx context.Context) error {
		return p.esClient.DeleteSnapshot(ctx, namespace, snapshot.Name)
	}).WithErrMessage("failed to delete created snapshot for PVC %q", helpers.FullNameStr(namespace, pvcName)).WithOriginalErr(&err).Run()

	readySnapshot, err := p.esClient.WaitForReadySnapshot(ctx, namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: opts.WaitForSnapshotTimeout})
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
		srcPvc, err = p.coreClient.GetPVC(ctx, namespace, pvcName)
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
	if readySnapshot.Status != nil && readySnapshot.Status.RestoreSize != nil {
		size = *readySnapshot.Status.RestoreSize
	} else {
		err = trace.Errorf("snapshot %q does not have a restore size", helpers.FullName(readySnapshot))
		return
	}

	clonedPvc, err = p.coreClient.CreatePVC(ctx, namespace, pvcNamePrefix, size, core.CreatePVCOptions{
		GenerateName:     true,
		StorageClassName: storageClassName,
		Source: &corev1.TypedObjectReference{
			APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
			Kind:     externalsnapshotter.VolumeSnapshotKind,
			Name:     readySnapshot.Name,
		},
	})
	if err != nil {
		err = trace.Wrap(err, "failed to create volume from created snapshot %q", helpers.FullName(readySnapshot))
		return
	}

	return
}
