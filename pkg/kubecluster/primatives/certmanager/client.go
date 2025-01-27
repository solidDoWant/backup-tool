package certmanager

import (
	"context"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cert-manager/cert-manager/pkg/client/clientset/versioned"
	"github.com/gravitational/trace"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	CreateCertificate(ctx context.Context, namespace, name, issuerName string, opts CreateCertificateOptions) (*certmanagerv1.Certificate, error)
	WaitForReadyCertificate(ctx context.Context, namespace, name string, opts WaitForReadyCertificateOpts) (*certmanagerv1.Certificate, error)
	DeleteCertificate(ctx context.Context, namespace, name string) error
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
