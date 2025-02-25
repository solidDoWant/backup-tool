package drvolume

import (
	"testing"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLookupCNPGClusterSize(t *testing.T) {
	namespace := "test-ns"
	clusterName := "test-cluster"

	tests := []struct {
		desc                    string
		size                    string
		simulateGetClusterError bool
		expectedQuantity        resource.Quantity
		wantErr                 bool
	}{
		{
			desc:             "basic size lookup",
			size:             "10Gi",
			expectedQuantity: resource.MustParse("10Gi"),
		},
		{
			desc:                    "cluster get error",
			simulateGetClusterError: true,
			wantErr:                 true,
		},
		{
			desc:    "invalid size format",
			size:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			p := newMockProvider(t)

			p.cnpgClient.EXPECT().GetCluster(mock.Anything, namespace, clusterName).
				RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*apiv1.Cluster, error) {
					assert.True(t, calledCtx.IsChildOf(ctx))

					cluster := &apiv1.Cluster{
						Spec: apiv1.ClusterSpec{
							StorageConfiguration: apiv1.StorageConfiguration{
								Size: tt.size,
							},
						},
					}
					return th.ErrOr1Val(cluster, tt.simulateGetClusterError)
				})

			actualSize, err := p.lookupCNPGClusterSize(ctx, namespace, clusterName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.True(t, actualSize.IsZero())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedQuantity, actualSize)
		})
	}
}

func TestLookupCNPGClustersSize(t *testing.T) {
	namespace := "test-ns"
	clusters := map[string]*apiv1.Cluster{
		"cluster-1": {
			Spec: apiv1.ClusterSpec{
				StorageConfiguration: apiv1.StorageConfiguration{
					Size: "1Gi",
				},
			},
		},
		"cluster-2": {
			Spec: apiv1.ClusterSpec{
				StorageConfiguration: apiv1.StorageConfiguration{
					Size: "2Gi",
				},
			},
		},
	}

	ctx := th.NewTestContext()
	p := newMockProvider(t)
	p.cnpgClient.EXPECT().GetCluster(mock.Anything, namespace, "cluster-1").Return(clusters["cluster-1"], nil)
	p.cnpgClient.EXPECT().GetCluster(mock.Anything, namespace, "cluster-2").Return(clusters["cluster-2"], nil)

	actualSize, err := p.lookupCNPGClustersSize(ctx, namespace, lo.Keys(clusters))
	assert.NoError(t, err)
	assert.Equal(t, "3Gi", actualSize.String())
}

func TestEnsureExists(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"
	storageClass := "test-storage-class"
	clusterName := "test-cluster"
	configuredSize := resource.MustParse("1Gi")

	existsOpts := DRVolumeCreateOptions{
		VolumeStorageClass: storageClass,
		CNPGClusterNames:   []string{clusterName},
	}
	createOpts := core.CreatePVCOptions{
		StorageClassName: storageClass,
	}

	t.Run("uses configured size when set", func(t *testing.T) {
		ctx := th.NewTestContext()
		p := newMockProvider(t)

		vol := &corev1.PersistentVolumeClaim{}

		p.coreClient.EXPECT().EnsurePVCExists(mock.Anything, namespace, pvcName, configuredSize, createOpts).Return(vol, nil)

		drv, err := p.NewDRVolume(ctx, namespace, pvcName, configuredSize, existsOpts)
		assert.NoError(t, err)
		require.NotNil(t, drv)
		assert.Equal(t, vol, drv.(*DRVolume).pvc)
	})

	t.Run("uses cluster size when configured size is not set", func(t *testing.T) {
		ctx := th.NewTestContext()
		p := newMockProvider(t)

		vol := &corev1.PersistentVolumeClaim{}

		p.cnpgClient.EXPECT().GetCluster(mock.Anything, namespace, clusterName).Return(&apiv1.Cluster{
			Spec: apiv1.ClusterSpec{
				StorageConfiguration: apiv1.StorageConfiguration{
					Size: "2Gi",
				},
			},
		}, nil)
		p.coreClient.EXPECT().EnsurePVCExists(mock.Anything, namespace, pvcName, mock.Anything, createOpts).
			RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
				assert.Equal(t, "4Gi", size.String())

				return vol, nil
			})

		drv, err := p.NewDRVolume(ctx, namespace, pvcName, resource.Quantity{}, existsOpts)
		assert.NoError(t, err)
		require.NotNil(t, drv)
		assert.Equal(t, vol, drv.(*DRVolume).pvc)
	})
}

func TestSnapshotAndWaitReady(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"
	snapshotName := "test-snapshot"
	opts := DRVolumeSnapshotAndWaitOptions{
		SnapshotClass: "test-snapshot-class",
		ReadyTimeout:  helpers.ShortWaitTime,
	}

	t.Run("basic snapshot", func(t *testing.T) {
		ctx := th.NewTestContext()
		p := newMockProvider(t)
		drv := &DRVolume{p: p}
		drv.pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: namespace,
			},
		}

		snapshot := &volumesnapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: namespace,
			},
		}

		p.esClient.EXPECT().SnapshotVolume(mock.Anything, drv.pvc.Namespace, drv.pvc.Name, externalsnapshotter.SnapshotVolumeOptions{
			Name:          snapshotName,
			SnapshotClass: opts.SnapshotClass,
		}).Return(snapshot, nil)

		p.esClient.EXPECT().WaitForReadySnapshot(mock.Anything, drv.pvc.Namespace, snapshotName, externalsnapshotter.WaitForReadySnapshotOpts{
			MaxWaitTime: opts.ReadyTimeout,
		}).Return(snapshot, nil)

		err := drv.SnapshotAndWaitReady(ctx, snapshotName, opts)
		assert.NoError(t, err)
	})
}
