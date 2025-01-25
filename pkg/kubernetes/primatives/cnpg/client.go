package cnpg

import (
	"context"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubernetes/primatives/cnpg/gen/clientset/versioned"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	CreateBackup(ctx context.Context, namespace, backupName, clusterName string, opts CreateBackupOptions) (*apiv1.Backup, error)
	WaitForReadyBackup(ctx context.Context, namespace, name string, opts WaitForReadyBackupOpts) error
	DeleteBackup(ctx context.Context, namespace, name string) error
	CreateCluster(ctx context.Context, namespace, clusterName string, volumeSize resource.Quantity, servingCertificateSecretName, clientCASecretName string, opts CreateClusterOptions) (*apiv1.Cluster, error)
	WaitForReadyCluster(ctx context.Context, namespace, name string, opts WaitForReadyClusterOpts) error
	GetCluster(ctx context.Context, namespace, name string) (*apiv1.Cluster, error)
	DeleteCluster(ctx context.Context, namespace, name string) error
}

type Client struct {
	cnpgClient          versioned.Interface
	apiExtensionsClient apiextensionsclientset.Interface
}

func NewClient(k8sRESTClient rest.Interface) *Client {
	return &Client{
		cnpgClient:          versioned.New(k8sRESTClient),
		apiExtensionsClient: apiextensionsclientset.New(k8sRESTClient),
	}
}
