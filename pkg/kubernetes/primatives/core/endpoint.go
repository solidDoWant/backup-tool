package core

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubernetes/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) GetEndpoint(ctx context.Context, namespace, name string) (*corev1.Endpoints, error) {
	endpoint, err := c.client.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to get endpoint %q", helpers.FullNameStr(namespace, name))
	}

	return endpoint, nil
}

type WaitForReadyEndpointOpts struct {
	helpers.MaxWaitTime
}

// Wait for at least one ready endpoint to be available.
func (c *Client) WaitForReadyEndpoint(ctx context.Context, namespace, name string, opts WaitForReadyEndpointOpts) error {
	_, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(5*time.Minute), c.client.CoreV1().Endpoints(namespace), name,
		func(_ context.Context, endpoint *corev1.Endpoints) (interface{}, bool, error) {
			for _, subset := range endpoint.Subsets {
				for _, address := range subset.Addresses {
					if address.IP != "" {
						return nil, true, nil
					}
				}
			}

			return nil, false, nil
		},
	)

	if err != nil {
		return trace.Wrap(err, "failed waiting for endpoint to become ready")
	}

	return nil
}
