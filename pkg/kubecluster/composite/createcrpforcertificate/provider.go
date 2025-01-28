package createcrpforcertificate

import (
	context "context"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
)

type ProviderInterface interface {
	CreateCRPForCertificate(ctx context.Context, cert *certmanagerv1.Certificate, opts CreateCRPForCertificateOpts) (*policyv1alpha1.CertificateRequestPolicy, error)
}

type Provider struct {
	apClient approverpolicy.ClientInterface
}

func NewProvider(apClient approverpolicy.ClientInterface) *Provider {
	return &Provider{apClient: apClient}
}
