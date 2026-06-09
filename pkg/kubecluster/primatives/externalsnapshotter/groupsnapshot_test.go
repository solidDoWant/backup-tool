package externalsnapshotter

import (
	"sync"
	"testing"
	"time"

	volumegroupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
)

func TestGroupSnapshotVolumes(t *testing.T) {
	namespace := "default"
	selector := metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}

	tests := []struct {
		name                string
		opts                GroupSnapshotOptions
		simulateClientError bool
	}{
		{
			name: "successful snapshot with generated name",
		},
		{
			name: "successful snapshot with all options",
			opts: GroupSnapshotOptions{Name: "group-snapshot-name", SnapshotClass: "group-snapshot-class"},
		},
		{
			name:                "client error",
			simulateClientError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, mockES := createTestClient()

			if tt.simulateClientError {
				mockES.PrependReactor("create", "volumegroupsnapshots", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}

			ctx := th.NewTestContext()
			vol, err := client.GroupSnapshotVolumes(ctx, namespace, selector, tt.opts)
			if tt.simulateClientError {
				require.Error(t, err)
				require.Nil(t, vol)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, vol)
			require.Equal(t, &selector, vol.Spec.Source.Selector)
		})
	}
}

func TestWaitForReadyGroupSnapshot(t *testing.T) {
	groupSnapshotName := "test-group-snapshot"
	podNamespace := "test-ns"

	noStatusSnapshot := &volumegroupsnapshotv1.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      groupSnapshotName,
			Namespace: podNamespace,
		},
	}

	noReadyToUseSnapshot := noStatusSnapshot.DeepCopy()
	noReadyToUseSnapshot.Status = &volumegroupsnapshotv1.VolumeGroupSnapshotStatus{}

	notReadySnapshot := noReadyToUseSnapshot.DeepCopy()
	notReadySnapshot.Status.ReadyToUse = new(false)

	readySnapshot := notReadySnapshot.DeepCopy()
	readySnapshot.Status.ReadyToUse = new(true)

	tests := []struct {
		desc                string
		initialSnapshot     *volumegroupsnapshotv1.VolumeGroupSnapshot
		shouldError         bool
		afterStartedWaiting func(*testing.T, *contexts.Context, versioned.Interface)
	}{
		{
			desc:            "snapshot starts ready",
			initialSnapshot: readySnapshot,
		},
		{
			desc:            "snapshot has no status",
			initialSnapshot: noStatusSnapshot,
			shouldError:     true,
		},
		{
			desc:            "snapshot has no ready to use field",
			initialSnapshot: noReadyToUseSnapshot,
			shouldError:     true,
		},
		{
			desc:            "snapshot not ready",
			initialSnapshot: notReadySnapshot,
			shouldError:     true,
		},
		{
			desc:        "snapshot does not exist",
			shouldError: true,
		},
		{
			desc:            "snapshot becomes ready",
			initialSnapshot: noStatusSnapshot,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client versioned.Interface) {
				_, err := client.GroupsnapshotV1().VolumeGroupSnapshots(podNamespace).Update(ctx, readySnapshot, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockES := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialSnapshot != nil {
				_, err := mockES.GroupsnapshotV1().VolumeGroupSnapshots(podNamespace).Create(ctx, tt.initialSnapshot, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var snapshot *volumegroupsnapshotv1.VolumeGroupSnapshot
			wg.Add(1)
			go func() {
				snapshot, waitErr = c.WaitForReadyGroupSnapshot(ctx, podNamespace, groupSnapshotName, WaitForReadyGroupSnapshotOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, mockES)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, snapshot)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, snapshot)
		})
	}
}

