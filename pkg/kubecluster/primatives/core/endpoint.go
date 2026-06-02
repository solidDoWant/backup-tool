package core

import (
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) GetEndpoint(ctx *contexts.Context, namespace, name string) (*discoveryv1.EndpointSlice, error) {
	ctx.Log.With("name", name).Info("Getting endpoint")

	endpoint, err := c.client.DiscoveryV1().EndpointSlices(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to get endpoint %q", helpers.FullNameStr(namespace, name))
	}

	ctx.Log.Debug("Retrieved endpoint", "endpoint", endpoint)
	return endpoint, nil
}

type WaitForReadyEndpointOpts struct {
	helpers.MaxWaitTime
}

// Wait for at least one ready endpoint to be available.
func (c *Client) WaitForReadyEndpoint(ctx *contexts.Context, namespace, name string, opts WaitForReadyEndpointOpts) (endpoints *discoveryv1.EndpointSlice, err error) {
	ctx.Log.With("name", name).Info("Waiting for endpoint to become ready")
	defer ctx.Log.Info("Finished waiting for endpoint to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	processEvent := func(_ *contexts.Context, endpoint *discoveryv1.EndpointSlice) (*discoveryv1.EndpointSlice, bool, error) {
		for _, subset := range endpoint.Endpoints {
			for _, address := range subset.Addresses {
				if address != "" {
					return endpoint, true, nil
				}
			}
		}

		return nil, false, nil
	}
	endpoints, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(5*time.Minute), c.client.DiscoveryV1().EndpointSlices(namespace), name, processEvent)

	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for endpoint to become ready")
	}

	return endpoints, nil
}
