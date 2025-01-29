package externalsnapshotter

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const VolumeSnapshotKind = "VolumeSnapshot" // This is not exported by the external-snapshotter package

type SnapshotVolumeOptions struct {
	Name string
}

func (c *Client) SnapshotVolume(ctx context.Context, namespace, pvcName string, opts SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
	snapshot := &volumesnapshotv1.VolumeSnapshot{
		Spec: volumesnapshotv1.VolumeSnapshotSpec{
			Source: volumesnapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptr.To(pvcName),
			},
		},
	}

	if opts.Name == "" {
		snapshot.ObjectMeta.GenerateName = helpers.CleanName(pvcName)
	} else {
		snapshot.ObjectMeta.Name = opts.Name
	}

	snapshot, err := c.client.SnapshotV1().VolumeSnapshots(namespace).Create(ctx, snapshot, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create snapshot for volume %q", helpers.FullNameStr(namespace, pvcName))
	}

	return snapshot, nil
}

type WaitForReadySnapshotOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadySnapshot(ctx context.Context, namespace, name string, opts WaitForReadySnapshotOpts) (*volumesnapshotv1.VolumeSnapshot, error) {
	processEvent := func(_ context.Context, snapshot *volumesnapshotv1.VolumeSnapshot) (*volumesnapshotv1.VolumeSnapshot, bool, error) {
		if snapshot.Status == nil {
			return nil, false, nil
		}

		if snapshot.Status.Error != nil {
			return nil, false, trace.Errorf("snapshot %q failed: %v", helpers.FullNameStr(namespace, name), *snapshot.Status.Error.Message)
		}

		if snapshot.Status == nil || snapshot.Status.ReadyToUse == nil {
			return nil, false, nil
		}

		if *snapshot.Status.ReadyToUse {
			return snapshot, true, nil
		}
		return nil, false, nil
	}

	snapshot, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(10*time.Minute), c.client.SnapshotV1().VolumeSnapshots(namespace), name, processEvent)

	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for snapshot to become ready")
	}

	return snapshot, nil
}

func (c *Client) DeleteSnapshot(ctx context.Context, namespace, snapshotName string) error {
	err := c.client.SnapshotV1().VolumeSnapshots(namespace).Delete(ctx, snapshotName, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete snapshot %q", helpers.FullNameStr(namespace, snapshotName))
}
