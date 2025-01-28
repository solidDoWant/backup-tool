package clusterusercert

import (
	context "context"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforcertificate"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
)

type ProviderInterface interface {
	NewClusterUserCert(ctx context.Context, namespace, username, issuerName, clusterName string, opts NewClusterUserCertOpts) (*ClusterUserCert, error)
}

type providerInterfaceInternal interface {
	ProviderInterface
	ap() approverpolicy.ClientInterface
	cm() certmanager.ClientInterface
}

type Provider struct {
	ccfp               createcrpforcertificate.ProviderInterface
	apClient           approverpolicy.ClientInterface
	cmClient           certmanager.ClientInterface
	newClusterUserCert func() ClusterUserCertInterface
}

func NewProvider(ccfp createcrpforcertificate.ProviderInterface, apClient approverpolicy.ClientInterface, cmClient certmanager.ClientInterface) *Provider {
	p := &Provider{
		ccfp:     ccfp,
		apClient: apClient,
		cmClient: cmClient,
	}

	p.newClusterUserCert = func() ClusterUserCertInterface {
		return newClusterUserCert(p)
	}

	return p
}

func (p *Provider) ap() approverpolicy.ClientInterface {
	return p.apClient
}

func (p *Provider) cm() certmanager.ClientInterface {
	return p.cmClient
}
