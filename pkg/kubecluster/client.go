package kubecluster

import (
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforcertificate"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
)

// Convert types so that they are not exported when embedded
type backupToolInstanceProvider backuptoolinstance.ProviderInterface
type clonedClusterProvider clonedcluster.ProviderInterface
type clonePVCProvider clonepvc.ProviderInterface
type clusterUserCertProvider clusterusercert.ProviderInterface
type createCRPForProfileProvider createcrpforcertificate.ProviderInterface
type drVolumeProvider drvolume.ProviderInterface

type ClientInterface interface {
	CM() certmanager.ClientInterface
	CNPG() cnpg.ClientInterface
	ES() externalsnapshotter.ClientInterface
	Core() core.ClientInterface
	AP() approverpolicy.ClientInterface
	backupToolInstanceProvider
	clonedClusterProvider
	clonePVCProvider
	clusterUserCertProvider
	createCRPForProfileProvider
	drVolumeProvider
}

type Client struct {
	cmClient   certmanager.ClientInterface
	cnpgClient cnpg.ClientInterface
	esClient   externalsnapshotter.ClientInterface
	coreClient core.ClientInterface
	apClient   approverpolicy.ClientInterface
	backupToolInstanceProvider
	clonedClusterProvider
	clonePVCProvider
	clusterUserCertProvider
	createCRPForProfileProvider
	drVolumeProvider
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

func NewClient(cm certmanager.ClientInterface, cnpg cnpg.ClientInterface, esClient externalsnapshotter.ClientInterface, coreClient core.ClientInterface, apClient approverpolicy.ClientInterface) *Client {
	c := &Client{
		cmClient:   cm,
		cnpgClient: cnpg,
		esClient:   esClient,
		coreClient: coreClient,
		apClient:   apClient,
	}

	c.backupToolInstanceProvider = backuptoolinstance.NewProvider(coreClient)
	c.clonePVCProvider = clonepvc.NewProvider(coreClient, esClient)
	c.createCRPForProfileProvider = createcrpforcertificate.NewProvider(apClient)
	c.clusterUserCertProvider = clusterusercert.NewProvider(c.createCRPForProfileProvider, apClient, cm)
	c.clonedClusterProvider = clonedcluster.NewProvider(c.clusterUserCertProvider, cm, cnpg)
	c.drVolumeProvider = drvolume.NewProvider(c.coreClient, c.esClient, c.cnpgClient)

	return c
}
