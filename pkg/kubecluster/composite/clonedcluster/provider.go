package clonedcluster

import (
	context "context"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
)

type ProviderInterface interface {
	CloneCluster(ctx context.Context, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName string, opts CloneClusterOptions) (cluster ClonedClusterInterface, err error)
}

type providerInterfaceInternal interface {
	ProviderInterface
	cm() certmanager.ClientInterface
	cnpg() cnpg.ClientInterface
}

type Provider struct {
	cmClient         certmanager.ClientInterface
	cnpgClient       cnpg.ClientInterface
	newClonedCluster func() ClonedClusterInterface
}

func NewProvider(cmClient certmanager.ClientInterface, cnpgClient cnpg.ClientInterface) *Provider {
	p := &Provider{
		cmClient:   cmClient,
		cnpgClient: cnpgClient,
	}
	p.newClonedCluster = func() ClonedClusterInterface {
		return newClonedCluster(p)
	}

	return p
}

func (p *Provider) cnpg() cnpg.ClientInterface {
	return p.cnpgClient
}

func (p *Provider) cm() certmanager.ClientInterface {
	return p.cmClient
}
