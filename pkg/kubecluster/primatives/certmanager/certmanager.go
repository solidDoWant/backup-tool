package certmanager

import (
	"context"
	"fmt"
	"time"

	cmutil "github.com/cert-manager/cert-manager/pkg/api/util"
	"github.com/cert-manager/cert-manager/pkg/apis/certmanager"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
)

type CreateCertificateOptions struct {
	helpers.GenerateName
	IsCA          bool
	CAConstraints *certmanagerv1.NameConstraints
	CommonName    string
	DNSNames      []string
	Duration      *time.Duration
	IssuerKind    string // Default to "Issuer" (namespace-specific)
	SecretLabels  map[string]string
	SecretName    string
	Subject       *certmanagerv1.X509Subject
	Usages        []certmanagerv1.KeyUsage
}

func (cmc *Client) CreateCertificate(ctx context.Context, namespace, name, issuerName string, opts CreateCertificateOptions) (*certmanagerv1.Certificate, error) {
	certificate := &certmanagerv1.Certificate{
		Spec: certmanagerv1.CertificateSpec{
			IsCA:                  opts.IsCA,
			NameConstraints:       opts.CAConstraints,
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
	certificate.Spec.Duration = &metav1.Duration{
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

	certificate, err := cmc.client.CertmanagerV1().Certificates(namespace).Create(ctx, certificate, metav1.CreateOptions{})
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

// Trigger an immediate re-issuance of a certificate
func (cmc *Client) ReissueCertificate(ctx context.Context, namespace, name string) (*certmanagerv1.Certificate, error) {
	cert, err := cmc.client.CertmanagerV1().Certificates(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to get certificate %q", helpers.FullNameStr(namespace, name))
	}

	cmutil.SetCertificateCondition(cert, cert.Generation, certmanagerv1.CertificateConditionIssuing, cmmeta.ConditionTrue, "ManuallyTriggered", fmt.Sprintf("Certificate re-issuance triggered by %s", constants.ToolName))
	var updatedCert *certmanagerv1.Certificate
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var err error
		updatedCert, err = cmc.client.CertmanagerV1().Certificates(cert.Namespace).UpdateStatus(ctx, cert, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return nil, trace.Wrap(err, "failed to update status of Certificate %s/%s", cert.Namespace, cert.Name)
	}

	return updatedCert, nil
}

func (cmc *Client) DeleteCertificate(ctx context.Context, namespace, name string) error {
	err := cmc.client.CertmanagerV1().Certificates(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete certificate %q", helpers.FullNameStr(namespace, name))
}

type CreateIssuerOptions struct {
	helpers.GenerateName
}

func (cmc *Client) CreateIssuer(ctx context.Context, namespace, name, caCertSecretName string, opts CreateIssuerOptions) (*certmanagerv1.Issuer, error) {
	issuer := &certmanagerv1.Issuer{
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				CA: &certmanagerv1.CAIssuer{
					SecretName: caCertSecretName,
				},
			},
		},
	}

	opts.SetName(&issuer.ObjectMeta, name)

	issuer, err := cmc.client.CertmanagerV1().Issuers(namespace).Create(ctx, issuer, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create issuer %q", helpers.FullNameStr(namespace, name))
	}

	return issuer, nil
}

type WaitForReadyIssuerOpts struct {
	helpers.MaxWaitTime
}

func (cmc *Client) WaitForReadyIssuer(ctx context.Context, namespace, name string, opts WaitForReadyIssuerOpts) (*certmanagerv1.Issuer, error) {
	precondition := func(ctx context.Context, issuer *certmanagerv1.Issuer) (*certmanagerv1.Issuer, bool, error) {
		isReady := false
		for _, condition := range issuer.Status.Conditions {
			if condition.Type != certmanagerv1.IssuerConditionReady {
				continue
			}

			isReady = condition.Status == cmmeta.ConditionTrue
			break
		}

		if isReady {
			return issuer, true, nil
		}

		return nil, false, nil
	}
	issuer, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(time.Minute), cmc.client.CertmanagerV1().Issuers(namespace), name, precondition)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for issuer %q to become ready", helpers.FullNameStr(namespace, name))
	}

	return issuer, nil
}

func (cmc *Client) DeleteIssuer(ctx context.Context, name, namespace string) error {
	err := cmc.client.CertmanagerV1().Issuers(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete issuer %q", helpers.FullNameStr(namespace, name))
}
