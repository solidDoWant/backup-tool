package externalsnapshotter

import (
	"context"

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

func NewClient(k8sRESTClient rest.Interface) *Client {
	return &Client{
		client: versioned.New(k8sRESTClient),
	}
}
