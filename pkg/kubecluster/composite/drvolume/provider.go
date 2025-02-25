package drvolume

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ProviderInterface interface {
	NewDRVolume(ctx *contexts.Context, namespace, name string, configuredSize resource.Quantity, opts DRVolumeCreateOptions) (DRVolumeInterface, error)
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
