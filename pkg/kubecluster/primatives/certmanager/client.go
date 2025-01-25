package certmanager

import (
	"context"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cert-manager/cert-manager/pkg/client/clientset/versioned"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	CreateCertificate(ctx context.Context, name, namespace, issuerName string, opts CreateCertificateOptions) (*certmanagerv1.Certificate, error)
	WaitForReadyCertificate(ctx context.Context, namespace, name string, opts WaitForReadyCertificateOpts) error
	DeleteCertificate(ctx context.Context, name, namespace string) error
}

type Client struct {
	client versioned.Interface
}

func NewClient(k8sRESTClient rest.Interface) *Client {
	return &Client{
		client: versioned.New(k8sRESTClient),
	}
}
