package clonepvc

import (
	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// createPVCFromSnapshot creates a (generate-named) PVC cloned from a ready VolumeSnapshot. The clone
// is sized from the snapshot's restore size and, unless destStorageClassName overrides it, uses the
// storage class of sourcePVCName (the PVC the snapshot was taken from). It does not force-bind or
// clean up the created PVC on a later error - callers own that, because the single-volume and
// group-volume clone paths manage cleanup differently.
//
// It is the shared per-snapshot building block of ClonePVC and ClonePVCGroup, which both clone a PVC
// from a snapshot (an individual VolumeSnapshot or a VolumeGroupSnapshot member) once it is ready.
func (p *Provider) createPVCFromSnapshot(ctx *contexts.Context, namespace, destNamePrefix, sourcePVCName string, snapshot *volumesnapshotv1.VolumeSnapshot, destStorageClassName string) (*corev1.PersistentVolumeClaim, error) {
	storageClassName := destStorageClassName
	if storageClassName == "" {
		// Default to the source PVC's storage class if none is specified.
		srcPvc, err := p.coreClient.GetPVC(ctx.Child(), namespace, sourcePVCName)
		if err != nil {
			return nil, trace.Wrap(err, "failed to get source PVC %q", helpers.FullNameStr(namespace, sourcePVCName))
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
		return nil, trace.Errorf("snapshot %q does not have a restore size", helpers.FullName(snapshot))
	}

	clonedPvc, err := p.coreClient.CreatePVC(ctx.Child(), namespace, destNamePrefix, size, core.CreatePVCOptions{
		GenerateName:     true,
		StorageClassName: storageClassName,
		Source: &corev1.TypedObjectReference{
			APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
			Kind:     externalsnapshotter.VolumeSnapshotKind,
			Name:     snapshot.Name,
		},
	})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create volume from snapshot %q", helpers.FullName(snapshot))
	}

	return clonedPvc, nil
}
