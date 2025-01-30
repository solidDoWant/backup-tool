package clonepvc

import (
	context "context"
	"testing"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
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
	"k8s.io/utils/ptr"
)

func TestClonePVC(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"
	snapshotName := "test-snapshot"
	podName := "test-pod"
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

	createdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
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
		simulatePodCreateErr       bool
		simulatePodWaitErr         bool
		simulatePodDeleteError     bool
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
					Name:      pvcName,
					Namespace: namespace,
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
					Name:      pvcName,
					Namespace: namespace,
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
				ForceBind:            true,
				ForceBindTimeout:     helpers.ShortWaitTime,
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
					Name:      "custom-prefix",
					Namespace: namespace,
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
		{
			desc: "error while creating pod for force bind",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
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
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
			simulatePodCreateErr: true,
		},
		{
			desc: "error while waiting for pod to be ready",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
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
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
			simulatePodWaitErr: true,
		},
		{
			desc: "error while deleting pod",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
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
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
			simulatePodDeleteError: true,
			simulatePodWaitErr:     true,
		},
		{
			desc: "error while deleting pvc",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("standard"),
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
						APIGroup: ptr.To(volumesnapshotv1.SchemeGroupVersion.Group),
						Kind:     externalsnapshotter.VolumeSnapshotKind,
						Name:     snapshotName,
					},
				},
			},
			simulatePodWaitErr:     true,
			simulatePVCDeleteError: true,
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

				p.coreClient.EXPECT().CreatePVC(ctx, namespace, newPVCName, size, opts).
					RunAndReturn(func(ctx context.Context, s1, s2 string, q resource.Quantity, cp core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						pvc := tt.clonedPVC
						if pvc == nil {
							pvc = tt.expectedPVC
						}

						return th.ErrOr1Val(pvc, tt.simulateCreateErr)
					})
				if tt.simulateCreateErr {
					return
				}

				if tt.simulatePodCreateErr || tt.simulatePodWaitErr || tt.simulatePodDeleteError {
					p.coreClient.EXPECT().DeletePVC(mock.Anything, namespace, mock.Anything).Return(th.ErrIfTrue(tt.simulatePVCDeleteError))
				}

				if !tt.opts.ForceBind {
					return
				}

				p.coreClient.EXPECT().CreatePod(ctx, namespace, mock.Anything).
					RunAndReturn(func(ctx context.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
						assert.Contains(t, pod.Name, "force-bind")
						assert.Len(t, pod.Spec.Containers, 1)
						assert.Len(t, pod.Spec.Volumes, 1)

						volume := pod.Spec.Volumes[0]
						require.NotNil(t, volume.PersistentVolumeClaim)

						container := pod.Spec.Containers[0]
						assert.Contains(t, container.Image, "pause")
						assert.Len(t, container.VolumeMounts, 1)
						volumeMount := container.VolumeMounts[0]
						assert.Equal(t, volume.Name, volumeMount.Name)

						return th.ErrOr1Val(createdPod, tt.simulatePodCreateErr)
					})
				if tt.simulatePodCreateErr {
					return
				}
				p.coreClient.EXPECT().DeletePod(mock.Anything, namespace, createdPod.Name).Return(th.ErrIfTrue(tt.simulatePodDeleteError))

				p.coreClient.EXPECT().WaitForReadyPod(ctx, namespace, createdPod.Name, core.WaitForReadyPodOpts{MaxWaitTime: tt.opts.ForceBindTimeout}).
					Return(th.ErrOr1Val(createdPod, tt.simulatePodWaitErr))
			}()

			clonedPVC, err := p.ClonePVC(ctx, namespace, pvcName, tt.opts)
			if th.ErrExpected(
				tt.simulateSnapshotErr,
				tt.simulateWaitForSnapshotErr,
				tt.simulateQueryExistingErr,
				tt.simulateCreateErr,
				tt.expectRestoreSizeErr,
				tt.simulatePodCreateErr,
				tt.simulatePodWaitErr,
				tt.simulatePodDeleteError,
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
