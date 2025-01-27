package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	kubetesting "k8s.io/client-go/testing"
)

func TestGetEndpoint(t *testing.T) {
	namespace := "test-ns"
	endpointName := "test-endpoint"

	tests := []struct {
		desc                string
		endpoint            *corev1.Endpoints
		simulateClientError bool
	}{
		{
			desc: "get endpoint successfully",
			endpoint: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      endpointName,
					Namespace: namespace,
				},
			},
		},
		{
			desc:                "get errors",
			simulateClientError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := context.Background()

			if tt.endpoint != nil {
				_, err := mockK8s.CoreV1().Endpoints(namespace).Create(ctx, tt.endpoint, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.simulateClientError {
				mockK8s.PrependReactor("get", "endpoints", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}

			endpoint, err := c.GetEndpoint(ctx, namespace, endpointName)
			if tt.simulateClientError {
				assert.Error(t, err)
				assert.Nil(t, endpoint)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.endpoint, endpoint)
		})
	}
}

func TestWaitForReadyEndpoint(t *testing.T) {
	endpointName := "test-endpoint"
	namespace := "test-ns"

	noSubsetEndpoint := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointName,
			Namespace: namespace,
		},
	}

	emptySubsetEndpoint := noSubsetEndpoint.DeepCopy()
	emptySubsetEndpoint.Subsets = []corev1.EndpointSubset{{}}

	noIPEndpoint := emptySubsetEndpoint.DeepCopy()
	noIPEndpoint.Subsets[0] = corev1.EndpointSubset{
		Addresses: []corev1.EndpointAddress{{}},
	}

	readyEndpoint := noIPEndpoint.DeepCopy()
	readyEndpoint.Subsets[0].Addresses[0].IP = "192.168.1.1"

	tests := []struct {
		desc                string
		initialEndpoint     *corev1.Endpoints
		shouldError         bool
		afterStartedWaiting func(*testing.T, context.Context, k8s.Interface)
	}{
		{
			desc:            "endpoint starts ready",
			initialEndpoint: readyEndpoint,
		},
		{
			desc:            "endpoint has no subsets",
			initialEndpoint: noSubsetEndpoint,
			shouldError:     true,
		},
		{
			desc:            "endpoint has empty subset",
			initialEndpoint: emptySubsetEndpoint,
			shouldError:     true,
		},
		{
			desc:            "endpoint has no IP",
			initialEndpoint: noIPEndpoint,
			shouldError:     true,
		},
		{
			desc:        "endpoint does not exist",
			shouldError: true,
		},
		{
			desc:            "endpoint becomes ready",
			initialEndpoint: noIPEndpoint,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client k8s.Interface) {
				_, err := client.CoreV1().Endpoints(namespace).Update(ctx, readyEndpoint, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			c, mockK8s := createTestClient()
			ctx := context.Background()

			if tt.initialEndpoint != nil {
				_, err := mockK8s.CoreV1().Endpoints(namespace).Create(ctx, tt.initialEndpoint, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var endpoints *corev1.Endpoints
			wg.Add(1)
			go func() {
				endpoints, waitErr = c.WaitForReadyEndpoint(ctx, namespace, endpointName, WaitForReadyEndpointOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, mockK8s)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, endpoints)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, endpoints)
		})
	}
}
