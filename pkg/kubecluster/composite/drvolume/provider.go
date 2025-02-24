package drvolume

import (
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
)

type ProviderInterface interface {
	// NewClusterUserCert(ctx *contexts.Context, namespace, username, issuerName, clusterName string, opts NewClusterUserCertOpts) (ClusterUserCertInterface, error)
}

type providerInterfaceInternal interface {
	ProviderInterface
	core() core.ClientInterface
	es() externalsnapshotter.ClientInterface
	cnpg() cnpg.ClientInterface
}

type Provider struct {
	coreClient  core.ClientInterface
	esClient    externalsnapshotter.ClientInterface
	cnpgClient  cnpg.ClientInterface
	newDRVolume func() DRVolumeInterface
}

func NewProvider(coreClient core.ClientInterface, esClient externalsnapshotter.ClientInterface, cnpgClient cnpg.ClientInterface) *Provider {
	p := &Provider{
		coreClient: coreClient,
		esClient:   esClient,
		cnpgClient: cnpgClient,
	}

	p.newDRVolume = func() DRVolumeInterface {
		return newDRVolume(p)
	}

	return p
}

func (p *Provider) core() core.ClientInterface {
	return p.coreClient
}

func (p *Provider) es() externalsnapshotter.ClientInterface {
	return p.esClient
}

func (p *Provider) cnpg() cnpg.ClientInterface {
	return p.cnpgClient
}