func TestWaitForReadyGroupSnapshotMembers(t *testing.T) {
	namespace := "default"
	groupSnapshotName := "test-group-snapshot"
	size := resource.MustParse("1Gi")

	// member builds a VolumeSnapshot as the snapshot controller would: owned by its group via an
	// ownerReference (set at creation, independent of readiness), with ready adding the reconciled status
	// (ReadyToUse + restore size). Membership is keyed on the ownerReference, NOT Status.VolumeGroupSnapshotName,
	// which the controller does not reliably populate (the production bug behind isReadyGroupMember).
	member := func(name, pvcName, groupName string, ready bool) *volumesnapshotv1.VolumeSnapshot {
		snapshot := &volumesnapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: volumesnapshotv1.VolumeSnapshotSpec{
				Source: volumesnapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: new(pvcName)},
			},
		}
		if groupName != "" {
			snapshot.OwnerReferences = []metav1.OwnerReference{{
				Kind: VolumeGroupSnapshotKind,
				Name: groupName,
			}}
		}
		if ready {
			snapshot.Status = &volumesnapshotv1.VolumeSnapshotStatus{
				ReadyToUse:  new(true),
				RestoreSize: &size,
			}
		}
		return snapshot
	}

	tests := []struct {
		desc                string
		expectedCount       int
		initialSnapshots    []*volumesnapshotv1.VolumeSnapshot
		afterStartedWaiting func(*testing.T, *contexts.Context, versioned.Interface)
		expectedMemberPVCs  []string
		shouldError         bool
	}{
		{
			desc:               "members already ready",
			expectedCount:      2,
			initialSnapshots:   []*volumesnapshotv1.VolumeSnapshot{member("snap-a", "pvc-a", groupSnapshotName, true), member("snap-b", "pvc-b", groupSnapshotName, true)},
			expectedMemberPVCs: []string{"pvc-a", "pvc-b"},
		},
		{
			desc:          "ignores other groups and not-ready members",
			expectedCount: 2,
			initialSnapshots: []*volumesnapshotv1.VolumeSnapshot{
				member("snap-a", "pvc-a", groupSnapshotName, true),
				member("snap-b", "pvc-b", groupSnapshotName, true),
				member("other", "pvc-c", "other-group", true),
				member("pending", "pvc-d", groupSnapshotName, false),
			},
			expectedMemberPVCs: []string{"pvc-a", "pvc-b"},
		},
		{
			desc:             "members become ready while waiting",
			expectedCount:    2,
			initialSnapshots: []*volumesnapshotv1.VolumeSnapshot{member("snap-a", "pvc-a", groupSnapshotName, false), member("snap-b", "pvc-b", groupSnapshotName, false)},
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client versioned.Interface) {
				for _, m := range []*volumesnapshotv1.VolumeSnapshot{member("snap-a", "pvc-a", groupSnapshotName, true), member("snap-b", "pvc-b", groupSnapshotName, true)} {
					_, err := client.SnapshotV1().VolumeSnapshots(namespace).Update(ctx, m, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
			},
			expectedMemberPVCs: []string{"pvc-a", "pvc-b"},
		},
		{
			desc:             "times out when not enough members become ready",
			expectedCount:    2,
			initialSnapshots: []*volumesnapshotv1.VolumeSnapshot{member("snap-a", "pvc-a", groupSnapshotName, true)},
			shouldError:      true,
		},
		{
			desc:          "non-positive expected count errors",
			expectedCount: 0,
			shouldError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, mockES := createTestClient()
			ctx := th.NewTestContext()

			for _, snapshot := range tt.initialSnapshots {
				_, err := mockES.SnapshotV1().VolumeSnapshots(namespace).Create(ctx, snapshot, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var members []*volumesnapshotv1.VolumeSnapshot
			var waitErr error
			wg.Add(1)
			go func() {
				members, waitErr = client.WaitForReadyGroupSnapshotMembers(ctx, namespace, groupSnapshotName, tt.expectedCount, WaitForReadyGroupSnapshotMembersOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that the watcher has been set up
				tt.afterStartedWaiting(t, ctx, mockES)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, members)
				return
			}
			require.NoError(t, waitErr)

			memberPVCs := make([]string, 0, len(members))
			for _, m := range members {
				memberPVCs = append(memberPVCs, *m.Spec.Source.PersistentVolumeClaimName)
			}
			assert.ElementsMatch(t, tt.expectedMemberPVCs, memberPVCs)
		})
	}
}

func TestDeleteGroupSnapshot(t *testing.T) {
	groupSnapshotName := "test-group-snapshot"
	namespace := "default"

	tests := []struct {
		name            string
		initialSnapshot *volumegroupsnapshotv1.VolumeGroupSnapshot
		shouldErr       bool
	}{
		{
			name: "successful delete",
			initialSnapshot: &volumegroupsnapshotv1.VolumeGroupSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      groupSnapshotName,
					Namespace: namespace,
				},
			},
		},
		{
			name:      "snapshot not found",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, mockES := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialSnapshot != nil {
				_, err := mockES.GroupsnapshotV1().VolumeGroupSnapshots(namespace).Create(ctx, tt.initialSnapshot, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteGroupSnapshot(ctx, namespace, groupSnapshotName)
			if tt.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
