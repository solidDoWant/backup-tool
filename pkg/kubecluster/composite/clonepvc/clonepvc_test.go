package clonepvc

import (
	context "context"
	"testing"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestClonePVC(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"
	snapshotName := "test-snapshot"
	size := resource.MustParse("5Gi")

	createdSnapshot := &volumesnapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: namespace,
		},
		Status: &volumesnapshotv1.VolumeSnapshotStatus{
			RestoreSize: &size,
		},
	}

	tests := []struct {
		desc                       string
		opts                       ClonePVCOptions
		initialPVC                 *corev1.PersistentVolumeClaim
		createdSnapshot            *volumesnapshotv1.VolumeSnapshot
		simulateSnapshotErr        bool
		simulateWaitForSnapshotErr bool
		simulateQueryExistingErr   bool
		simulateCreateErr          bool
		expectRestoreSizeErr       bool
		expectedPVC                *corev1.PersistentVolumeClaim
	}{
		{
			desc: "successful clone with default options",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: size,
						},
					},
				},
			},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: pvcName,
					Namespace:    namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
		},
		{
			desc: "successful clone with default options, storage class not specified on existing PVC",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
			},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: pvcName,
					Namespace:    namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
		},
		{
			desc: "successful clone with custom options",
			opts: ClonePVCOptions{
				DestStorageClassName: "custom-class",
				DestPvcNamePrefix:    "custom-prefix",
			},
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
				},
			},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "custom-prefix-",
					Namespace:    namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("custom-class"),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
		},
		{
			desc:                "snapshot error",
			simulateSnapshotErr: true,
		},
		{
			desc:                       "wait for snapshot error",
			simulateWaitForSnapshotErr: true,
		},
		{
			desc: "error while querying existing PVC",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
				},
			},
			simulateQueryExistingErr: true,
		},
		{
			desc: "error while querying existing PVC",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
				},
			},
			simulateQueryExistingErr: true,
		},
		{
			desc: "snapshot has no restore size",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
				},
			},
			createdSnapshot: &volumesnapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      snapshotName,
					Namespace: namespace,
				},
				Status: &volumesnapshotv1.VolumeSnapshotStatus{},
			},
			expectRestoreSizeErr: true,
		},
		{
			desc: "snapshot has no status",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
				},
			},
			createdSnapshot: &volumesnapshotv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      snapshotName,
					Namespace: namespace,
				},
			},
			expectRestoreSizeErr: true,
		},
		{
			desc: "creation error",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
				},
			},
			simulateCreateErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := newMockProvider(t)
			ctx := context.Background()

			// This makes the logic for setting up mocks/expected calls easier, because once an error
			// becomes expected, the function can be returned from
			func() {
				snapshot := th.ValOrDefault(tt.createdSnapshot, createdSnapshot)
				p.esClient.EXPECT().SnapshotVolume(ctx, namespace, pvcName, externalsnapshotter.SnapshotVolumeOptions{}).
					Return(th.ErrOr1Val(snapshot, tt.simulateSnapshotErr))
				if tt.simulateSnapshotErr {
					return
				}

				p.esClient.EXPECT().DeleteSnapshot(mock.Anything, namespace, snapshotName).RunAndReturn(func(cleanupCtx context.Context, _, _ string) error {
					require.NotEqual(t, ctx, cleanupCtx)
					return nil
				})

				p.esClient.EXPECT().WaitForReadySnapshot(ctx, namespace, snapshotName, mock.Anything).
					Return(th.ErrOr1Val(snapshot, tt.simulateWaitForSnapshotErr))
				if tt.simulateWaitForSnapshotErr {
					return
				}

				p.coreClient.EXPECT().GetPVC(ctx, namespace, pvcName).RunAndReturn(func(ctx context.Context, namespace, pvcName string) (*corev1.PersistentVolumeClaim, error) {
					if tt.initialPVC != nil {
						return th.ErrOr1Val(tt.initialPVC, tt.simulateQueryExistingErr)
					}
					return nil, assert.AnError
				}).Maybe() // This only needs to be called when certain options are not set
				if tt.simulateQueryExistingErr {
					return
				}

				if tt.expectRestoreSizeErr {
					return
				}

				newPVCName := pvcName
				if tt.opts.DestPvcNamePrefix != "" {
					newPVCName = tt.opts.DestPvcNamePrefix
				}

				opts := core.CreatePVCOptions{
					GenerateName: true,
					Source: &corev1.TypedObjectReference{
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				}

				if tt.opts.DestStorageClassName != "" {
					opts.StorageClassName = tt.opts.DestStorageClassName
				} else if tt.initialPVC != nil && tt.initialPVC.Spec.StorageClassName != nil {
					opts.StorageClassName = *tt.initialPVC.Spec.StorageClassName
				}

				p.coreClient.EXPECT().CreatePVC(ctx, namespace, newPVCName, size, opts).Return(th.ErrOr1Val(tt.expectedPVC, tt.simulateCreateErr))
			}()

			clonedPVC, err := p.ClonePVC(ctx, namespace, pvcName, tt.opts)
			if th.ErrExpected(tt.simulateSnapshotErr, tt.simulateWaitForSnapshotErr, tt.simulateQueryExistingErr, tt.simulateCreateErr, tt.expectRestoreSizeErr) {
				require.Error(t, err)
				require.Nil(t, clonedPVC)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedPVC, clonedPVC)
		})
	}
}
