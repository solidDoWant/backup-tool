package externalsnapshotter

import (
	"maps"
	"slices"
	"time"

	"github.com/gravitational/trace"
	volumegroupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const VolumeGroupSnapshotKind = "VolumeGroupSnapshot" // This is not exported by the external-snapshotter package

// Default GenerateName prefix used when no explicit group snapshot name is provided.
const generatedGroupSnapshotNamePrefix = "volume-group-snapshot-"

type GroupSnapshotOptions struct {
	Name          string
	SnapshotClass string
}

func (c *Client) GroupSnapshotVolumes(ctx *contexts.Context, namespace string, selector metav1.LabelSelector, opts GroupSnapshotOptions) (*volumegroupsnapshotv1.VolumeGroupSnapshot, error) {
	ctx.Log.Info("Creating group snapshot for volumes")
	ctx.Log.Debug("Call parameters", "selector", selector, "opts", opts)

	groupSnapshot := &volumegroupsnapshotv1.VolumeGroupSnapshot{
		Spec: volumegroupsnapshotv1.VolumeGroupSnapshotSpec{
			Source: volumegroupsnapshotv1.VolumeGroupSnapshotSource{
				Selector: selector.DeepCopy(),
			},
		},
	}

	if opts.Name == "" {
		groupSnapshot.GenerateName = generatedGroupSnapshotNamePrefix
	} else {
		groupSnapshot.Name = opts.Name
	}

	if opts.SnapshotClass != "" {
		groupSnapshot.Spec.VolumeGroupSnapshotClassName = new(opts.SnapshotClass)
	}

	groupSnapshot, err := c.client.GroupsnapshotV1().VolumeGroupSnapshots(namespace).Create(ctx, groupSnapshot, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create group snapshot in namespace %q", namespace)
	}

	return groupSnapshot, nil
}

type WaitForReadyGroupSnapshotOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadyGroupSnapshot(ctx *contexts.Context, namespace, name string, opts WaitForReadyGroupSnapshotOpts) (groupSnapshot *volumegroupsnapshotv1.VolumeGroupSnapshot, err error) {
	ctx.Log.With("name", name).Info("Waiting for group snapshot to become ready")
	defer ctx.Log.Info("Finished waiting for group snapshot to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	processEvent := func(ctx *contexts.Context, groupSnapshot *volumegroupsnapshotv1.VolumeGroupSnapshot) (*volumegroupsnapshotv1.VolumeGroupSnapshot, bool, error) {
		ctx.Log.Debug("Group snapshot status", "status", groupSnapshot.Status)

		// As with VolumeSnapshots, the controller can transiently report an error state and later
		// resolve it successfully, so don't fail on the error field.

		if groupSnapshot.Status == nil || groupSnapshot.Status.ReadyToUse == nil {
			return nil, false, nil
		}

		if *groupSnapshot.Status.ReadyToUse {
			return groupSnapshot, true, nil
		}
		return nil, false, nil
	}
	groupSnapshot, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(10*time.Minute), c.client.GroupsnapshotV1().VolumeGroupSnapshots(namespace), name, processEvent)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for group snapshot to become ready")
	}

	return groupSnapshot, nil
}

type WaitForReadyGroupSnapshotMembersOpts struct {
	helpers.MaxWaitTime
}

// WaitForReadyGroupSnapshotMembers waits until expectedCount member VolumeSnapshots of the named
// VolumeGroupSnapshot are present and individually ready, then returns them.
//
// It must wait rather than list once: a VolumeGroupSnapshot reports ready based on its
// VolumeGroupSnapshotContent (set by the CSI sidecar), but the individual member VolumeSnapshots are created
// with empty status around that same moment and reconciled to ready by a separate snapshot-controller loop
// afterwards. expectedCount is the number of PVCs matched by the group's selector - the controller creates
// exactly one member per matched PVC. See isReadyGroupMember for how a member is identified and judged ready.
func (c *Client) WaitForReadyGroupSnapshotMembers(ctx *contexts.Context, namespace, groupSnapshotName string, expectedCount int, opts WaitForReadyGroupSnapshotMembersOpts) (members []*volumesnapshotv1.VolumeSnapshot, err error) {
	ctx.Log.With("name", groupSnapshotName, "expectedMembers", expectedCount).Info("Waiting for group snapshot members to become ready")
	defer ctx.Log.Info("Finished waiting for group snapshot members to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if expectedCount <= 0 {
		return nil, trace.Errorf("expected member count must be positive, got %d", expectedCount)
	}

	// Accumulate ready members across watch events, keyed by name so re-delivered events don't
	// double-count and a member that briefly leaves the ready state is dropped.
	readyMembers := make(map[string]*volumesnapshotv1.VolumeSnapshot, expectedCount)
	processEvent := func(ctx *contexts.Context, snapshot *volumesnapshotv1.VolumeSnapshot) ([]*volumesnapshotv1.VolumeSnapshot, bool, error) {
		if isReadyGroupMember(snapshot, groupSnapshotName) {
			readyMembers[snapshot.Name] = snapshot
		} else {
			delete(readyMembers, snapshot.Name)
		}

		if len(readyMembers) >= expectedCount {
			return slices.Collect(maps.Values(readyMembers)), true, nil
		}
		return nil, false, nil
	}

	// An empty label selector matches every VolumeSnapshot in the namespace; processEvent filters to
	// this group's ready members.
	members, err = helpers.WaitForResourceConditionByLabel(ctx.Child(), opts.MaxWait(10*time.Minute), c.client.SnapshotV1().VolumeSnapshots(namespace), "", processEvent)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for %d members of group snapshot %q to become ready", expectedCount, helpers.FullNameStr(namespace, groupSnapshotName))
	}

	return members, nil
}

// isReadyGroupMember reports whether the VolumeSnapshot is a fully-reconciled member of the named group:
// it is owned by that VolumeGroupSnapshot, is ready to use, and has a restore size.
//
// Membership is determined by the ownerReference the snapshot-controller sets at member creation, not by
// Status.VolumeGroupSnapshotName: that status field is left unset when the controller can't resolve a
// default VolumeSnapshotClass for the member's driver (observed against the host-path driver when the
// cluster's default snapshot class targets a different driver), yet the member is otherwise ready and the
// ownerReference is always present.
func isReadyGroupMember(snapshot *volumesnapshotv1.VolumeSnapshot, groupSnapshotName string) bool {
	status := snapshot.Status
	if status == nil || status.ReadyToUse == nil || !*status.ReadyToUse || status.RestoreSize == nil {
		return false
	}

	for _, ownerRef := range snapshot.OwnerReferences {
		if ownerRef.Kind == VolumeGroupSnapshotKind && ownerRef.Name == groupSnapshotName {
			return true
		}
	}
	return false
}

func (c *Client) DeleteGroupSnapshot(ctx *contexts.Context, namespace, groupSnapshotName string) error {
	ctx.Log.With("name", groupSnapshotName).Info("Deleting group snapshot")

	err := c.client.GroupsnapshotV1().VolumeGroupSnapshots(namespace).Delete(ctx, groupSnapshotName, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete group snapshot %q", helpers.FullNameStr(namespace, groupSnapshotName))
}
