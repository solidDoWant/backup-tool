package backuptoolinstance

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
)

type ProviderInterface interface {
	CreateBackupToolInstance(ctx *contexts.Context, namespace, instance string, opts CreateBackupToolInstanceOptions) (btInstance BackupToolInstanceInterface, err error)
}

type providerInterfaceInternal interface {
	ProviderInterface
	core() core.ClientInterface
}

type Provider struct {
	coreClient            core.ClientInterface
	newBackupToolInstance func() BackupToolInstanceInterface
}

func NewProvider(coreClient core.ClientInterface) *Provider {
	p := &Provider{
		coreClient: coreClient,
	}
	p.newBackupToolInstance = func() BackupToolInstanceInterface {
		return newBackupToolInstance(p)
	}

	return p
}

func (p *Provider) core() core.ClientInterface {
	return p.coreClient
}
