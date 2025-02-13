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
	"k8s.io/apimachinery/pkg/util/intstr"
	k8s "k8s.io/client-go/kubernetes"
	kubetesting "k8s.io/client-go/testing"
)

func TestContainerPortToServicePort(t *testing.T) {
	tests := []struct {
		name          string
		containerPort corev1.ContainerPort
		want          corev1.ServicePort
	}{
		{
			name: "with port name",
			containerPort: corev1.ContainerPort{
				Name:          "http",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: 8080,
			},
			want: corev1.ServicePort{
				Name:       "http",
				Protocol:   corev1.ProtocolTCP,
				Port:       8080,
				TargetPort: intstr.FromString("http"),
			},
		},
		{
			name: "without port name",
			containerPort: corev1.ContainerPort{
				Protocol:      corev1.ProtocolUDP,
				ContainerPort: 9090,
			},
			want: corev1.ServicePort{
				Protocol:   corev1.ProtocolUDP,
				Port:       9090,
				TargetPort: intstr.FromInt(9090),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainerPortToServicePort(tt.containerPort)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCreateService(t *testing.T) {
	namespace := "test-ns"
	serviceName := "test-service"
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}

	tests := []struct {
		desc                string
		simulateClientError bool
	}{
		{
			desc: "create service successfully",
		},
		{
			desc:                "creation errors",
			simulateClientError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.simulateClientError {
				mockK8s.PrependReactor("create", "services", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}

			createdService, err := c.CreateService(ctx, namespace, service)
			if tt.simulateClientError {
				assert.Error(t, err)
				assert.Nil(t, createdService)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, service, createdService)
		})
	}
}

func TestWaitForReadyService(t *testing.T) {
	serviceName := "test-service"
	namespace := "test-ns"

	baseService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
	}

	loadBalancerService := baseService.DeepCopy()
	loadBalancerService.Spec.Type = corev1.ServiceTypeLoadBalancer

	readyLoadBalancerService := loadBalancerService.DeepCopy()
	readyLoadBalancerService.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "192.168.1.1"},
	}

	clusterIPService := baseService.DeepCopy()
	clusterIPService.Spec.Type = corev1.ServiceTypeClusterIP

	readyClusterIPService := clusterIPService.DeepCopy()
	readyClusterIPService.Spec.ClusterIP = "10.0.0.1"
	readyClusterIPService.Status = corev1.ServiceStatus{}

	notReadyEndpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
	}

	readyEndpoints := notReadyEndpoints.DeepCopy()
	readyEndpoints.Subsets = []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{IP: "172.16.0.1"},
			},
		},
	}

	tests := []struct {
		desc                string
		initialService      *corev1.Service
		initialEndpoints    *corev1.Endpoints
		shouldError         bool
		afterStartedWaiting func(*testing.T, *contexts.Context, k8s.Interface)
	}{
		{
			desc:             "service starts ready and endpoints exist",
			initialService:   readyLoadBalancerService,
			initialEndpoints: readyEndpoints,
		},
		{
			desc:           "service starts ready but endpoints dont exist",
			initialService: readyLoadBalancerService,
			shouldError:    true,
		},
		{
			desc:             "service starts ready but endpoints never become ready",
			initialService:   readyLoadBalancerService,
			initialEndpoints: notReadyEndpoints,
			shouldError:      true,
		},
		{
			desc:        "service does not exist",
			shouldError: true,
		},
		{
			desc:             "loadbalancer service becomes ready",
			initialService:   loadBalancerService,
			initialEndpoints: readyEndpoints,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Services(namespace).Update(ctx, readyLoadBalancerService, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:             "clusterip service becomes ready",
			initialService:   clusterIPService,
			initialEndpoints: readyEndpoints,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Services(namespace).Update(ctx, readyClusterIPService, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:           "loadbalancer service not ready",
			initialService: loadBalancerService,
			shouldError:    true,
		},
		{
			desc:           "clusterip service not ready",
			initialService: clusterIPService,
			shouldError:    true,
		},
		{
			desc:             "service and endpoints become ready",
			initialService:   clusterIPService,
			initialEndpoints: notReadyEndpoints,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client k8s.Interface) {
				_, err := client.CoreV1().Services(namespace).Update(ctx, readyClusterIPService, metav1.UpdateOptions{})
				require.NoError(t, err)
				_, err = client.CoreV1().Endpoints(namespace).Update(ctx, readyEndpoints, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialService != nil {
				_, err := mockK8s.CoreV1().Services(namespace).Create(ctx, tt.initialService, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.initialEndpoints != nil {
				_, err := mockK8s.CoreV1().Endpoints(namespace).Create(ctx, tt.initialEndpoints, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var service *corev1.Service
			wg.Add(1)
			go func() {
				service, waitErr = c.WaitForReadyService(ctx, namespace, serviceName, WaitForReadyServiceOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure watcher is setup
				tt.afterStartedWaiting(t, ctx, mockK8s)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, service)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, service)
		})
	}
}

func TestDeleteService(t *testing.T) {
	namespace := "test-ns"
	serviceName := "test-service"

	tests := []struct {
		desc           string
		initialService *corev1.Service
		wantErr        bool
	}{
		{
			desc: "delete existing service",
			initialService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
			},
		},
		{
			desc:    "delete non-existent service",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialService != nil {
				_, err := mockK8s.CoreV1().Services(namespace).Create(ctx, tt.initialService, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := c.DeleteService(ctx, namespace, serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the service was deleted
			svcList, err := mockK8s.CoreV1().Services(namespace).List(ctx, metav1.SingleObject(tt.initialService.ObjectMeta))
			assert.NoError(t, err)
			assert.Empty(t, svcList.Items)
		})
	}
}
