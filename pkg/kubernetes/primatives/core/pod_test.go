package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/kubernetes/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	kubetesting "k8s.io/client-go/testing"
)

func TestCreatePod(t *testing.T) {
	namespace := "test-ns"
	podName := "test-pod"

	tests := []struct {
		desc                string
		pod                 *corev1.Pod
		simulateClientError bool
	}{
		{
			desc: "create pod successfully",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
				},
			},
		},
		{
			desc:                "creation errors",
			simulateClientError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := context.Background()

			if tt.simulateClientError {
				mockK8s.PrependReactor("create", "pods", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			}

			pod, err := c.CreatePod(ctx, namespace, tt.pod)
			if tt.simulateClientError {
				assert.Error(t, err)
				assert.Nil(t, pod)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.pod, pod)
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
		afterStartedWaiting func(*testing.T, context.Context, k8s.Interface)
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
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client k8s.Interface) {
				_, err := client.CoreV1().Pods(podNamespace).Update(ctx, readyPod, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:       "multiple conditions",
			initialPod: notReadyPod,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client k8s.Interface) {
				_, err := client.CoreV1().Pods(podNamespace).Update(ctx, multipleConditionsPod, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			kc, mockK8s := createTestClient()
			ctx := context.Background()

			if tt.initialPod != nil {
				_, err := mockK8s.CoreV1().Pods(podNamespace).Create(ctx, tt.initialPod, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			wg.Add(1)
			go func() {
				waitErr = kc.WaitForReadyPod(ctx, podNamespace, podName, WaitForReadyPodOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, mockK8s)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				return
			}
			assert.NoError(t, waitErr)
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
			ctx := context.Background()

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
