package cnpg

import (
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	CreateBackup(ctx *contexts.Context, namespace, backupName, clusterName string, opts CreateBackupOptions) (*apiv1.Backup, error)
	WaitForReadyBackup(ctx *contexts.Context, namespace, name string, opts WaitForReadyBackupOpts) (*apiv1.Backup, error)
	DeleteBackup(ctx *contexts.Context, namespace, name string) error
	CreateCluster(ctx *contexts.Context, namespace, clusterName string, volumeSize resource.Quantity, servingCertificateSecretName, clientCASecretName, replicationUserCertName string, opts CreateClusterOptions) (*apiv1.Cluster, error)
	WaitForReadyCluster(ctx *contexts.Context, namespace, name string, opts WaitForReadyClusterOpts) (*apiv1.Cluster, error)
	GetCluster(ctx *contexts.Context, namespace, name string) (*apiv1.Cluster, error)
	DeleteCluster(ctx *contexts.Context, namespace, name string) error
}

type Client struct {
	cnpgClient          versioned.Interface
	apiExtensionsClient apiextensionsclientset.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingCNPGClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create cloudnative-pg client")
	}

	underlyingAPIExtensionsClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create apiextensions client")
	}

	return &Client{
		cnpgClient:          underlyingCNPGClient,
		apiExtensionsClient: underlyingAPIExtensionsClient,
	}, nil
}
