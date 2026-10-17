package externalsnapshotter

import (
	"sync"
	"testing"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
)

func TestSnapshotVolume(t *testing.T) {
	pvcName := "test-pvc"
	namespace := "default"

	tests := []struct {
		name                string
		opts                SnapshotVolumeOptions
		simulateClientError bool
	}{
		{
			name: "successful snapshot with generated name",
		},
		{
			name: "successful snapshot with all options",
			opts: SnapshotVolumeOptions{Name: "snapshot-name", SnapshotClass: "snapshot-class", Labels: map[string]string{"key": "value"}},
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
				mockES.PrependReactor("create", "volumesnapshots", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}

			ctx := th.NewTestContext()
			vol, err := client.SnapshotVolume(ctx, namespace, pvcName, tt.opts)
			if tt.simulateClientError {
				require.Error(t, err)
				require.Nil(t, vol)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, vol)
			assert.Equal(t, tt.opts.Labels, vol.Labels)
		})
	}
}

func TestWaitForReadySnapshot(t *testing.T) {
	snapshotName := "test-snapshot"
	podNamespace := "test-ns"

	noStatusSnapshot := &volumesnapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: podNamespace,
		},
	}

	noReadyToUseSnapshot := noStatusSnapshot.DeepCopy()
	noReadyToUseSnapshot.Status = &volumesnapshotv1.VolumeSnapshotStatus{}

	notReadySnapshot := noReadyToUseSnapshot.DeepCopy()
	notReadySnapshot.Status.ReadyToUse = new(false)

	readySnapshot := notReadySnapshot.DeepCopy()
	readySnapshot.Status.ReadyToUse = new(true)

	failedSnapshot := noReadyToUseSnapshot.DeepCopy()
	failedSnapshot.Status.Error = &volumesnapshotv1.VolumeSnapshotError{
		Message: new("snapshot failed"),
	}

	tests := []struct {
		desc                string
		initialSnapshot     *volumesnapshotv1.VolumeSnapshot
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
				_, err := client.SnapshotV1().VolumeSnapshots(podNamespace).Update(ctx, readySnapshot, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:            "snapshot errors after creation",
			initialSnapshot: noStatusSnapshot,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client versioned.Interface) {
				_, err := client.SnapshotV1().VolumeSnapshots(podNamespace).Update(ctx, failedSnapshot, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockES := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialSnapshot != nil {
				_, err := mockES.SnapshotV1().VolumeSnapshots(podNamespace).Create(ctx, tt.initialSnapshot, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var snapshot *volumesnapshotv1.VolumeSnapshot
			wg.Add(1)
			go func() {
				snapshot, waitErr = c.WaitForReadySnapshot(ctx, podNamespace, snapshotName, WaitForReadySnapshotOpts{MaxWaitTime: helpers.ShortWaitTime})
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

func TestListSnapshots(t *testing.T) {
	snapshotWithLabels := func(namespace, name string, labels map[string]string) *volumesnapshotv1.VolumeSnapshot {
		return &volumesnapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		}
	}

	initialSnapshots := []*volumesnapshotv1.VolumeSnapshot{
		snapshotWithLabels("ns-a", "app-a", map[string]string{"app": "test"}),
		snapshotWithLabels("ns-a", "other", map[string]string{"app": "other"}),
		snapshotWithLabels("ns-b", "app-b", map[string]string{"app": "test"}),
	}

	tests := []struct {
		name                string
		namespace           string
		opts                ListSnapshotsOptions
		expectedNames       []string
		simulateClientError bool
	}{
		{
			name:          "filters by label selector",
			namespace:     "ns-a",
			opts:          ListSnapshotsOptions{LabelSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}},
			expectedNames: []string{"app-a"},
		},
		{
			name:          "empty selector matches all in namespace",
			namespace:     "ns-a",
			expectedNames: []string{"app-a", "other"},
		},
		{
			name:          "empty namespace matches all namespaces",
			opts:          ListSnapshotsOptions{LabelSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}},
			expectedNames: []string{"app-a", "app-b"},
		},
		{
			name:                "client error",
			namespace:           "ns-a",
			simulateClientError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, mockES := createTestClient()
			ctx := th.NewTestContext()

			for _, snapshot := range initialSnapshots {
				_, err := mockES.SnapshotV1().VolumeSnapshots(snapshot.Namespace).Create(ctx, snapshot, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.simulateClientError {
				mockES.PrependReactor("list", "volumesnapshots", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}

			snapshots, err := client.ListSnapshots(ctx, tt.namespace, tt.opts)
			if tt.simulateClientError {
				require.Error(t, err)
				require.Nil(t, snapshots)
				return
			}
			require.NoError(t, err)

			names := make([]string, 0, len(snapshots))
			for _, snapshot := range snapshots {
				names = append(names, snapshot.Name)
			}
			require.ElementsMatch(t, tt.expectedNames, names)
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	snapshotName := "test-snapshot"
	namespace := "default"

	tests := []struct {
		name            string
		initialSnapshot *volumesnapshotv1.VolumeSnapshot
		shouldErr       bool
	}{
		{
			name: "successful delete",
			initialSnapshot: &volumesnapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      snapshotName,
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
				_, err := mockES.SnapshotV1().VolumeSnapshots(namespace).Create(ctx, tt.initialSnapshot, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteSnapshot(ctx, namespace, snapshotName)
			if tt.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
