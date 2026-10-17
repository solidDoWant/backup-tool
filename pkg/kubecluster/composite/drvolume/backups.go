package drvolume

import (
	"regexp"
	"time"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

// BackupSnapshot is a backup snapshot of a DR volume.
type BackupSnapshot struct {
	Snapshot *volumesnapshotv1.VolumeSnapshot
	// BackupName is the name of the backup that the snapshot belongs to.
	BackupName string
	// Legacy is set for snapshots taken before backup snapshots were labeled. These are identified by
	// their source DR volume and their name instead of by their labels.
	Legacy bool
}

// Time returns when the snapshot was taken, falling back to when the resource was created if the
// snapshot controller hasn't reported it.
func (bs BackupSnapshot) Time() time.Time {
	if status := bs.Snapshot.Status; status != nil && status.CreationTime != nil {
		return status.CreationTime.Time
	}

	return bs.Snapshot.CreationTimestamp.Time
}

// IsReady returns true if the snapshot can be restored from.
func (bs BackupSnapshot) IsReady() bool {
	status := bs.Snapshot.Status
	return status != nil && status.ReadyToUse != nil && *status.ReadyToUse
}

// IsDeleting returns true if the snapshot has already been marked for deletion.
func (bs BackupSnapshot) IsDeleting() bool {
	return bs.Snapshot.DeletionTimestamp != nil
}

type ListBackupSnapshotsOptions struct {
	// Selector limits the returned backup snapshots. Labeled snapshots are matched against their own
	// labels, and legacy (unlabeled) snapshots are matched against the labels of their DR volume. The
	// zero value matches every backup snapshot.
	Selector metav1.LabelSelector
}

// Backup snapshots taken before snapshots were labeled are named helpers.CleanName(<DR volume name> + "-" +
// <RFC 3339 UTC start time>) (see disasterrecovery.DREvent.GetFullName). This matches the cleaned time
// suffix, e.g. "-2006-01-02t15-04-05z".
var legacyBackupSnapshotTimeSuffix = regexp.MustCompile(`^-\d{4}-\d{2}-\d{2}t\d{2}-\d{2}-\d{2}z$`)

// isLegacyBackupSnapshotName returns true if snapshotName is the name that an older version of the tool
// gave to a backup snapshot of the DR volume named drVolumeName.
func isLegacyBackupSnapshotName(snapshotName, drVolumeName string) bool {
	prefix := helpers.CleanName(drVolumeName)
	if len(snapshotName) <= len(prefix) || snapshotName[:len(prefix)] != prefix {
		return false
	}

	return legacyBackupSnapshotTimeSuffix.MatchString(snapshotName[len(prefix):])
}

// backupSelector returns the selector that matches backup-tool backup resources, narrowed by the user's selector.
func backupSelector(userSelector metav1.LabelSelector) metav1.LabelSelector {
	selector := *userSelector.DeepCopy()
	selector.MatchExpressions = append(selector.MatchExpressions,
		metav1.LabelSelectorRequirement{Key: LabelManagedBy, Operator: metav1.LabelSelectorOpIn, Values: []string{LabelManagedByValue}},
		metav1.LabelSelectorRequirement{Key: LabelBackupName, Operator: metav1.LabelSelectorOpExists},
	)

	return selector
}

// ListBackupSnapshots returns the backup snapshots in the namespace, or in all namespaces when the namespace
// is empty. Backup snapshots are found by their tool-owned labels. Snapshots taken by versions of the tool that
// didn't label them are found by being a snapshot of a (labeled) DR volume with a backup snapshot name. Legacy
// snapshots of a DR volume that no longer exists cannot be told apart from other snapshots, so they are not
// returned.
func (p *Provider) ListBackupSnapshots(ctx *contexts.Context, namespace string, opts ListBackupSnapshotsOptions) ([]BackupSnapshot, error) {
	ctx.Log.With("namespace", namespace).Info("Listing backup snapshots")

	metaSelector := backupSelector(opts.Selector)
	selector, err := metav1.LabelSelectorAsSelector(&metaSelector)
	if err != nil {
		return nil, trace.Wrap(err, "failed to convert label selector")
	}

	ctx.Log.Step().Info("Listing DR volumes")
	drVolumes, err := p.core().ListPVCs(ctx.Child(), namespace, core.ListPVCsOptions{LabelSelector: metaSelector})
	if err != nil {
		return nil, trace.Wrap(err, "failed to list DR volumes")
	}

	drVolumeBackupNames := make(map[types.NamespacedName]string, len(drVolumes))
	for _, drVolume := range drVolumes {
		drVolumeBackupNames[types.NamespacedName{Namespace: drVolume.Namespace, Name: drVolume.Name}] = drVolume.Labels[LabelBackupName]
	}

	// Legacy snapshots have no labels to select on, so every snapshot must be listed
	ctx.Log.Step().Info("Listing snapshots")
	snapshots, err := p.es().ListSnapshots(ctx.Child(), namespace, externalsnapshotter.ListSnapshotsOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to list snapshots")
	}

	var backupSnapshots []BackupSnapshot
	for i := range snapshots {
		snapshot := &snapshots[i]

		if _, ok := snapshot.Labels[LabelManagedBy]; ok {
			if selector.Matches(labels.Set(snapshot.Labels)) {
				backupSnapshots = append(backupSnapshots, BackupSnapshot{Snapshot: snapshot, BackupName: snapshot.Labels[LabelBackupName]})
			}
			continue
		}

		sourcePVC := snapshot.Spec.Source.PersistentVolumeClaimName
		if sourcePVC == nil {
			continue
		}

		backupName, ok := drVolumeBackupNames[types.NamespacedName{Namespace: snapshot.Namespace, Name: *sourcePVC}]
		if !ok || !isLegacyBackupSnapshotName(snapshot.Name, *sourcePVC) {
			continue
		}

		backupSnapshots = append(backupSnapshots, BackupSnapshot{Snapshot: snapshot, BackupName: backupName, Legacy: true})
	}

	ctx.Log.Debug("Found backup snapshots", "count", len(backupSnapshots))
	return backupSnapshots, nil
}
