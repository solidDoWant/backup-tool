package externalsnapshotter

import (
	"context"

	"github.com/gravitational/trace"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	SnapshotVolume(context.Context, string, string, SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error)
	WaitForReadySnapshot(ctx context.Context, namespace, name string, opts WaitForReadySnapshotOpts) error
	DeleteSnapshot(context.Context, string, string) error
}

type Client struct {
	client versioned.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingExternalSnapshotterClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create external-snapshotter client")
	}

	return &Client{
		client: underlyingExternalSnapshotterClient,
	}, nil
}
