package barmancloud

import (
	barmancloudv1 "github.com/cloudnative-pg/plugin-barman-cloud/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/barmancloud/gen/clientset/versioned"
	"k8s.io/client-go/rest"
)

// ClientInterface exposes the subset of the barman-cloud plugin API
// (barmancloud.cnpg.io) that the backup tool needs. Currently this is limited
// to reading ObjectStore resources so that a cloned cluster can reference the
// same WAL archive as its source.
type ClientInterface interface {
	GetObjectStore(ctx *contexts.Context, namespace, name string) (*barmancloudv1.ObjectStore, error)
}

type Client struct {
	barmanCloudClient versioned.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create barman-cloud plugin client")
	}

	return &Client{barmanCloudClient: underlyingClient}, nil
}
