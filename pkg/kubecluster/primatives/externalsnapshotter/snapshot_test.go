package externalsnapshotter

import (
	"context"
	"sync"
	"testing"
	"time"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
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
			name: "successful snapshot with provided name",
			opts: SnapshotVolumeOptions{Name: "snapshot-name"},
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

			vol, err := client.SnapshotVolume(context.Background(), namespace, pvcName, tt.opts)
			if tt.simulateClientError {
				require.Error(t, err)
				require.Nil(t, vol)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, vol)
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
	notReadySnapshot.Status.ReadyToUse = ptr.To(false)

	readySnapshot := notReadySnapshot.DeepCopy()
	readySnapshot.Status.ReadyToUse = ptr.To(true)

	failedSnapshot := noReadyToUseSnapshot.DeepCopy()
	failedSnapshot.Status.Error = &volumesnapshotv1.VolumeSnapshotError{
		Message: ptr.To("snapshot failed"),
	}

	tests := []struct {
		desc                string
		initialSnapshot     *volumesnapshotv1.VolumeSnapshot
		shouldError         bool
		afterStartedWaiting func(*testing.T, context.Context, versioned.Interface)
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
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.SnapshotV1().VolumeSnapshots(podNamespace).Update(ctx, readySnapshot, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:            "snapshot errors after creation",
			initialSnapshot: noStatusSnapshot,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.SnapshotV1().VolumeSnapshots(podNamespace).Update(ctx, failedSnapshot, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			c, mockES := createTestClient()
			ctx := context.Background()

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
			ctx := context.Background()

			if tt.initialSnapshot != nil {
				mockES.SnapshotV1().VolumeSnapshots(namespace).Create(ctx, tt.initialSnapshot, metav1.CreateOptions{})
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
