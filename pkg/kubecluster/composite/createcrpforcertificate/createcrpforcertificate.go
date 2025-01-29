package createcrpforcertificate

import (
	context "context"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"k8s.io/utils/ptr"
)

type CreateCRPForCertificateOpts struct {
	helpers.MaxWaitTime
}

// Create a Certificate Request Policy that matches the given certificate as closely as possible.
func (p *Provider) CreateCRPForCertificate(ctx context.Context, cert *certmanagerv1.Certificate, opts CreateCRPForCertificateOpts) (*policyv1alpha1.CertificateRequestPolicy, error) {
	spec := policyv1alpha1.CertificateRequestPolicySpec{
		Selector: policyv1alpha1.CertificateRequestPolicySelector{
			Namespace: &policyv1alpha1.CertificateRequestPolicySelectorNamespace{
				MatchNames: []string{cert.Namespace},
			},
			IssuerRef: &policyv1alpha1.CertificateRequestPolicySelectorIssuerRef{
				Group: &cert.Spec.IssuerRef.Group,
				Kind:  &cert.Spec.IssuerRef.Kind,
				Name:  &cert.Spec.IssuerRef.Name,
			},
		},
	}

	// Constraints
	constraintsSet := false
	constraints := &policyv1alpha1.CertificateRequestPolicyConstraints{}
	if cert.Spec.Duration != nil {
		constraints.MinDuration = cert.Spec.Duration
		constraints.MaxDuration = cert.Spec.Duration
		constraintsSet = true
	}

	if cert.Spec.PrivateKey != nil {
		// TODO remove this after https://github.com/cert-manager/approver-policy/pull/572 is merged
		// This is needed to work around an upstream bug
		if cert.Spec.PrivateKey.Algorithm != certmanagerv1.Ed25519KeyAlgorithm {
			privateKeySet := false
			privateKey := &policyv1alpha1.CertificateRequestPolicyConstraintsPrivateKey{}

			if cert.Spec.PrivateKey.Algorithm != "" {
				privateKey.Algorithm = &cert.Spec.PrivateKey.Algorithm
				privateKeySet = true
			}

			if cert.Spec.PrivateKey.Size != 0 {
				privateKey.MinSize = &cert.Spec.PrivateKey.Size
				privateKey.MaxSize = &cert.Spec.PrivateKey.Size
				privateKeySet = true
			}

			if privateKeySet {
				constraints.PrivateKey = privateKey
				constraintsSet = true
			}
		}
	}

	if constraintsSet {
		spec.Constraints = constraints
	}

	// Allowed
	allowedSet := false
	allowed := &policyv1alpha1.CertificateRequestPolicyAllowed{}
	if cert.Spec.CommonName != "" {
		allowed.CommonName = &policyv1alpha1.CertificateRequestPolicyAllowedString{
			Value:    &cert.Spec.CommonName,
			Required: ptr.To(true),
		}
		allowedSet = true
	}

	if len(cert.Spec.DNSNames) > 0 {
		allowed.DNSNames = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
			Values:   &cert.Spec.DNSNames,
			Required: ptr.To(true),
		}
		allowedSet = true
	}

	if len(cert.Spec.IPAddresses) > 0 {
		allowed.IPAddresses = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
			Values:   &cert.Spec.IPAddresses,
			Required: ptr.To(true),
		}
		allowedSet = true
	}

	if len(cert.Spec.URIs) > 0 {
		allowed.URIs = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
			Values:   &cert.Spec.URIs,
			Required: ptr.To(true),
		}
		allowedSet = true
	}

	if len(cert.Spec.EmailAddresses) > 0 {
		allowed.EmailAddresses = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
			Values:   &cert.Spec.EmailAddresses,
			Required: ptr.To(true),
		}
		allowedSet = true
	}

	if cert.Spec.IsCA {
		allowed.IsCA = ptr.To(true)
		allowedSet = true
	}

	if cert.Spec.Usages != nil {
		allowed.Usages = &cert.Spec.Usages
		allowedSet = true
	}

	if cert.Spec.Subject != nil {
		subjectSet := false
		subject := &policyv1alpha1.CertificateRequestPolicyAllowedX509Subject{}

		if len(cert.Spec.Subject.Organizations) > 0 {
			subject.Organizations = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.Organizations,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if len(cert.Spec.Subject.Countries) > 0 {
			subject.Countries = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.Countries,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if len(cert.Spec.Subject.OrganizationalUnits) > 0 {
			subject.OrganizationalUnits = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.OrganizationalUnits,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if len(cert.Spec.Subject.Localities) > 0 {
			subject.Localities = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.Localities,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if len(cert.Spec.Subject.Provinces) > 0 {
			subject.Provinces = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.Provinces,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if len(cert.Spec.Subject.StreetAddresses) > 0 {
			subject.StreetAddresses = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.StreetAddresses,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if len(cert.Spec.Subject.PostalCodes) > 0 {
			subject.PostalCodes = &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
				Values:   &cert.Spec.Subject.PostalCodes,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if cert.Spec.Subject.SerialNumber != "" {
			subject.SerialNumber = &policyv1alpha1.CertificateRequestPolicyAllowedString{
				Value:    &cert.Spec.Subject.SerialNumber,
				Required: ptr.To(true),
			}
			subjectSet = true
		}

		if subjectSet {
			allowed.Subject = subject
			allowedSet = true
		}
	}

	if allowedSet {
		spec.Allowed = allowed
	}

	crp, err := p.apClient.CreateCertificateRequestPolicy(ctx, cert.Name, spec, approverpolicy.CreateCertificateRequestPolicyOptions{GenerateName: true})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create certificate request policy for certificate %q", cert.Name)
	}

	readyCRP, err := p.apClient.WaitForReadyCertificateRequestPolicy(ctx, crp.Name, approverpolicy.WaitForReadyCertificateRequestPolicyOpts{MaxWaitTime: opts.MaxWaitTime})
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for certificate request policy %q to become ready", crp.Name)
	}

	return readyCRP, nil
}
