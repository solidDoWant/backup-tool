package drvolume

import (
	"testing"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsLegacyBackupSnapshotName(t *testing.T) {
	tests := []struct {
		desc         string
		snapshotName string
		drVolumeName string
		want         bool
	}{
		{
			desc:         "legacy backup snapshot",
			snapshotName: "vaultwarden-2026-01-02t03-04-05z",
			drVolumeName: "vaultwarden",
			want:         true,
		},
		{
			desc:         "DR volume name is cleaned",
			snapshotName: "my-backup-2026-01-02t03-04-05z",
			drVolumeName: "my.backup",
			want:         true,
		},
		{
			desc:         "different DR volume",
			snapshotName: "vaultwarden-2026-01-02t03-04-05z",
			drVolumeName: "teleport",
		},
		{
			desc:         "DR volume name is only a prefix of the backup name",
			snapshotName: "vaultwarden-extra-2026-01-02t03-04-05z",
			drVolumeName: "vaultwarden",
		},
		{
			desc:         "extra suffix",
			snapshotName: "vaultwarden-2026-01-02t03-04-05z-copy",
			drVolumeName: "vaultwarden",
		},
		{
			desc:         "no timestamp",
			snapshotName: "vaultwarden-manual",
			drVolumeName: "vaultwarden",
		},
		{
			desc:         "just the DR volume name",
			snapshotName: "vaultwarden",
			drVolumeName: "vaultwarden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			assert.Equal(t, tt.want, isLegacyBackupSnapshotName(tt.snapshotName, tt.drVolumeName))
		})
	}
}

func TestBackupSnapshotTime(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	taken := created.Add(-time.Minute)

	withoutStatus := BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(created)}}}
	assert.Equal(t, created, withoutStatus.Time())

	withStatus := BackupSnapshot{Snapshot: withoutStatus.Snapshot.DeepCopy()}
	withStatus.Snapshot.Status = &volumesnapshotv1.VolumeSnapshotStatus{CreationTime: new(metav1.NewTime(taken))}
	assert.Equal(t, taken, withStatus.Time())
}

func TestBackupSnapshotIsReady(t *testing.T) {
	assert.False(t, BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{}}.IsReady())
	assert.False(t, BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{Status: &volumesnapshotv1.VolumeSnapshotStatus{}}}.IsReady())
	assert.False(t, BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{Status: &volumesnapshotv1.VolumeSnapshotStatus{ReadyToUse: new(false)}}}.IsReady())
	assert.True(t, BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{Status: &volumesnapshotv1.VolumeSnapshotStatus{ReadyToUse: new(true)}}}.IsReady())
}

func TestBackupSnapshotIsDeleting(t *testing.T) {
	assert.False(t, BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{}}.IsDeleting())
	assert.True(t, BackupSnapshot{Snapshot: &volumesnapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: new(metav1.Now())}}}.IsDeleting())
}

