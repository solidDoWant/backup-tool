package kubecluster

import (
	context "context"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
)

type ClientInterface interface {
	CM() certmanager.ClientInterface
	CNPG() cnpg.ClientInterface
	ES() externalsnapshotter.ClientInterface
	Core() core.ClientInterface
	AP() approverpolicy.ClientInterface
	CreateBackupToolInstance(ctx context.Context, namespace, instance string, opts CreateBackupToolInstanceOptions) (BackupToolInstanceInterface, error)
	CloneCluster(ctx context.Context, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName string, opts CloneClusterOptions) (cluster ClonedClusterInterface, err error)
	ClonePVC(ctx context.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error)
}

type Client struct {
	cmClient              certmanager.ClientInterface
	cnpgClient            cnpg.ClientInterface
	esClient              externalsnapshotter.ClientInterface
	coreClient            core.ClientInterface
	apClient              approverpolicy.ClientInterface
	newClonedCluster      func() ClonedClusterInterface
	newBackupToolInstance func() BackupToolInstanceInterface
}

func (c *Client) CM() certmanager.ClientInterface {
	return c.cmClient
}

func (c *Client) CNPG() cnpg.ClientInterface {
	return c.cnpgClient
}

func (c *Client) ES() externalsnapshotter.ClientInterface {
	return c.esClient
}

func (c *Client) Core() core.ClientInterface {
	return c.coreClient
}

func (c *Client) AP() approverpolicy.ClientInterface {
	return c.apClient
}

func NewClient(cm certmanager.ClientInterface, cnpg cnpg.ClientInterface, esClient externalsnapshotter.ClientInterface, sClient core.ClientInterface, apClient approverpolicy.ClientInterface) ClientInterface {
	c := &Client{
		cmClient:   cm,
		cnpgClient: cnpg,
		esClient:   esClient,
		coreClient: sClient,
		apClient:   apClient,
	}
	c.newClonedCluster = func() ClonedClusterInterface {
		return newClonedCluster(c)
	}
	c.newBackupToolInstance = func() BackupToolInstanceInterface {
		return newBackupToolInstance(c)
	}

	return c
}
