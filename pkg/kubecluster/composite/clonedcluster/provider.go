package clonedcluster

import (
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/barmancloud"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
)

type ProviderInterface interface {
	// CreateClusterBackup and CloneClusterFromBackup are the two halves of cloning a cluster, split so
	// callers that need a wall-clock recovery target can take the base backup early (before the other
	// captures, which fixes the consistency point) and create the recovering clone later (once the
	// target time is known). The caller owns the returned backup's lifecycle and must DeleteBackup it
	// after the clone is created, since the volume snapshots are owned by the backup.
	CreateClusterBackup(ctx *contexts.Context, namespace, existingClusterName string, opts CloneClusterOptions) (*apiv1.Backup, error)
	CloneClusterFromBackup(ctx *contexts.Context, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName string, backup *apiv1.Backup, opts CloneClusterOptions) (cluster ClonedClusterInterface, err error)
}

type providerInterfaceInternal interface {
	ProviderInterface
	cm() certmanager.ClientInterface
	cnpg() cnpg.ClientInterface
}

type Provider struct {
	cucp              clusterusercert.ProviderInterface
	cmClient          certmanager.ClientInterface
	cnpgClient        cnpg.ClientInterface
	barmanCloudClient barmancloud.ClientInterface
	coreClient        core.ClientInterface
	newClonedCluster  func() ClonedClusterInterface
}

func NewProvider(cucp clusterusercert.ProviderInterface, cmClient certmanager.ClientInterface, cnpgClient cnpg.ClientInterface, barmanCloudClient barmancloud.ClientInterface, coreClient core.ClientInterface) *Provider {
	p := &Provider{
		cucp:              cucp,
		cmClient:          cmClient,
		cnpgClient:        cnpgClient,
		barmanCloudClient: barmanCloudClient,
		coreClient:        coreClient,
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
