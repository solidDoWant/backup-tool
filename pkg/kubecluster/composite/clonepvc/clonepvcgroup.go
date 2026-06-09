package clonepvc

import (
	"time"

	"github.com/gravitational/trace"
	volumegroupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClonePVCGroupOptions struct {
	GroupSnapshotName      string // Override the name of the created VolumeGroupSnapshot. Defaults to a generated name when empty.
	SnapshotClass          string // Override the VolumeGroupSnapshotClass used to snapshot the source volumes. Defaults to the cluster default when empty.
	WaitForSnapshotTimeout helpers.MaxWaitTime
	DestStorageClassName   string // Override the storage class used for the created volumes. Must be compatible with the snapshots.
	ForceBind              bool   // Force the cloned PVCs to be bound immediately. This should be set if the storage class does not have `volumeBindingMode: Immediate` set, because the member snapshots are deleted when the group snapshot is deleted.
	ForceBindTimeout       helpers.MaxWaitTime
	CleanupTimeout         helpers.MaxWaitTime
}

type ClonePVCGroupResult struct {
	// GroupSnapshot is the created VolumeGroupSnapshot. The caller owns its lifecycle on success
	// (its creation time is the consistency point, and it must be deleted once the clones are no
	// longer needed - deleting it deletes the member snapshots).
	GroupSnapshot *volumegroupsnapshotv1.VolumeGroupSnapshot
	// ClonedPVCs maps each source PVC name to the PVC cloned from that member's snapshot.
	ClonedPVCs map[string]*corev1.PersistentVolumeClaim
}

// ClonePVCGroup atomically snapshots a label-selected group of volumes via a VolumeGroupSnapshot and
// clones each member into a new PVC. Callers are responsible for ensuring consistency of the source
// volumes. It is the group analog of ClonePVC and shares the same per-snapshot create-and-force-bind
// machinery.
//
// On success the created VolumeGroupSnapshot is returned (not deleted) so the caller can pin its
// creation instant and clean it up later; deleting it deletes the member snapshots, so ForceBind
// should be set unless the destination storage class binds immediately. On error everything created
// is cleaned up.
func (p *Provider) ClonePVCGroup(ctx *contexts.Context, namespace string, selector metav1.LabelSelector, opts ClonePVCGroupOptions) (result *ClonePVCGroupResult, err error) {
	ctx.Log.Info("Cloning PVC group")
	defer ctx.Log.Info("Finished cloning PVC group", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	// Resolve the selector to its source PVCs up front. This is the snapshot controller's own
	// definition of group membership (it lists PVCs by the same selector), so the count is exactly
	// how many member snapshots to expect, and a zero match is a misconfiguration.
	sourcePVCs, err := p.coreClient.ListPVCs(ctx.Child(), namespace, core.ListPVCsOptions{LabelSelector: selector})
	if err != nil {
		err = trace.Wrap(err, "failed to list PVCs matching the group selector in namespace %q", namespace)
		return
	}
	if len(sourcePVCs) == 0 {
		err = trace.Errorf("group selector matched no PVCs in namespace %q", namespace)
		return
	}
	expectedMemberCount := len(sourcePVCs)

	ctx.Log.Step().Info("Creating group snapshot of volumes")
	groupSnapshot, err := p.esClient.GroupSnapshotVolumes(ctx.Child(), namespace, selector, externalsnapshotter.GroupSnapshotOptions{
		Name:          opts.GroupSnapshotName,
		SnapshotClass: opts.SnapshotClass,
	})
	if err != nil {
		err = trace.Wrap(err, "failed to create group snapshot in namespace %q", namespace)
		return
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		// On success the caller owns the group snapshot's lifecycle.
		if err == nil {
			return nil
		}
		return p.esClient.DeleteGroupSnapshot(ctx, namespace, groupSnapshot.Name)
	}).WithErrMessage("failed to delete created group snapshot in namespace %q", namespace).WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	readyGroupSnapshot, err := p.esClient.WaitForReadyGroupSnapshot(ctx.Child(), namespace, groupSnapshot.Name, externalsnapshotter.WaitForReadyGroupSnapshotOpts{MaxWaitTime: opts.WaitForSnapshotTimeout})
	if err != nil {
		err = trace.Wrap(err, "failed to wait for group snapshot %q to become ready", helpers.FullName(groupSnapshot))
		return
	}

	members, err := p.esClient.WaitForReadyGroupSnapshotMembers(ctx.Child(), namespace, readyGroupSnapshot.Name, expectedMemberCount, externalsnapshotter.WaitForReadyGroupSnapshotMembersOpts{MaxWaitTime: opts.WaitForSnapshotTimeout})
	if err != nil {
		err = trace.Wrap(err, "failed to wait for members of group snapshot %q to become ready", helpers.FullName(readyGroupSnapshot))
		return
	}

	result = &ClonePVCGroupResult{
		GroupSnapshot: readyGroupSnapshot,
		ClonedPVCs:    make(map[string]*corev1.PersistentVolumeClaim, len(members)),
	}
	// On error, delete every clone created so far. Nil the result so callers don't act on a partial
	// set. The group snapshot is cleaned up by the deferred above.
	defer cleanup.To(func(ctx *contexts.Context) error {
		if err == nil {
			return nil
		}

		var cleanupErr error
		for _, clonedPvc := range result.ClonedPVCs {
			if deleteErr := p.coreClient.DeletePVC(ctx, namespace, clonedPvc.Name); deleteErr != nil {
				cleanupErr = trace.NewAggregate(cleanupErr, deleteErr)
			}
		}
		result = nil
		return cleanupErr
	}).WithErrMessage("failed to delete created volumes for group snapshot %q", helpers.FullName(readyGroupSnapshot)).WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	for _, member := range members {
		var clonedPvc *corev1.PersistentVolumeClaim
		clonedPvc, err = p.clonePVCFromMember(ctx.Child(), namespace, member, opts)
		if err != nil {
			err = trace.Wrap(err, "failed to clone member snapshot %q", helpers.FullName(member))
			return
		}

		sourcePVCName := *member.Spec.Source.PersistentVolumeClaimName
		result.ClonedPVCs[sourcePVCName] = clonedPvc
	}

	if !opts.ForceBind {
		return
	}

	ctx.Log.Step().Info("Forcing immediate bind of cloned PVCs")
	// The clones come from one atomic group and are consumed together, so they are already
	// co-schedulable and can share a single force-bind pod.
	clonePVCNames := lo.Map(lo.Values(result.ClonedPVCs), func(pvc *corev1.PersistentVolumeClaim, _ int) string {
		return pvc.Name
	})
	err = p.forceBindVolumes(ctx.Child(), namespace, clonePVCNames, forceBindVolumesOptions{
		WaitForReadyTimeout: opts.ForceBindTimeout,
		CleanupTimeout:      opts.CleanupTimeout,
	})
	if err != nil {
		err = trace.Wrap(err, "failed to force bind cloned volumes for group snapshot %q", helpers.FullName(readyGroupSnapshot))
		return
	}

	return
}

// clonePVCFromMember creates a PVC from a single member VolumeSnapshot, named after the member's
// source PVC. It does not force-bind; binding for the whole group is handled together.
func (p *Provider) clonePVCFromMember(ctx *contexts.Context, namespace string, member *volumesnapshotv1.VolumeSnapshot, opts ClonePVCGroupOptions) (*corev1.PersistentVolumeClaim, error) {
	if member.Spec.Source.PersistentVolumeClaimName == nil || *member.Spec.Source.PersistentVolumeClaimName == "" {
		return nil, trace.Errorf("member snapshot %q does not reference a source PVC", helpers.FullName(member))
	}
	sourcePVCName := *member.Spec.Source.PersistentVolumeClaimName

	ctx.Log.With("sourcePVC", sourcePVCName).Step().Info("Creating PVC from member snapshot", "snapshot", member.Name)

	clonedPvc, err := p.createPVCFromSnapshot(ctx.Child(), namespace, sourcePVCName, sourcePVCName, member, opts.DestStorageClassName)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create volume from member snapshot %q", helpers.FullName(member))
	}

	return clonedPvc, nil
}
