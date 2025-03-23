package core

import (
	"sync"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestCreatePod(t *testing.T) {
	namespace := "test-ns"
	podName := "test-pod"

	tests := []struct {
		desc                string
		labels              map[string]string
		simulateClientError bool
	}{
		{
			desc: "create pod successfully",
		},
		{
			desc:   "create pod with labels",
			labels: map[string]string{"key": "value"},
		},
		{
			desc:                "creation errors",
			simulateClientError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			c.SetCommonLabels(tt.labels)
			ctx := th.NewTestContext()

			if tt.simulateClientError {
				mockK8s.PrependReactor("create", "pods", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					Labels:    map[string]string{"app": "test"},
				},
			}

			createdPod, err := c.CreatePod(ctx, namespace, pod)
			if tt.simulateClientError {
				assert.Error(t, err)
				assert.Nil(t, createdPod)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, pod, createdPod)
		})
	}
}

func TestWaitForReadyPod(t *testing.T) {
	podName := "test-pod"
	podNamespace := "test-ns"

	noStatusPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
	}

	notReadyPod := noStatusPod.DeepCopy()
	notReadyCondition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionFalse}
	notReadyPod.Status.Conditions = append(notReadyPod.Status.Conditions, notReadyCondition)

	readyPod := notReadyPod.DeepCopy()
	readyCondition := notReadyCondition.DeepCopy()
	readyCondition.Status = corev1.ConditionTrue
	readyPod.Status.Conditions[0] = *readyCondition

	multipleConditionsPod := readyPod.DeepCopy()
	issuingCondition := corev1.PodCondition{Type: corev1.PodReadyToStartContainers, Status: corev1.ConditionFalse}
	multipleConditionsPod.Status.Conditions = []corev1.PodCondition{issuingCondition, *readyCondition}

	tests := []struct {
		desc                string
		initialPod          *corev1.Pod
		shouldError         bool
		afterStartedWaiting func(*testing.T, *contexts.Context, k8s.Interface)
	}{
		{
			desc:       "pod starts ready",
			initialPod: readyPod,
		},
		{
			desc:        "pod not ready",
			initialPod:  notReadyPod,
			shouldError: true,
		},
		{
			desc:        "pod has no status",
			initialPod:  noStatusPod,
			shouldError: true,
		},
		{
			desc:        "pod does not exist",
			shouldError: true,
		},
		{
			desc:       "pod becomes ready",
			initialPod: notReadyPod,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Pods(podNamespace).Update(ctx, readyPod, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:       "multiple conditions",
			initialPod: notReadyPod,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Pods(podNamespace).Update(ctx, multipleConditionsPod, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			kc, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialPod != nil {
				_, err := mockK8s.CoreV1().Pods(podNamespace).Create(ctx, tt.initialPod, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var pod *corev1.Pod
			wg.Add(1)
			go func() {
				pod, waitErr = kc.WaitForReadyPod(ctx, podNamespace, podName, WaitForReadyPodOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, mockK8s)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, pod)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, pod)
		})
	}
}

func TestDeletePod(t *testing.T) {
	namespace := "test-ns"
	podName := "test-pod"

	tests := []struct {
		desc           string
		shouldSetupPod bool
		wantErr        bool
	}{
		{
			desc:           "delete existing pod",
			shouldSetupPod: true,
		},
		{
			desc:    "delete non-existent pod",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			var existingPod *corev1.Pod
			if tt.shouldSetupPod {
				existingPod = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podName,
						Namespace: namespace,
					},
				}
				_, err := mockK8s.CoreV1().Pods(namespace).Create(ctx, existingPod, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := c.DeletePod(ctx, namespace, podName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the pod was deleted
			podList, err := mockK8s.CoreV1().Pods(namespace).List(ctx, metav1.SingleObject(existingPod.ObjectMeta))
			assert.NoError(t, err)
			assert.Empty(t, podList.Items)
		})
	}
}

func TestNewSingleContainerPVC(t *testing.T) {
	scv := NewSingleContainerPVC("test-pvc", "/data")
	assert.Equal(t, "test-pvc", scv.Name)
	assert.Equal(t, []string{"/data"}, scv.MountPaths)
	assert.Equal(t, corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "test-pvc",
		},
	}, scv.VolumeSource)
}

