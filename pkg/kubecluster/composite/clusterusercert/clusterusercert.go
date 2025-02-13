package clusterusercert

import (
	"fmt"
	"time"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforcertificate"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
)

type ClusterUserCertInterface interface {
	setCertificate(*certmanagerv1.Certificate)
	GetCertificate() *certmanagerv1.Certificate
	setCRP(*policyv1alpha1.CertificateRequestPolicy)
	GetCertificateRequestPolicy() *policyv1alpha1.CertificateRequestPolicy
	Delete(ctx *contexts.Context) error
}

type ClusterUserCert struct {
	p           providerInterfaceInternal
	certificate *certmanagerv1.Certificate
	crp         *policyv1alpha1.CertificateRequestPolicy
}

type NewClusterUserCertOptsCRP struct {
	Enabled           bool                `yaml:"enabled,omitempty"`
	WaitForCRPTimeout helpers.MaxWaitTime `yaml:"waitForCRPTimeout,omitempty"`
}

type NewClusterUserCertOpts struct {
	IssuerKind         string                     `yaml:"issuerKind,omitempty"`
	Subject            *certmanagerv1.X509Subject `yaml:"subject,omitempty"`
	CRPOpts            NewClusterUserCertOptsCRP  `yaml:"certificateRequestPolicy,omitempty"`
	WaitForCertTimeout helpers.MaxWaitTime        `yaml:"waitForCertTimeout,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime        `yaml:"cleanupTimeout,omitempty"`
}

func newClusterUserCert(p providerInterfaceInternal) ClusterUserCertInterface {
	return &ClusterUserCert{p: p}
}

func (p *Provider) NewClusterUserCert(ctx *contexts.Context, namespace, username, issuerName, clusterName string, opts NewClusterUserCertOpts) (cuc ClusterUserCertInterface, err error) {
	ctx.Log.With("username", username, "issuerName", issuerName).Info("Creating user certificate")
	defer ctx.Log.Info("Finished creating user certificate", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	cuc = p.newClusterUserCert()

	errHandler := func(originalErr error, args ...interface{}) (*ClusterUserCert, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.To(cuc.Delete).
			WithErrMessage("failed to cleanup user cert %q in namespace %q", username, namespace).
			WithOriginalErr(&originalErr).
			WithParentCtx(ctx).
			WithTimeout(opts.CleanupTimeout.MaxWait(10 * time.Minute)).
			Run()
	}

	// 1. Create the certificate itself
	ctx.Log.Step().Info("Creating user certificate")
	certName := helpers.CleanName(fmt.Sprintf("%s-%s-user", clusterName, username))
	certOptions := certmanager.CreateCertificateOptions{
		CommonName: username,
		Subject:    opts.Subject,
		Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageClientAuth},
		SecretLabels: map[string]string{
			utils.WatchedLabelName: "true",
		},
		IssuerKind: opts.IssuerKind,
	}

	cert, err := p.cmClient.CreateCertificate(ctx.Child(), namespace, certName, issuerName, certOptions)
	if err != nil {
		return errHandler(err, "failed to create %q user cert %q", username, helpers.FullNameStr(namespace, certName))
	}
	cuc.setCertificate(cert)

	// 2. Create the CertificateRequestPolicy, if enabled
	if opts.CRPOpts.Enabled {
		ctx.Log.Step().Info("Creating CertificateRequestPolicy for user certificate")
		crpName := certName
		crp, err := p.ccfp.CreateCRPForCertificate(ctx.Child(), cert, createcrpforcertificate.CreateCRPForCertificateOpts{MaxWaitTime: opts.CRPOpts.WaitForCRPTimeout})
		if err != nil {
			return errHandler(err, "failed to create CertificateRequestPolicy %q for user cert %q", crpName, helpers.FullName(cert))
		}
		cuc.setCRP(crp)

		// 2.1. Re-issue the certificate, as it more than likely failed the first time
		reissuedCert, err := p.cmClient.ReissueCertificate(ctx.Child(), cert.Namespace, cert.Name)
		if err != nil {
			return errHandler(err, "failed to re-issue user cert %q", helpers.FullName(cert))
		}
		cert = reissuedCert
		cuc.setCertificate(cert)
	}

	// 3. Wait for the certificate to be ready
	ctx.Log.Step().Info("Waiting for user certificate to be ready")
	readyCert, err := p.cmClient.WaitForReadyCertificate(ctx.Child(), cert.Namespace, cert.Name, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: opts.WaitForCertTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for user cert %q to be ready", helpers.FullName(cert))
	}
	cuc.setCertificate(readyCert)

	return cuc, nil
}

func (cuc *ClusterUserCert) setCertificate(cert *certmanagerv1.Certificate) {
	cuc.certificate = cert
}

func (cuc *ClusterUserCert) GetCertificate() *certmanagerv1.Certificate {
	return cuc.certificate
}

func (cuc *ClusterUserCert) setCRP(crp *policyv1alpha1.CertificateRequestPolicy) {
	cuc.crp = crp
}

func (cuc *ClusterUserCert) GetCertificateRequestPolicy() *policyv1alpha1.CertificateRequestPolicy {
	return cuc.crp
}

func (cuc *ClusterUserCert) Delete(ctx *contexts.Context) (err error) {
	ctx.Log.Info("Cleaning up cluster user certificate resources")
	defer ctx.Log.Info("Finished cleaning up cluster user certificate resources", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	cleanupErrs := make([]error, 0, 2)

	if cuc.crp != nil {
		err := cuc.p.ap().DeleteCertificateRequestPolicy(ctx.Child(), cuc.crp.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete certificate request policy %q", helpers.FullName(cuc.crp)))
		}
	}

	if cuc.certificate != nil {
		err := cuc.p.cm().DeleteCertificate(ctx.Child(), cuc.certificate.Namespace, cuc.certificate.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete certificate %q", helpers.FullName(cuc.certificate)))
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed while cleaning up cluster user certificate resources")
}
