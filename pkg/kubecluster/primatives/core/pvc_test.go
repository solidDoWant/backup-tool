package core

import (
	"testing"

	"dario.cat/mergo"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestCreatePVC(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"
	snapshotName := "test-snapshot"
	size := resource.MustParse("1Gi")

	standardPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	tests := []struct {
		name                string
		simulateClientError bool
		opts                CreatePVCOptions
		expectedPVC         *corev1.PersistentVolumeClaim
		expectedErr         bool
	}{
		{
			name: "basic create with no options",
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: pvcName,
				},
			},
		},
		{
			name: "all options set",
			opts: CreatePVCOptions{
				GenerateName:     true,
				StorageClassName: "custom-class",
				Source: &corev1.TypedObjectReference{
					APIGroup:  ptr.To("api-group"),
					Kind:      "some-kind",
					Namespace: &namespace,
					Name:      snapshotName,
				},
			},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: pvcName,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("custom-class"),
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup:  ptr.To("api-group"),
						Kind:      "some-kind",
						Namespace: &namespace,
						Name:      snapshotName,
					},
				},
			},
		},
		{
			name:                "simulate client error",
			simulateClientError: true,
			expectedErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.simulateClientError {
				mockK8s.PrependReactor("create", "persistentvolumeclaims", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			pvc, err := c.CreatePVC(ctx, namespace, pvcName, size, tt.opts)
			if tt.expectedErr {
				require.Error(t, err)
				require.Nil(t, pvc)
			} else {
				require.NoError(t, err)

				expectedPVC := standardPVC.DeepCopy()
				err := mergo.MergeWithOverwrite(expectedPVC, tt.expectedPVC)
				require.NoError(t, err)

				require.Equal(t, expectedPVC, pvc)
			}
		})
	}
}

func TestGetPVC(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"

	tests := []struct {
		name        string
		initialPVC  *corev1.PersistentVolumeClaim
		expectedErr bool
	}{
		{
			name: "pvc exists",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
			},
		},
		{
			name:        "pvc does not exist",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialPVC != nil {
				_, err := mockK8s.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, tt.initialPVC, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			pvc, err := c.GetPVC(ctx, namespace, pvcName)
			if tt.expectedErr {
				require.Error(t, err)
				require.Nil(t, pvc)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, pvc)
		})
	}
}

func TestDoesPVCExist(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"

	tests := []struct {
		name          string
		initialPVC    *corev1.PersistentVolumeClaim
		expectedExist bool
		expectedErr   bool
	}{
		{
			name: "pvc exists",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
			},
			expectedExist: true,
		},
		{
			name:          "pvc does not exist",
			expectedExist: false,
		},
		{
			name:          "error querying pvc",
			expectedExist: false,
			expectedErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialPVC != nil {
				_, err := mockK8s.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, tt.initialPVC, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.expectedErr {
				mockK8s.PrependReactor("get", "persistentvolumeclaims", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			exists, err := c.DoesPVCExist(ctx, namespace, pvcName)
			if tt.expectedErr {
				require.Error(t, err)
				require.False(t, exists)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedExist, exists)
			}
		})
	}
}

func TestEnsurePVCExists(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-pvc"
	size := resource.MustParse("1Gi")

	tests := []struct {
		name                string
		initialPVC          *corev1.PersistentVolumeClaim
		simulateClientError bool
		opts                CreatePVCOptions
		expectedPVC         *corev1.PersistentVolumeClaim
		expectedErr         bool
	}{
		{
			name: "pvc already exists",
			opts: CreatePVCOptions{GenerateName: true, StorageClassName: "custom-class"},
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
			},
		},
		{
			name: "pvc does not exist, create new",
			opts: CreatePVCOptions{GenerateName: true, StorageClassName: "custom-class"},
			expectedPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: pvcName,
					Namespace:    namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To("custom-class"),
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: size,
						},
					},
				},
			},
		},
		{
			name:                "simulate client error when querying for existing pvc",
			simulateClientError: true,
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
			},
			expectedErr: true,
		},
		{
			name:                "simulate client error when creating a new pvc",
			simulateClientError: true,
			expectedErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialPVC != nil {
				_, err := mockK8s.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, tt.initialPVC, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.simulateClientError {
				verb := "get"
				if tt.initialPVC == nil {
					verb = "create"
				}

				mockK8s.PrependReactor(verb, "persistentvolumeclaims", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			pvc, err := c.EnsurePVCExists(ctx, namespace, pvcName, size, tt.opts)
			if tt.expectedErr {
				require.Error(t, err)
				require.Nil(t, pvc)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedPVC, pvc)
			}
		})
	}
}

func TestDeleteVolume(t *testing.T) {
	namespace := "test-ns"
	pvcName := "test-volume"

	tests := []struct {
		name                string
		initialPVC          *corev1.PersistentVolumeClaim
		simulateClientError bool
		expectedErr         bool
	}{
		{
			name: "successful deletion",
			initialPVC: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
			},
		},
		{
			name:                "simulate client error",
			simulateClientError: true,
			expectedErr:         true,
		},
		{
			name:        "volume does not exist",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialPVC != nil {
				_, err := mockK8s.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, tt.initialPVC, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.simulateClientError {
				mockK8s.PrependReactor("delete", "persistentvolumeclaims", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			err := c.DeletePVC(ctx, namespace, pvcName)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
