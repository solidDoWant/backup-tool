package backuptoolinstance

import (
	"testing"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestNewBackupToolInstance(t *testing.T) {
	c := newMockProvider(t)
	btInstance := newBackupToolInstance(c)
	casted := btInstance.(*BackupToolInstance)

	assert.Equal(t, c, casted.p)
}

func TestCreateBackupToolInstanceOptions(t *testing.T) {
	th.OptStructTest[CreateBackupToolInstanceOptions](t)
}

func TestCreateBackupToolInstance(t *testing.T) {
	namespace := "test-namespace"

	tests := []struct {
		name                                   string
		opts                                   CreateBackupToolInstanceOptions
		simulateBackupToolInstanceCleanupError bool
		simulateCreatePodError                 bool
		simulateWaitForPodError                bool
	}{
		{
			name: "basic instance creation",
		},
		{
			name: "basic instance creation with all options set",
			opts: CreateBackupToolInstanceOptions{
				NamePrefix: "test-prefix-",
				Volumes: []core.SingleContainerVolume{
					{
						Name:       "vol1",
						MountPaths: []string{"/data1"},
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc1",
							},
						},
					},
					{
						Name:       "vol2",
						MountPaths: []string{"/data2"},
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc2",
							},
						},
					},

					{
						Name:       "vol3",
						MountPaths: []string{"/secret1"},
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  "secret1",
								DefaultMode: ptr.To(int32(0765)),
							},
						},
					},
				},
				CleanupTimeout: helpers.ShortWaitTime,
				PodWaitTimeout: helpers.ShortWaitTime,
			},
		},
		{
			name:                   "simulate create pod error",
			simulateCreatePodError: true,
		},
		{
			name:                                   "simulate create pod error and cleanup error",
			simulateCreatePodError:                 true,
			simulateBackupToolInstanceCleanupError: true,
		},
		{
			name:                    "simulate wait for pod error",
			simulateWaitForPodError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newMockProvider(t)
			ctx := th.NewTestContext()

			errExpected := th.ErrExpected(
				tt.simulateBackupToolInstanceCleanupError,
				tt.simulateCreatePodError,
				tt.simulateWaitForPodError,
			)

			func() {
				if errExpected {
					p.backupToolInstance.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
						require.NotEqual(t, ctx, cleanupCtx)
						return th.ErrIfTrue(tt.simulateBackupToolInstanceCleanupError)
					})
				}

				var createdPod *corev1.Pod
				p.coreClient.EXPECT().CreatePod(mock.Anything, namespace, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, _ string, pod *corev1.Pod) (*corev1.Pod, error) {
						createdPod = pod

						assert.True(t, calledCtx.IsChildOf(ctx))

						require.Len(t, pod.Spec.Containers, 1)
						require.Equal(t, len(tt.opts.Volumes), len(pod.Spec.Volumes))
						require.Contains(t, pod.ObjectMeta.Labels, "app.kubernetes.io/component")

						container := pod.Spec.Containers[0]
						require.Equal(t, constants.ToolName, container.Name)
						require.Equal(t, constants.FullImageName, container.Image)
						require.Equal(t, []string{constants.ToolName}, container.Command)
						require.Equal(t, len(tt.opts.Volumes), len(container.VolumeMounts))
						require.Len(t, container.Ports, 1)

						port := container.Ports[0]
						require.Equal(t, "grpc", port.Name)
						require.Equal(t, int32(grpc.GRPCPort), port.ContainerPort)
						require.Equal(t, corev1.ProtocolTCP, port.Protocol)

						require.NotNil(t, pod.Spec.SecurityContext)
						require.NotNil(t, pod.Spec.SecurityContext.RunAsUser)
						require.Equal(t, int64(0), *pod.Spec.SecurityContext.RunAsUser)
						require.NotNil(t, pod.Spec.SecurityContext.RunAsGroup)
						require.Equal(t, int64(0), *pod.Spec.SecurityContext.RunAsGroup)

						require.NotNil(t, container.SecurityContext)
						require.NotNil(t, container.SecurityContext.RunAsGroup)
						require.Equal(t, *pod.Spec.SecurityContext.RunAsGroup, *container.SecurityContext.RunAsGroup)

						return th.ErrOr1Val(pod, tt.simulateCreatePodError)
					})
				if tt.simulateCreatePodError {
					return
				}
				// setPod is called once with the freshly-created pod (so the deferred cleanup can
				// delete it even if the readiness wait fails) and again with the ready pod, which
				// carries the assigned IP address used by the GRPC client.
				p.backupToolInstance.EXPECT().setPod(mock.Anything).Run(func(pod *corev1.Pod) {
					require.Equal(t, createdPod, pod)
				})

				p.coreClient.EXPECT().WaitForReadyPod(mock.Anything, namespace, mock.Anything, core.WaitForReadyPodOpts{MaxWaitTime: tt.opts.PodWaitTimeout}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, wfrpo core.WaitForReadyPodOpts) (*corev1.Pod, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(createdPod, tt.simulateWaitForPodError)
					})
				if tt.simulateWaitForPodError {
					return
				}
			}()

			btInstance, err := p.CreateBackupToolInstance(ctx, namespace, "unique-instance-name", tt.opts)
			if errExpected {
				assert.Error(t, err)
				assert.Nil(t, btInstance)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, btInstance)

			// The expected "set" functions above confirm that the backuptoolinstance values are set correctly
		})
	}
}

func TestBackupToolInstanceSetPod(t *testing.T) {
	tests := []struct {
		desc string
		pod  *corev1.Pod
	}{
		{
			desc: "set non-nil pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil pod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			btInstance := &BackupToolInstance{}
			btInstance.setPod(tt.pod)
			assert.Equal(t, tt.pod, btInstance.pod)
		})
	}
}

func TestBackupToolInstanceGetPod(t *testing.T) {
	tests := []struct {
		desc       string
		btInstance BackupToolInstance
		want       *corev1.Pod
	}{
		{
			desc: "get existing pod",
			btInstance: BackupToolInstance{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-ns",
					},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil pod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.btInstance.GetPod()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBackupToolInstanceGetGRPCClient(t *testing.T) {
	tests := []struct {
		desc string
		pod  *corev1.Pod
	}{
		{
			desc: "nil pod returns an error",
		},
		{
			desc: "pod without an assigned IP returns an error",
			pod:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			btInstance := &BackupToolInstance{pod: tt.pod}

			client, err := btInstance.GetGRPCClient(ctx)
			assert.Error(t, err)
			assert.True(t, trace.IsNotFound(err))
			assert.Nil(t, client)
		})
	}
}

func TestBackupToolInstanceDelete(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns"},
	}

	tests := []struct {
		desc                   string
		btInstance             BackupToolInstance
		simulatePodDeleteError bool
		wantErr                bool
	}{
		{
			desc:       "delete with pod",
			btInstance: BackupToolInstance{pod: pod},
		},
		{
			desc: "delete empty backup tool instance",
		},
		{
			desc:                   "pod deletion fails",
			btInstance:             BackupToolInstance{pod: pod},
			simulatePodDeleteError: true,
			wantErr:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			p := newMockProvider(t)
			tt.btInstance.p = p

			if tt.btInstance.pod != nil {
				p.coreClient.EXPECT().DeletePod(mock.Anything, tt.btInstance.pod.Namespace, tt.btInstance.pod.Name).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulatePodDeleteError)
					})
			}

			err := tt.btInstance.Delete(ctx)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
