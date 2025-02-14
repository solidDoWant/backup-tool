package certmanager

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cert-manager/cert-manager/pkg/client/clientset/versioned"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	// Issuers
	CreateIssuer(ctx *contexts.Context, namespace, name, caCertSecretName string, opts CreateIssuerOptions) (*certmanagerv1.Issuer, error)
	WaitForReadyIssuer(ctx *contexts.Context, namespace, name string, opts WaitForReadyIssuerOpts) (*certmanagerv1.Issuer, error)
	GetIssuer(ctx *contexts.Context, namespace, name string) (*certmanagerv1.Issuer, error)
	DeleteIssuer(ctx *contexts.Context, namespace, name string) error
	// Certificates
	CreateCertificate(ctx *contexts.Context, namespace, name, issuerName string, opts CreateCertificateOptions) (*certmanagerv1.Certificate, error)
	WaitForReadyCertificate(ctx *contexts.Context, namespace, name string, opts WaitForReadyCertificateOpts) (*certmanagerv1.Certificate, error)
	ReissueCertificate(ctx *contexts.Context, namespace, name string) (*certmanagerv1.Certificate, error)
	GetCertificate(ctx *contexts.Context, namespace, name string) (*certmanagerv1.Certificate, error)
	DeleteCertificate(ctx *contexts.Context, namespace, name string) error
}

type Client struct {
	client versioned.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create cert-manager client")
	}

	return &Client{
		client: underlyingClient,
	}, nil
}
