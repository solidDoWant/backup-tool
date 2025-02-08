package clonepvc

import (
	"time"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type ClonePVCOptions struct {
	WaitForSnapshotTimeout helpers.MaxWaitTime
	DestStorageClassName   string // Override the storage class used for the created volume. Must be compatible with the snapshot.
	DestPvcNamePrefix      string // Override the prefix used for the created volume name
	ForceBind              bool   // Force the PVC to be bound immediately. This should be set if the storage class does not have `volumeBindingMode: Immediate` set, because the snapshot will be deleted after the PVC is created.
	ForceBindTimeout       helpers.MaxWaitTime
	CleanupTimeout         helpers.MaxWaitTime
}

// Snapshots a given volume and clones it. Callers are responsible for ensuring consistency.
func (p *Provider) ClonePVC(ctx *contexts.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error) {
	snapshot, err := p.esClient.SnapshotVolume(ctx, namespace, pvcName, externalsnapshotter.SnapshotVolumeOptions{})
	if err != nil {
		err = trace.Wrap(err, "failed to snapshot %q", helpers.FullNameStr(namespace, pvcName))
		return
	}
	defer cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(time.Minute), func(ctx *contexts.Context) error {
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
	defer cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(time.Minute), func(ctx *contexts.Context) error {
		if err == nil {
			return nil
		}
		cleanupErr := p.coreClient.DeletePVC(ctx, namespace, clonedPvc.Name)
		clonedPvc = nil
		return cleanupErr
	}).WithErrMessage("failed to delete created volume for PVC %q", helpers.FullNameStr(namespace, pvcName)).WithOriginalErr(&err).Run()

	if opts.ForceBind {
		podVol := core.NewSingleContainerPVC(clonedPvc.Name, "/mnt")

		var pod *corev1.Pod
		pod, err = p.coreClient.CreatePod(ctx, namespace, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "force-bind-" + clonedPvc.Name,
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{podVol.ToVolume()},
				Containers: []corev1.Container{
					{
						Name:            "force-bind",
						Image:           "registry.k8s.io/pause", // TODO pin
						VolumeMounts:    []corev1.VolumeMount{podVol.ToVolumeMount()},
						SecurityContext: core.RestrictedContainerSecurityContext(1000, 1000),
					},
				},
				SecurityContext: core.RestrictedPodSecurityContext(1000, 1000),
				RestartPolicy:   corev1.RestartPolicyNever,
			},
		})
		if err != nil {
			err = trace.Wrap(err, "failed to create 'force bind' pod for PVC %q", helpers.FullName(clonedPvc))
			return
		}
		defer cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(time.Minute), func(ctx *contexts.Context) error {
			return p.coreClient.DeletePod(ctx, namespace, pod.Name)
		}).WithErrMessage("failed to delete 'force bind' pod for PVC %q", helpers.FullName(clonedPvc)).WithOriginalErr(&err).Run()

		_, err = p.coreClient.WaitForReadyPod(ctx, namespace, pod.Name, core.WaitForReadyPodOpts{MaxWaitTime: opts.ForceBindTimeout})
		if err != nil {
			err = trace.Wrap(err, "failed to wait for 'force bind' pod %q to become ready", helpers.FullName(pod))
			return
		}
	}

	return
}
