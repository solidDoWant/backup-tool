package disasterrecovery

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/solidDoWant/backup-tool/pkg/retention"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestBackupCleanupConfigValidate(t *testing.T) {
	validRetention := retention.Policy{{Schedule: "@daily", Keep: 14}}

	tests := []struct {
		desc    string
		config  BackupCleanupConfig
		wantErr bool
	}{
		{
			desc:   "minimal",
			config: BackupCleanupConfig{Retention: validRetention},
		},
		{
			desc: "all fields",
			config: BackupCleanupConfig{
				Namespace: "ns",
				Selector:  metav1.LabelSelector{MatchLabels: map[string]string{"backup-tool.home.arpa/dr-type": "generic"}},
				Retention: validRetention,
				Timezone:  "America/Chicago",
				DryRun:    true,
			},
		},
		{
			desc:    "no retention rules",
			wantErr: true,
		},
		{
			desc:    "invalid timezone",
			config:  BackupCleanupConfig{Retention: validRetention, Timezone: "Not/AZone"},
			wantErr: true,
		},
		{
			desc: "invalid selector",
			config: BackupCleanupConfig{
				Retention: validRetention,
				Selector:  metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "key", Operator: "bad"}}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewBackupCleanup(t *testing.T) {
	client := kubecluster.NewMockClientInterface(t)
	assert.Equal(t, client, NewBackupCleanup(client).kubeClusterClient)
}

func TestBackupCleanupRun(t *testing.T) {
	namespace := "test-ns"
	day := time.Date(2026, 1, 10, 0, 5, 0, 0, time.UTC)

	backupSnapshot := func(backupName string, daysAgo int, ready bool) drvolume.BackupSnapshot {
		return drvolume.BackupSnapshot{
			BackupName: backupName,
			Snapshot: &volumesnapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-%d", backupName, daysAgo),
				},
				Status: &volumesnapshotv1.VolumeSnapshotStatus{
					CreationTime: new(metav1.NewTime(day.AddDate(0, 0, -daysAgo))),
					ReadyToUse:   new(ready),
				},
			},
		}
	}

	// Two backups with five nightly snapshots each, plus some snapshots that should be ignored
	var backupSnapshots []drvolume.BackupSnapshot
	for daysAgo := range 5 {
		backupSnapshots = append(backupSnapshots, backupSnapshot("a", daysAgo, true), backupSnapshot("b", daysAgo, true))
	}
	notReady := backupSnapshot("a", -1, false) // An in-progress backup, newer than every other snapshot
	deleting := backupSnapshot("b", 10, true)
	deleting.Snapshot.DeletionTimestamp = new(metav1.Now())
	legacy := backupSnapshot("b", 20, true)
	legacy.Legacy = true
	backupSnapshots = append(backupSnapshots, notReady, deleting, legacy)

	// Keeping the last two days of each backup deletes everything else that isn't ignored
	policy := retention.Policy{{Schedule: "@daily", Keep: 2}}
	expectedDeletions := []string{"a-2", "a-3", "a-4", "b-2", "b-3", "b-4", "b-20"}

	tests := []struct {
		desc                   string
		dryRun                 bool
		simulateListError      bool
		deleteErrors           map[string]error
		wantErr                bool
		expectedDeleteAttempts []string
	}{
		{
			desc:                   "deletes snapshots that are not kept",
			expectedDeleteAttempts: expectedDeletions,
		},
		{
			desc:   "dry run",
			dryRun: true,
		},
		{
			desc:              "list error",
			simulateListError: true,
			wantErr:           true,
		},
		{
			desc: "continues after delete errors",
			deleteErrors: map[string]error{
				"a-3": assert.AnError,
			},
			wantErr:                true,
			expectedDeleteAttempts: expectedDeletions,
		},
		{
			desc: "already deleted snapshots are not errors",
			deleteErrors: map[string]error{
				"a-3": trace.Wrap(apierrors.NewNotFound(schema.GroupResource{Group: "snapshot.storage.k8s.io", Resource: "volumesnapshots"}, "a-3")),
			},
			expectedDeleteAttempts: expectedDeletions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			mockClient := kubecluster.NewMockClientInterface(t)
			mockES := externalsnapshotter.NewMockClientInterface(t)
			mockClient.EXPECT().ES().Return(mockES).Maybe()

			selector := metav1.LabelSelector{MatchLabels: map[string]string{"key": "value"}}
			mockClient.EXPECT().ListBackupSnapshots(mock.Anything, namespace, drvolume.ListBackupSnapshotsOptions{Selector: selector}).
				RunAndReturn(func(calledCtx *contexts.Context, _ string, _ drvolume.ListBackupSnapshotsOptions) ([]drvolume.BackupSnapshot, error) {
					assert.True(t, calledCtx.IsChildOf(ctx))
					return th.ErrOr1Val(backupSnapshots, tt.simulateListError)
				})

			var mu sync.Mutex
			var deleteAttempts []string
			mockES.EXPECT().DeleteSnapshot(mock.Anything, namespace, mock.Anything).
				RunAndReturn(func(calledCtx *contexts.Context, _, name string) error {
					assert.True(t, calledCtx.IsChildOf(ctx))
					mu.Lock()
					defer mu.Unlock()
					deleteAttempts = append(deleteAttempts, name)
					return tt.deleteErrors[name]
				}).Maybe()

			err := NewBackupCleanup(mockClient).Run(ctx, BackupCleanupConfig{
				Namespace: namespace,
				Selector:  selector,
				Retention: policy,
				DryRun:    tt.dryRun,
			})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.ElementsMatch(t, tt.expectedDeleteAttempts, deleteAttempts)
		})
	}
}

func TestBackupCleanupRunDeletesOldestFirst(t *testing.T) {
	ctx := th.NewTestContext()
	mockClient := kubecluster.NewMockClientInterface(t)
	mockES := externalsnapshotter.NewMockClientInterface(t)
	mockClient.EXPECT().ES().Return(mockES)

	var backupSnapshots []drvolume.BackupSnapshot
	for daysAgo := range 4 {
		backupSnapshots = append(backupSnapshots, drvolume.BackupSnapshot{
			BackupName: "backup",
			Snapshot: &volumesnapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("backup-%d", daysAgo)},
				Status: &volumesnapshotv1.VolumeSnapshotStatus{
					CreationTime: new(metav1.NewTime(time.Now().AddDate(0, 0, -daysAgo))),
					ReadyToUse:   new(true),
				},
			},
		})
	}

	mockClient.EXPECT().ListBackupSnapshots(mock.Anything, "", mock.Anything).Return(backupSnapshots, nil)

	var deleteAttempts []string
	mockES.EXPECT().DeleteSnapshot(mock.Anything, "", mock.Anything).
		RunAndReturn(func(_ *contexts.Context, _, name string) error {
			deleteAttempts = append(deleteAttempts, name)
			return nil
		})

	require.NoError(t, NewBackupCleanup(mockClient).Run(ctx, BackupCleanupConfig{Retention: retention.Policy{{Schedule: "* * * * *", Keep: 1}}}))
	assert.Equal(t, []string{"backup-3", "backup-2", "backup-1"}, deleteAttempts)
}
