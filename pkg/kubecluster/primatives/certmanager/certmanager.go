package certmanager

import (
	"context"
	"time"

	"github.com/cert-manager/cert-manager/pkg/apis/certmanager"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type CreateCertificateOptions struct {
	helpers.GenerateName
	CommonName   string
	DNSNames     []string
	Duration     *time.Duration
	IssuerKind   string // Default to "Issuer" (namespace-specific)
	SecretLabels map[string]string
	SecretName   string
	Subject      *certmanagerv1.X509Subject
	Usages       []certmanagerv1.KeyUsage
}

func (cmc *Client) CreateCertificate(ctx context.Context, name, namespace, issuerName string, opts CreateCertificateOptions) (*certmanagerv1.Certificate, error) {
	certificate := &certmanagerv1.Certificate{
		Spec: certmanagerv1.CertificateSpec{
			CommonName:            opts.CommonName,
			DNSNames:              opts.DNSNames,
			Subject:               opts.Subject,
			Usages:                opts.Usages,
			EncodeUsagesInRequest: ptr.To(true),
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm:      certmanagerv1.Ed25519KeyAlgorithm,
				Encoding:       certmanagerv1.PKCS8,
				RotationPolicy: certmanagerv1.RotationPolicyAlways,
			},
			SecretName: name,
			IssuerRef: cmmeta.ObjectReference{
				Group: certmanager.GroupName,
				Kind:  "Issuer",
				Name:  issuerName,
			},
		},
	}

	opts.SetName(&certificate.ObjectMeta, name)

	// Default cert duration to an hour, which is more inline with the needs of this tool than the cert-manager default (90 days)
	certDuration := opts.Duration
	if certDuration == nil {
		certDuration = ptr.To(time.Hour)
	}
	certificate.Spec.Duration = &v1.Duration{
		Duration: *certDuration,
	}

	if opts.IssuerKind != "" {
		certificate.Spec.IssuerRef.Kind = opts.IssuerKind
	}

	if opts.SecretName != "" {
		certificate.Spec.SecretName = opts.SecretName
	}

	if len(opts.SecretLabels) != 0 {
		certificate.Spec.SecretTemplate = &certmanagerv1.CertificateSecretTemplate{
			Labels: opts.SecretLabels,
		}
	}

	certificate, err := cmc.client.CertmanagerV1().Certificates(namespace).Create(ctx, certificate, v1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create certificate %q", helpers.FullNameStr(namespace, name))
	}

	return certificate, nil
}

type WaitForReadyCertificateOpts struct {
	helpers.MaxWaitTime
}

func (cmc *Client) WaitForReadyCertificate(ctx context.Context, namespace, name string, opts WaitForReadyCertificateOpts) (*certmanagerv1.Certificate, error) {
	precondition := func(ctx context.Context, certificate *certmanagerv1.Certificate) (*certmanagerv1.Certificate, bool, error) {
		isReady := false
		for _, condition := range certificate.Status.Conditions {
			if condition.Type != certmanagerv1.CertificateConditionReady {
				continue
			}

			isReady = condition.Status == cmmeta.ConditionTrue
			break
		}

		if isReady {
			return certificate, true, nil
		}

		return nil, false, nil
	}
	certificate, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(time.Minute), cmc.client.CertmanagerV1().Certificates(namespace), name, precondition)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for certificate %q to become ready", helpers.FullNameStr(namespace, name))
	}

	return certificate, nil
}

func (cmc *Client) DeleteCertificate(ctx context.Context, name, namespace string) error {
	err := cmc.client.CertmanagerV1().Certificates(namespace).Delete(ctx, name, v1.DeleteOptions{})

	// TODO delete related requests if needed
	// can't remember if these are handled by CM or not

	return trace.Wrap(err, "failed to delete certificate %q", helpers.FullNameStr(namespace, name))
}
