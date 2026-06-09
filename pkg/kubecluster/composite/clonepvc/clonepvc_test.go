package clonepvc

import (
	"testing"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		clonedPVC                  *corev1.PersistentVolumeClaim
		simulateSnapshotErr        bool
		simulateWaitForSnapshotErr bool
		simulateQueryExistingErr   bool
		simulateCreateErr          bool
		simulateForceBindErr       bool
		simulatePVCDeleteError     bool
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
					StorageClassName: new("standard"),
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: size,
						},
					},
				},
			},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: new("standard"),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
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
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
		},
		{
			desc: "successful clone with custom options",
			opts: ClonePVCOptions{
				SnapshotClass:        "custom-snap-class",
				DestStorageClassName: "custom-class",
				DestPvcNamePrefix:    "custom-prefix",
				ForceBind:            true,
				ForceBindTimeout:     helpers.ShortWaitTime,
			},
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: new("standard"),
				},
			},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-prefix",
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: new("custom-class"),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
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
					StorageClassName: new("standard"),
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
					StorageClassName: new("standard"),
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
					StorageClassName: new("standard"),
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
					StorageClassName: new("standard"),
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
					StorageClassName: new("standard"),
				},
			},
			simulateCreateErr: true,
		},
		{
			desc: "error while force binding",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: new("standard"),
				},
			},
			opts: ClonePVCOptions{ForceBind: true},
			clonedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
			simulateForceBindErr: true,
		},
		{
			desc: "error while deleting pvc after force bind failure",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: new("standard"),
				},
			},
			opts: ClonePVCOptions{ForceBind: true},
			clonedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
			simulateForceBindErr:   true,
			simulatePVCDeleteError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := newMockProvider(t)
			ctx := th.NewTestContext()

			// This makes the logic for setting up mocks/expected calls easier, because once an error
			// becomes expected, the function can be returned from
			func() {
				snapshot := th.ValOrDefault(tt.createdSnapshot, createdSnapshot)
				p.esClient.EXPECT().SnapshotVolume(mock.Anything, namespace, pvcName, externalsnapshotter.SnapshotVolumeOptions{SnapshotClass: tt.opts.SnapshotClass}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, opts externalsnapshotter.SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(snapshot, tt.simulateSnapshotErr)
					})

				if tt.simulateSnapshotErr {
					return
				}

				p.esClient.EXPECT().DeleteSnapshot(mock.Anything, namespace, snapshotName).RunAndReturn(func(cleanupCtx *contexts.Context, namespace, name string) error {
					require.NotEqual(t, ctx, cleanupCtx)
					return nil
				})

				p.esClient.EXPECT().WaitForReadySnapshot(mock.Anything, namespace, snapshotName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, opts externalsnapshotter.WaitForReadySnapshotOpts) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(snapshot, tt.simulateWaitForSnapshotErr)
					})
				if tt.simulateWaitForSnapshotErr {
					return
				}

				if tt.opts.DestStorageClassName == "" {
					p.coreClient.EXPECT().GetPVC(mock.Anything, namespace, pvcName).RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						if tt.initialPVC != nil {
							return th.ErrOr1Val(tt.initialPVC, tt.simulateQueryExistingErr)
						}
						return nil, assert.AnError
					})
				}
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
						APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				}

				if tt.opts.DestStorageClassName != "" {
					opts.StorageClassName = tt.opts.DestStorageClassName
				} else if tt.initialPVC != nil && tt.initialPVC.Spec.StorageClassName != nil {
					opts.StorageClassName = *tt.initialPVC.Spec.StorageClassName
				}

				p.coreClient.EXPECT().CreatePVC(mock.Anything, namespace, newPVCName, size, opts).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, newPVCName string, size resource.Quantity, opts core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						pvc := tt.clonedPVC
						if pvc == nil {
							pvc = tt.expectedPVC
						}

						return th.ErrOr1Val(pvc, tt.simulateCreateErr)
					})
				if tt.simulateCreateErr {
					return
				}

				if tt.simulateForceBindErr {
					p.coreClient.EXPECT().DeletePVC(mock.Anything, namespace, mock.Anything).
						RunAndReturn(func(cleanupCtx *contexts.Context, namespace, name string) error {
							assert.NotEqual(t, ctx, cleanupCtx)
							return th.ErrIfTrue(tt.simulatePVCDeleteError)
						})
				}

				if !tt.opts.ForceBind {
					return
				}

				// Force-binding runs the internal forceBindVolumes helper against the core client.
				p.expectForceBind(t, ctx, namespace, tt.simulateForceBindErr)
			}()

			clonedPVC, err := p.ClonePVC(ctx, namespace, pvcName, tt.opts)
			if th.ErrExpected(
				tt.simulateSnapshotErr,
				tt.simulateWaitForSnapshotErr,
				tt.simulateQueryExistingErr,
				tt.simulateCreateErr,
				tt.expectRestoreSizeErr,
				tt.simulateForceBindErr,
				tt.simulatePVCDeleteError,
			) {
				require.Error(t, err)
				require.Nil(t, clonedPVC)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedPVC, clonedPVC)
		})
	}
}
