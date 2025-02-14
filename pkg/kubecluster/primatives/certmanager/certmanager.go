package certmanager

import (
	"fmt"
	"time"

	cmutil "github.com/cert-manager/cert-manager/pkg/api/util"
	"github.com/cert-manager/cert-manager/pkg/apis/certmanager"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
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
	KeyAlgorithm  certmanagerv1.PrivateKeyAlgorithm
}

func (cmc *Client) CreateCertificate(ctx *contexts.Context, namespace, name, issuerName string, opts CreateCertificateOptions) (*certmanagerv1.Certificate, error) {
	ctx.Log.With("name", name).Info("Creating certificate")
	ctx.Log.Debug("Call parameters", "issuerName", issuerName, "opts", opts)

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

	if opts.KeyAlgorithm != "" {
		certificate.Spec.PrivateKey.Algorithm = opts.KeyAlgorithm
	}

	certificate, err := cmc.client.CertmanagerV1().Certificates(namespace).Create(ctx.Child(), certificate, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create certificate %q", helpers.FullNameStr(namespace, name))
	}

	return certificate, nil
}

func IsCertificateReady(certificate *certmanagerv1.Certificate) bool {
	for _, condition := range certificate.Status.Conditions {
		if condition.Type != certmanagerv1.CertificateConditionReady {
			continue
		}

		return condition.Status == cmmeta.ConditionTrue
	}

	return false
}

type WaitForReadyCertificateOpts struct {
	helpers.MaxWaitTime
}

func (cmc *Client) WaitForReadyCertificate(ctx *contexts.Context, namespace, name string, opts WaitForReadyCertificateOpts) (certificate *certmanagerv1.Certificate, err error) {
	ctx.Log.With("name", name).Info("Waiting for certificate to become ready")
	defer ctx.Log.Info("Finished waiting for certificate to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	precondition := func(ctx *contexts.Context, certificate *certmanagerv1.Certificate) (*certmanagerv1.Certificate, bool, error) {
		ctx.Log.Debug("Certificate conditions", "conditions", certificate.Status.Conditions)
		if IsCertificateReady(certificate) {
			return certificate, true, nil
		}
		return nil, false, nil
	}
	certificate, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(time.Minute), cmc.client.CertmanagerV1().Certificates(namespace), name, precondition)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for certificate %q to become ready", helpers.FullNameStr(namespace, name))
	}

	return certificate, nil
}

// Trigger an immediate re-issuance of a certificate
func (cmc *Client) ReissueCertificate(ctx *contexts.Context, namespace, name string) (certificate *certmanagerv1.Certificate, err error) {
	ctx.Log.With("name", name).Info("Reissuing certificate")
	defer ctx.Log.Info("Finished reissuing certificate", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	retryCtx := ctx.Child() // Do this once instead of on every retry
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cert, err := cmc.client.CertmanagerV1().Certificates(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return trace.Wrap(err, "failed to get certificate %q", helpers.FullNameStr(namespace, name))
		}

		cmutil.SetCertificateCondition(cert, cert.Generation, certmanagerv1.CertificateConditionIssuing, cmmeta.ConditionTrue, "ManuallyTriggered", fmt.Sprintf("Certificate re-issuance triggered by %s", constants.ToolName))
		certificate, err = cmc.client.CertmanagerV1().Certificates(cert.Namespace).UpdateStatus(retryCtx, cert, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return nil, trace.Wrap(err, "failed to update status of Certificate %q", helpers.FullNameStr(namespace, name))
	}

	return certificate, nil
}

func (cmc *Client) GetCertificate(ctx *contexts.Context, namespace, name string) (*certmanagerv1.Certificate, error) {
	ctx.Log.With("name", name).Info("Getting certificate")

	certificate, err := cmc.client.CertmanagerV1().Certificates(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to get certificate %q", helpers.FullNameStr(namespace, name))
	}

	ctx.Log.Debug("Retrieved certificate", "certificate", certificate)
	return certificate, nil
}

func (cmc *Client) DeleteCertificate(ctx *contexts.Context, namespace, name string) error {
	ctx.Log.With("name", name).Info("Deleting certificate")

	err := cmc.client.CertmanagerV1().Certificates(namespace).Delete(ctx.Child(), name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete certificate %q", helpers.FullNameStr(namespace, name))
}

type CreateIssuerOptions struct {
	helpers.GenerateName
}

func (cmc *Client) CreateIssuer(ctx *contexts.Context, namespace, name, caCertSecretName string, opts CreateIssuerOptions) (*certmanagerv1.Issuer, error) {
	ctx.Log.With("name", name).Info("Creating issuer")
	ctx.Log.Debug("Call parameters", "caCertSecretName", caCertSecretName, "opts", opts)

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

	issuer, err := cmc.client.CertmanagerV1().Issuers(namespace).Create(ctx.Child(), issuer, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create issuer %q", helpers.FullNameStr(namespace, name))
	}

	return issuer, nil
}

func IsIssuerReady(issuer *certmanagerv1.Issuer) bool {
	for _, condition := range issuer.Status.Conditions {
		if condition.Type != certmanagerv1.IssuerConditionReady {
			continue
		}

		return condition.Status == cmmeta.ConditionTrue
	}

	return false
}

type WaitForReadyIssuerOpts struct {
	helpers.MaxWaitTime
}

func (cmc *Client) WaitForReadyIssuer(ctx *contexts.Context, namespace, name string, opts WaitForReadyIssuerOpts) (issuer *certmanagerv1.Issuer, err error) {
	ctx.Log.With("name", name).Info("Waiting for issuer to become ready")
	defer ctx.Log.Info("Finished waiting for issuer to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	precondition := func(ctx *contexts.Context, issuer *certmanagerv1.Issuer) (*certmanagerv1.Issuer, bool, error) {
		ctx.Log.Debug("Issuer conditions", "conditions", issuer.Status.Conditions)
		if IsIssuerReady(issuer) {
			return issuer, true, nil
		}
		return nil, false, nil
	}
	issuer, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(time.Minute), cmc.client.CertmanagerV1().Issuers(namespace), name, precondition)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for issuer %q to become ready", helpers.FullNameStr(namespace, name))
	}

	return issuer, nil
}

func (cmc *Client) GetIssuer(ctx *contexts.Context, namespace, name string) (*certmanagerv1.Issuer, error) {
	ctx.Log.With("name", name).Info("Getting issuer")

	issuer, err := cmc.client.CertmanagerV1().Issuers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to get issuer %q", helpers.FullNameStr(namespace, name))
	}

	ctx.Log.Debug("Retrieved issuer", "issuer", issuer)
	return issuer, nil
}

func (cmc *Client) DeleteIssuer(ctx *contexts.Context, namespace, name string) error {
	ctx.Log.With("name", name).Info("Deleting issuer")

	err := cmc.client.CertmanagerV1().Issuers(namespace).Delete(ctx.Child(), name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete issuer %q", helpers.FullNameStr(namespace, name))
}
