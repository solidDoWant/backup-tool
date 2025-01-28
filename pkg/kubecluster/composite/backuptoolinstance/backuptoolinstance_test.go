package backuptoolinstance

import (
	context "context"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/grpc/servers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func TestNewSingleContainerPVC(t *testing.T) {
	tests := []struct {
		name      string
		pvcName   string
		mountPath string
		want      SingleContainerVolume
	}{
		{
			name:      "basic pvc volume",
			pvcName:   "test-pvc",
			mountPath: "/data",
			want: SingleContainerVolume{
				Name:      "test-pvc",
				MountPath: "/data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
					},
				},
			},
		},
		{
			name: "empty paths",
			want: SingleContainerVolume{
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSingleContainerPVC(tt.pvcName, tt.mountPath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewSingleContainerSecret(t *testing.T) {
	tests := []struct {
		name       string
		secretName string
		mountPath  string
		want       SingleContainerVolume
	}{
		{
			name:       "basic secret volume",
			secretName: "test-secret",
			mountPath:  "/secrets",
			want: SingleContainerVolume{
				Name:      "test-secret",
				MountPath: "/secrets",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  "test-secret",
						DefaultMode: ptr.To(int32(0400)),
					},
				},
			},
		},
		{
			name: "empty paths",
			want: SingleContainerVolume{
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						DefaultMode: ptr.To(int32(0400)),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSingleContainerSecret(tt.secretName, tt.mountPath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewBackupToolInstance(t *testing.T) {
	c := newMockProvider(t)
	btInstance := newBackupToolInstance(c)
	casted := btInstance.(*BackupToolInstance)

	assert.Equal(t, c, casted.p)
	assert.Equal(t, reflect.ValueOf(net.LookupIP), reflect.ValueOf(casted.lookupIP))

	// Test the default testConnection function
	listener, err := net.Listen("tcp", net.JoinHostPort("localhost", fmt.Sprintf("%d", servers.GRPCPort)))
	require.NoError(t, err)
	defer listener.Close()

	assert.True(t, casted.testConnection("localhost"))
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
		simulateCreateServiceError             bool
		simulateWaitForServiceError            bool
	}{
		{
			name: "basic instance creation",
		},
		{
			name: "basic instance creation with all options set",
			opts: CreateBackupToolInstanceOptions{
				NamePrefix: "test-prefix-",
				Volumes: []SingleContainerVolume{
					{
						Name:      "vol1",
						MountPath: "/data1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc1",
							},
						},
					},
					{
						Name:      "vol2",
						MountPath: "/data2",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc2",
							},
						},
					},

					{
						Name:      "vol3",
						MountPath: "/secret1",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  "secret1",
								DefaultMode: ptr.To(int32(0765)),
							},
						},
					},
				},
				CleanupTimeout:     helpers.ShortWaitTime,
				ServiceType:        corev1.ServiceTypeNodePort,
				PodWaitTimeout:     helpers.ShortWaitTime,
				ServiceWaitTimeout: helpers.ShortWaitTime,
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
		{
			name:                       "simulate create service error",
			simulateCreateServiceError: true,
		},
		{
			name:                        "simulate wait for service error",
			simulateWaitForServiceError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newMockProvider(t)
			ctx := context.Background()

			errExpected := th.ErrExpected(
				tt.simulateBackupToolInstanceCleanupError,
				tt.simulateCreatePodError,
				tt.simulateWaitForPodError,
				tt.simulateCreateServiceError,
				tt.simulateWaitForServiceError,
			)

			func() {
				if errExpected {
					p.backupToolInstance.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx context.Context) error {
						require.NotEqual(t, ctx, cleanupCtx)
						return th.ErrIfTrue(tt.simulateBackupToolInstanceCleanupError)
					})
				}

				var createdPod *corev1.Pod
				p.coreClient.EXPECT().CreatePod(ctx, namespace, mock.Anything).RunAndReturn(func(_ context.Context, _ string, pod *corev1.Pod) (*corev1.Pod, error) {
					createdPod = pod

					require.Len(t, pod.Spec.Containers, 1)
					require.Equal(t, len(tt.opts.Volumes), len(pod.Spec.Volumes))
					require.Contains(t, pod.ObjectMeta.Labels, "app.kubernetes.io/name")
					require.Contains(t, pod.ObjectMeta.Labels, "app.kubernetes.io/instance")

					container := pod.Spec.Containers[0]
					require.Equal(t, constants.ToolName, container.Name)
					require.Equal(t, constants.FullImageName, container.Image)
					require.Equal(t, []string{constants.ToolName}, container.Command)
					require.Equal(t, len(tt.opts.Volumes), len(container.VolumeMounts))
					require.Len(t, container.Ports, 1)

					port := container.Ports[0]
					require.Equal(t, "grpc", port.Name)
					require.Equal(t, int32(servers.GRPCPort), port.ContainerPort)
					require.Equal(t, corev1.ProtocolTCP, port.Protocol)

					return th.ErrOr1Val(pod, tt.simulateCreatePodError)
				})
				if tt.simulateCreatePodError {
					return
				}
				p.backupToolInstance.EXPECT().setPod(mock.Anything).Run(func(pod *corev1.Pod) {
					require.Equal(t, createdPod, pod)
				})

				p.coreClient.EXPECT().WaitForReadyPod(ctx, namespace, mock.Anything, core.WaitForReadyPodOpts{MaxWaitTime: tt.opts.PodWaitTimeout}).
					RunAndReturn(func(ctx context.Context, s1, s2 string, wfrpo core.WaitForReadyPodOpts) (*corev1.Pod, error) {
						return th.ErrOr1Val(createdPod, tt.simulateWaitForPodError)
					})
				if tt.simulateWaitForPodError {
					return
				}

				var createdService *corev1.Service
				p.coreClient.EXPECT().CreateService(ctx, namespace, mock.Anything).RunAndReturn(func(_ context.Context, _ string, service *corev1.Service) (*corev1.Service, error) {
					createdService = service
					require.Equal(t, createdPod.ObjectMeta.Labels, service.Spec.Selector)

					require.Len(t, service.Spec.Ports, 1)
					port := service.Spec.Ports[0]
					require.Equal(t, "grpc", port.Name)
					require.Equal(t, int32(servers.GRPCPort), port.Port)
					require.Equal(t, intstr.FromString("grpc"), port.TargetPort)
					require.Equal(t, corev1.ProtocolTCP, port.Protocol)

					return th.ErrOr1Val(service, tt.simulateCreateServiceError)
				})
				if tt.simulateCreateServiceError {
					return
				}
				p.backupToolInstance.EXPECT().setService(mock.Anything).Run(func(service *corev1.Service) {
					require.Equal(t, createdService, service)
				})

				p.coreClient.EXPECT().WaitForReadyService(ctx, namespace, mock.Anything, core.WaitForReadyServiceOpts{MaxWaitTime: tt.opts.ServiceWaitTimeout}).
					Return(th.ErrOr1Val(&corev1.Service{}, tt.simulateWaitForServiceError))
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

func TestBackupToolInstanceSetService(t *testing.T) {
	tests := []struct {
		desc    string
		service *corev1.Service
	}{
		{
			desc: "set non-nil service",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			btInstance := &BackupToolInstance{}
			btInstance.setService(tt.service)
			assert.Equal(t, tt.service, btInstance.service)
		})
	}
}

func TestBackupToolInstanceGetService(t *testing.T) {
	tests := []struct {
		desc       string
		btInstance BackupToolInstance
		want       *corev1.Service
	}{
		{
			desc: "get existing service",
			btInstance: BackupToolInstance{
				service: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "test-ns",
					},
				},
			},
			want: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.btInstance.GetService()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBackupToolInstanceDelete(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns"},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "test-ns"},
	}

	allResourcesInstance := BackupToolInstance{
		pod:     pod,
		service: service,
	}

	tests := []struct {
		desc                       string
		btInstance                 BackupToolInstance
		simulatePodDeleteError     bool
		simulateServiceDeleteError bool
		expectedErrorsInMessage    int
	}{
		{
			desc:       "delete all resources",
			btInstance: allResourcesInstance,
		},
		{
			desc: "delete with just pod",
			btInstance: BackupToolInstance{
				pod: pod,
			},
		},
		{
			desc: "delete with just service",
			btInstance: BackupToolInstance{
				service: service,
			},
		},
		{
			desc: "delete empty backup tool instance",
		},
		{
			desc:                       "all deletions fail",
			btInstance:                 allResourcesInstance,
			simulatePodDeleteError:     true,
			simulateServiceDeleteError: true,
			expectedErrorsInMessage:    2,
		},
		{
			desc:                    "pod deletion fails",
			btInstance:              allResourcesInstance,
			simulatePodDeleteError:  true,
			expectedErrorsInMessage: 1,
		},
		{
			desc:                       "service deletion fails",
			btInstance:                 allResourcesInstance,
			simulateServiceDeleteError: true,
			expectedErrorsInMessage:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.Background()
			p := newMockProvider(t)
			tt.btInstance.p = p

			if tt.btInstance.pod != nil {
				p.coreClient.EXPECT().DeletePod(ctx, tt.btInstance.pod.Namespace, tt.btInstance.pod.Name).Return(th.ErrIfTrue(tt.simulatePodDeleteError))
			}

			if tt.btInstance.service != nil {
				p.coreClient.EXPECT().DeleteService(ctx, tt.btInstance.service.Namespace, tt.btInstance.service.Name).Return(th.ErrIfTrue(tt.simulateServiceDeleteError))
			}

			err := tt.btInstance.Delete(ctx)
			if tt.expectedErrorsInMessage == 0 {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			if tErr, ok := err.(trace.Error); ok {
				if oErrs, ok := tErr.OrigError().(trace.Aggregate); ok {
					assert.Equal(t, tt.expectedErrorsInMessage, len(oErrs.Errors()))
				}
			} else {
				require.Fail(t, "error is not a trace.Error")
			}
		})
	}
}

func TestBackupToolInstanceFindReachableServiceAddress(t *testing.T) {
	noSucceedFunc := func(string) bool {
		return false
	}

	serviceClusterIP := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Type:       corev1.ServiceTypeClusterIP,
			ClusterIPs: []string{"1.2.3.4", "5.6.7.8"},
		},
	}

	serviceLoadBalancer := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{IP: "1.2.3.4"},
					{IP: "5.6.7.8"},
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}

	tests := []struct {
		desc            string
		searchDomains   []string
		service         *corev1.Service
		testConnFunc    func(string) bool
		expectedQueries []string
		want            string
		wantErr         bool
	}{
		{
			desc:         "test default queries made",
			service:      serviceClusterIP,
			testConnFunc: noSucceedFunc, // Run through all options
			expectedQueries: []string{
				"test-svc",
				"test-svc.test-ns",
				"test-svc.test-ns.svc",
				"test-svc.test-ns.svc.cluster.local",
			},
			wantErr: true,
		},
		{
			desc:          "test queries with search domains set",
			service:       serviceLoadBalancer,
			searchDomains: []string{"example.com", "test.local"},
			testConnFunc:  noSucceedFunc, // Run through all options
			expectedQueries: []string{
				"test-svc",
				"test-svc.test-ns",
				"test-svc.test-ns.svc",
				"test-svc.test-ns.svc.cluster.local",
				"test-svc.example.com",
				"test-svc.test-ns.example.com",
				"test-svc.test-ns.svc.example.com",
				"test-svc.test-ns.svc.cluster.local.example.com",
				"test-svc.test.local",
				"test-svc.test-ns.test.local",
				"test-svc.test-ns.svc.test.local",
				"test-svc.test-ns.svc.cluster.local.test.local",
			},
			wantErr: true,
		},
		{
			desc:          "find via DNS lookup",
			searchDomains: []string{"example.com", "test.local"},
			service:       serviceClusterIP,
			testConnFunc: func(addr string) bool {
				return addr == "127.0.0.12"
			},
			want: "test-svc.test-ns.svc.cluster.local",
		},
		{
			desc:    "find via cluster IP",
			service: serviceClusterIP,
			testConnFunc: func(addr string) bool {
				return addr == "5.6.7.8"
			},
			want: "5.6.7.8",
		},
		{
			desc:    "find via load balancer IP",
			service: serviceLoadBalancer,
			testConnFunc: func(addr string) bool {
				return addr == "5.6.7.8"
			},
			want: "5.6.7.8",
		},
		{
			desc: "no reachable address found",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "test-ns",
				},
			},
			testConnFunc: func(addr string) bool {
				return false
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			var testedConnections []string
			var queries []string

			btInstance := &BackupToolInstance{
				service: tt.service,
				testConnection: func(address string) bool {
					testedConnections = append(testedConnections, address)
					return tt.testConnFunc(address)
				},
				lookupIP: func(host string) ([]net.IP, error) {
					queries = append(queries, host)
					return []net.IP{
						net.ParseIP(fmt.Sprintf("127.0.0.%d", len(queries))),
					}, nil
				},
			}

			got, err := btInstance.findReachableServiceAddress(tt.searchDomains)

			if len(tt.expectedQueries) > 0 {
				assert.ElementsMatch(t, tt.expectedQueries, queries)
			}

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