func TestListBackupSnapshots(t *testing.T) {
	namespace := "test-ns"
	otherNamespace := "other-ns"

	drVolume := func(namespace, name string) corev1.PersistentVolumeClaim {
		return corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: ToolOwnedLabels(name, "vaultwarden")},
		}
	}

	snapshot := func(namespace, name, sourcePVC string, labels map[string]string) volumesnapshotv1.VolumeSnapshot {
		snapshot := volumesnapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		}
		if sourcePVC != "" {
			snapshot.Spec.Source.PersistentVolumeClaimName = new(sourcePVC)
		}
		return snapshot
	}

	labeled := func(backupName string, extra map[string]string) map[string]string {
		return mergeLabels(extra, ToolOwnedLabels(backupName, "vaultwarden"))
	}

	snapshots := []volumesnapshotv1.VolumeSnapshot{
		snapshot(namespace, "labeled-2026-01-02t03-04-05z", "labeled", labeled("labeled", nil)),
		snapshot(namespace, "selected-2026-01-02t03-04-05z", "selected", labeled("selected", map[string]string{"tier": "gold"})),
		snapshot(namespace, "vw-2026-01-02t03-04-05z", "vw", nil),
		// Not backups
		snapshot(namespace, "vw-manual", "vw", nil),
		snapshot(namespace, "data-2026-01-02t03-04-05z", "data", nil),
		snapshot(namespace, "from-content", "", nil),
		snapshot(namespace, "other-tool-2026-01-02t03-04-05z", "vw", map[string]string{LabelManagedBy: "other-tool"}),
		snapshot(otherNamespace, "vw-2026-01-02t03-04-05z", "vw", nil),
	}

	tests := []struct {
		desc                 string
		opts                 ListBackupSnapshotsOptions
		drVolumes            []corev1.PersistentVolumeClaim
		simulateListPVCError bool
		simulateListError    bool
		expected             map[string]BackupSnapshot
	}{
		{
			desc:      "labeled and legacy snapshots",
			drVolumes: []corev1.PersistentVolumeClaim{drVolume(namespace, "vw")},
			expected: map[string]BackupSnapshot{
				"labeled-2026-01-02t03-04-05z":  {BackupName: "labeled"},
				"selected-2026-01-02t03-04-05z": {BackupName: "selected"},
				"vw-2026-01-02t03-04-05z":       {BackupName: "vw", Legacy: true},
			},
		},
		{
			desc: "legacy snapshots require a DR volume",
			expected: map[string]BackupSnapshot{
				"labeled-2026-01-02t03-04-05z":  {BackupName: "labeled"},
				"selected-2026-01-02t03-04-05z": {BackupName: "selected"},
			},
		},
		{
			desc:      "user selector",
			opts:      ListBackupSnapshotsOptions{Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}}},
			drVolumes: nil, // The API server filters the DR volumes by the selector
			expected: map[string]BackupSnapshot{
				"selected-2026-01-02t03-04-05z": {BackupName: "selected"},
			},
		},
		{
			desc:                 "list DR volumes error",
			simulateListPVCError: true,
		},
		{
			desc:              "list snapshots error",
			simulateListError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			p := newMockProvider(t)

			p.coreClient.EXPECT().ListPVCs(mock.Anything, namespace, mock.Anything).
				RunAndReturn(func(calledCtx *contexts.Context, _ string, opts core.ListPVCsOptions) ([]corev1.PersistentVolumeClaim, error) {
					assert.True(t, calledCtx.IsChildOf(ctx))
					assert.Equal(t, backupSelector(tt.opts.Selector), opts.LabelSelector)
					return th.ErrOr1Val(tt.drVolumes, tt.simulateListPVCError)
				})

			if !tt.simulateListPVCError {
				p.esClient.EXPECT().ListSnapshots(mock.Anything, namespace, externalsnapshotter.ListSnapshotsOptions{}).
					RunAndReturn(func(calledCtx *contexts.Context, _ string, _ externalsnapshotter.ListSnapshotsOptions) ([]volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						// The API server would only return snapshots in the namespace, but this checks that
						// legacy snapshots are matched to DR volumes by namespace as well as name
						return th.ErrOr1Val(snapshots, tt.simulateListError)
					})
			}

			backupSnapshots, err := p.ListBackupSnapshots(ctx, namespace, tt.opts)
			if tt.simulateListPVCError || tt.simulateListError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			actual := make(map[string]BackupSnapshot, len(backupSnapshots))
			for _, backupSnapshot := range backupSnapshots {
				require.NotNil(t, backupSnapshot.Snapshot)
				name := backupSnapshot.Snapshot.Name
				backupSnapshot.Snapshot = nil
				actual[name] = backupSnapshot
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestBackupSelector(t *testing.T) {
	userSelector := metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}}
	selector := backupSelector(userSelector)

	assert.Equal(t, userSelector.MatchLabels, selector.MatchLabels)
	assert.Len(t, selector.MatchExpressions, 2)
	assert.Empty(t, userSelector.MatchExpressions, "the user's selector should not be modified")
}