func TestNewSingleContainerSecret(t *testing.T) {
	tests := []struct {
		name       string
		secretName string
		mountPath  string
		items      []corev1.KeyToPath
		want       SingleContainerVolume
	}{
		{
			name:       "basic secret volume",
			secretName: "test-secret",
			mountPath:  "/secrets",
			want: SingleContainerVolume{
				Name:       "test-secret",
				MountPaths: []string{"/secrets"},
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "test-secret",
						DefaultMode: ptr.To(int32(0400)),
					},
				},
			},
		},
		{
			name:       "only mount specific items",
			secretName: "test-secret",
			mountPath:  "/secrets",
			items: []corev1.KeyToPath{
				{
					Key:  "secret-key",
					Path: "file-mount-path",
					Mode: ptr.To(int32(0123)),
				},
			},
			want: SingleContainerVolume{
				Name:       "test-secret",
				MountPaths: []string{"/secrets"},
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "test-secret",
						DefaultMode: ptr.To(int32(0400)),
						Items: []corev1.KeyToPath{
							{
								Key:  "secret-key",
								Path: "file-mount-path",
								Mode: ptr.To(int32(0123)),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSingleContainerSecret(tt.secretName, tt.mountPath, tt.items...)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSingleContainerVolumeToVolume(t *testing.T) {
	tests := []struct {
		name string
		scv  SingleContainerVolume
		want corev1.Volume
	}{
		{
			name: "pvc volume",
			scv: SingleContainerVolume{
				Name: "test-pvc",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
					},
				},
			},
			want: corev1.Volume{
				Name: "test-pvc",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
					},
				},
			},
		},
		{
			name: "secret volume",
			scv: SingleContainerVolume{
				Name: "test-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "test-secret",
						DefaultMode: ptr.To(int32(0400)),
						Items: []corev1.KeyToPath{
							{
								Key:  "secret-key",
								Path: "file-mount-path",
								Mode: ptr.To(int32(0123)),
							},
						},
					},
				},
			},
			want: corev1.Volume{
				Name: "test-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "test-secret",
						DefaultMode: ptr.To(int32(0400)),
						Items: []corev1.KeyToPath{
							{
								Key:  "secret-key",
								Path: "file-mount-path",
								Mode: ptr.To(int32(0123)),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.scv.ToVolume()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSingleContainerVolumeToVolumeMounts(t *testing.T) {
	tests := []struct {
		name string
		scv  SingleContainerVolume
		want []corev1.VolumeMount
	}{
		{
			name: "basic volume mount",
			scv: SingleContainerVolume{
				Name:       "test-volume",
				MountPaths: []string{"/mnt/data"},
			},
			want: []corev1.VolumeMount{
				{
					Name:      "test-volume",
					MountPath: "/mnt/data",
				},
			},
		},
		{
			name: "multiple volume mounts",
			scv: SingleContainerVolume{
				Name:       "test-volume",
				MountPaths: []string{"/mnt/data", "/mnt/data2"},
			},
			want: []corev1.VolumeMount{
				{
					Name:      "test-volume",
					MountPath: "/mnt/data",
				},
				{
					Name:      "test-volume",
					MountPath: "/mnt/data2",
				},
			},
		},
		{
			name: "empty paths",
			scv:  SingleContainerVolume{},
			want: []corev1.VolumeMount{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.scv.ToVolumeMounts()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertSingleContainerVolumes(t *testing.T) {
	tests := []struct {
		desc            string
		scvs            []SingleContainerVolume
		expectedVolumes []corev1.Volume
		expectedMounts  []corev1.VolumeMount
	}{
		{
			desc: "basic conversion of a PVC",
			scvs: []SingleContainerVolume{
				{
					Name:       "test-pvc",
					MountPaths: []string{"/mnt/data"},
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
			expectedVolumes: []corev1.Volume{
				{
					Name: "test-pvc",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
			expectedMounts: []corev1.VolumeMount{
				{
					Name:      "test-pvc",
					MountPath: "/mnt/data",
				},
			},
		},
		{
			desc: "basic conversion of a secret",
			scvs: []SingleContainerVolume{
				{
					Name:       "test-secret",
					MountPaths: []string{"/secrets"},
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: ptr.To(int32(0400)),
						},
					},
				},
			},
			expectedVolumes: []corev1.Volume{
				{
					Name: "test-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: ptr.To(int32(0400)),
						},
					},
				},
			},
			expectedMounts: []corev1.VolumeMount{
				{
					Name:      "test-secret",
					MountPath: "/secrets",
				},
			},
		},
		{
			desc: "multiple PVC SCVs with the same name",
			scvs: []SingleContainerVolume{
				{
					Name:       "test-pvc",
					MountPaths: []string{"/mnt/data1"},
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
				{
					Name:       "test-pvc",
					MountPaths: []string{"/mnt/data2"},
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
			expectedVolumes: []corev1.Volume{
				{
					Name: "test-pvc",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
			expectedMounts: []corev1.VolumeMount{
				{
					Name:      "test-pvc",
					MountPath: "/mnt/data1",
				},
				{
					Name:      "test-pvc",
					MountPath: "/mnt/data2",
				},
			},
		},
		{
			desc: "multiple secret SCVs with the same name",
			scvs: []SingleContainerVolume{
				{
					Name:       "test-secret",
					MountPaths: []string{"/secrets1"},
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: ptr.To(int32(0400)),
						},
					},
				},
				{
					Name:       "test-secret",
					MountPaths: []string{"/secrets2", "/secrets3"},
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: ptr.To(int32(0400)),
						},
					},
				},
			},
			expectedVolumes: []corev1.Volume{
				{
					Name: "test-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: ptr.To(int32(0400)),
						},
					},
				},
			},
			expectedMounts: []corev1.VolumeMount{
				{
					Name:      "test-secret",
					MountPath: "/secrets1",
				},
				{
					Name:      "test-secret",
					MountPath: "/secrets2",
				},
				{
					Name:      "test-secret",
					MountPath: "/secrets3",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			volumes, mounts := ConvertSingleContainerVolumes(tt.scvs)
			assert.Equal(t, tt.expectedVolumes, volumes)
			assert.Equal(t, tt.expectedMounts, mounts)
		})
	}
}

func TestRestrictedPodSecurityContext(t *testing.T) {
	tests := []struct {
		name string
		uid  int64
		gid  int64
	}{
		{
			name: "non-root user/group",
			uid:  1000,
			gid:  1000,
		},
		{
			name: "different uid/gid",
			uid:  2000,
			gid:  3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdSC := RestrictedPodSecurityContext(tt.uid, tt.gid)
			require.NotNil(t, createdSC)
			assert.Equal(t, tt.uid, *createdSC.RunAsUser)
			assert.Equal(t, tt.gid, *createdSC.RunAsGroup)
			assert.True(t, *createdSC.RunAsNonRoot)
			require.NotNil(t, createdSC.SeccompProfile)
			assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, createdSC.SeccompProfile.Type)
		})
	}
}

func TestRestrictedContainerSecurityContext(t *testing.T) {
	tests := []struct {
		name string
		uid  int64
		gid  int64
	}{
		{
			name: "non-root user/group",
			uid:  1000,
			gid:  1000,
		},
		{
			name: "different uid/gid",
			uid:  2000,
			gid:  3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdSC := RestrictedContainerSecurityContext(tt.uid, tt.gid)
			require.NotNil(t, createdSC)
			assert.NotNil(t, createdSC.Capabilities)
			assert.Equal(t, []corev1.Capability{"ALL"}, createdSC.Capabilities.Drop)
			assert.False(t, *createdSC.Privileged)
			assert.Equal(t, tt.uid, *createdSC.RunAsUser)
			assert.Equal(t, tt.gid, *createdSC.RunAsGroup)
			assert.True(t, *createdSC.RunAsNonRoot)
			assert.True(t, *createdSC.ReadOnlyRootFilesystem)
			assert.False(t, *createdSC.AllowPrivilegeEscalation)
			require.NotNil(t, createdSC.SeccompProfile)
			assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, createdSC.SeccompProfile.Type)
		})
	}
}

func TestPrivilegedPodSecurityContext(t *testing.T) {
	createdSC := PrivilegedPodSecurityContext()
	require.NotNil(t, createdSC)
	assert.Equal(t, int64(0), *createdSC.RunAsUser)
	assert.Equal(t, int64(0), *createdSC.RunAsGroup)
	assert.False(t, *createdSC.RunAsNonRoot)
	require.NotNil(t, createdSC.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, createdSC.SeccompProfile.Type)
}

func TestPrivilegedContainerSecurityContext(t *testing.T) {
	createdSC := PrivilegedContainerSecurityContext()
	require.NotNil(t, createdSC)
	assert.True(t, *createdSC.Privileged)
	assert.Equal(t, int64(0), *createdSC.RunAsUser)
	assert.Equal(t, int64(0), *createdSC.RunAsGroup)
	assert.False(t, *createdSC.RunAsNonRoot)
	assert.True(t, *createdSC.ReadOnlyRootFilesystem)
	assert.True(t, *createdSC.AllowPrivilegeEscalation)
	require.NotNil(t, createdSC.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, createdSC.SeccompProfile.Type)
}
