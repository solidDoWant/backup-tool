package core

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) CreatePod(ctx context.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	pod, err := c.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create pod %q", helpers.FullNameStr(namespace, pod.Name))
	}

	return pod, nil
}

type WaitForReadyPodOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadyPod(ctx context.Context, namespace, name string, opts WaitForReadyPodOpts) (*corev1.Pod, error) {
	processEvent := func(_ context.Context, pod *corev1.Pod) (*corev1.Pod, bool, error) {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				if condition.Status == corev1.ConditionTrue {
					return pod, true, nil
				}
				return nil, false, nil
			}
		}
		return nil, false, nil
	}
	pod, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(time.Minute), c.client.CoreV1().Pods(namespace), name, processEvent)

	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for pod to become ready")
	}

	return pod, nil
}

func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	err := c.client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete pod %q", helpers.FullNameStr(namespace, name))
}
