package externalsnapshotter

import (
	"time"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const VolumeSnapshotKind = "VolumeSnapshot" // This is not exported by the external-snapshotter package

type SnapshotVolumeOptions struct {
	Name          string
	SnapshotClass string
}

func (c *Client) SnapshotVolume(ctx *contexts.Context, namespace, pvcName string, opts SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
	ctx.Log.With("name", pvcName).Info("Creating snapshot for volume")
	ctx.Log.Debug("Call parameters", "opts", opts)

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

	if opts.SnapshotClass != "" {
		snapshot.Spec.VolumeSnapshotClassName = &opts.SnapshotClass
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

func (c *Client) WaitForReadySnapshot(ctx *contexts.Context, namespace, name string, opts WaitForReadySnapshotOpts) (snapshot *volumesnapshotv1.VolumeSnapshot, err error) {
	ctx.Log.With("name", name).Info("Waiting for snapshot to become ready")
	defer ctx.Log.Info("Finished waiting for snapshot to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	processEvent := func(ctx *contexts.Context, snapshot *volumesnapshotv1.VolumeSnapshot) (*volumesnapshotv1.VolumeSnapshot, bool, error) {
		ctx.Log.Debug("Snapshot status", "status", snapshot.Status)

		if snapshot.Status == nil {
			return nil, false, nil
		}

		// This previously checked is the snapshot was in an error state, but it was removed because
		// the volume snapshot controller appears to sometimes place a snapshot in an error state, and
		// later resolve it successfully.

		if snapshot.Status == nil || snapshot.Status.ReadyToUse == nil {
			return nil, false, nil
		}

		if *snapshot.Status.ReadyToUse {
			return snapshot, true, nil
		}
		return nil, false, nil
	}
	snapshot, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(10*time.Minute), c.client.SnapshotV1().VolumeSnapshots(namespace), name, processEvent)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for snapshot to become ready")
	}

	return snapshot, nil
}

func (c *Client) DeleteSnapshot(ctx *contexts.Context, namespace, snapshotName string) error {
	ctx.Log.With("name", snapshotName).Info("Deleting snapshot")

	err := c.client.SnapshotV1().VolumeSnapshots(namespace).Delete(ctx, snapshotName, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete snapshot %q", helpers.FullNameStr(namespace, snapshotName))
}
