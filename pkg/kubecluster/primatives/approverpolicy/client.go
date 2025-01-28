package approverpolicy

import (
	"context"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy/gen/clientset/versioned"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	CreateCertificateRequestPolicy(ctx context.Context, name string, spec policyv1alpha1.CertificateRequestPolicySpec, opts CreateCertificateRequestPolicyOptions) (*policyv1alpha1.CertificateRequestPolicy, error)
	WaitForReadyCertificateRequestPolicy(ctx context.Context, name string, opts WaitForReadyCertificateRequestPolicyOpts) (*policyv1alpha1.CertificateRequestPolicy, error)
	DeleteCertificateRequestPolicy(ctx context.Context, name string) error
}

type Client struct {
	client versioned.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create approver-policy client")
	}

	return &Client{
		client: underlyingClient,
	}, nil
}
