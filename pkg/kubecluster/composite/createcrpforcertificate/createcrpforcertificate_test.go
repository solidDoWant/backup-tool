package createcrpforcertificate

import (
	"testing"
	"time"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestCreateCRPForCertificate(t *testing.T) {
	certName := "test-cert"
	certNamespace := "test-namespace"

	basicCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: certNamespace,
			Name:      certName,
		},
		Spec: certmanagerv1.CertificateSpec{
			IssuerRef: cmmeta.ObjectReference{
				Group: "test-group",
				Kind:  "test-kind",
				Name:  "test-name",
			},
		},
	}

	tests := []struct {
		desc                                              string
		cert                                              *certmanagerv1.Certificate
		opts                                              CreateCRPForCertificateOpts
		simulateCreateCertificateRequestPolicyError       bool
		simulateWaitForReadyCertificateRequestPolicyError bool
		expectedCRP                                       *policyv1alpha1.CertificateRequestPolicy
	}{
		{
			desc: "minimal certificate",
			cert: basicCert,
			expectedCRP: &policyv1alpha1.CertificateRequestPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: certName,
				},
				Spec: policyv1alpha1.CertificateRequestPolicySpec{
					Selector: policyv1alpha1.CertificateRequestPolicySelector{
						Namespace: &policyv1alpha1.CertificateRequestPolicySelectorNamespace{
							MatchNames: []string{certNamespace},
						},
						IssuerRef: &policyv1alpha1.CertificateRequestPolicySelectorIssuerRef{
							Group: &basicCert.Spec.IssuerRef.Group,
							Kind:  &basicCert.Spec.IssuerRef.Kind,
							Name:  &basicCert.Spec.IssuerRef.Name,
						},
					},
				},
			},
		},
		{
			desc: "all opts",
			cert: &certmanagerv1.Certificate{
				ObjectMeta: basicCert.ObjectMeta,
				Spec: certmanagerv1.CertificateSpec{
					IssuerRef:  basicCert.Spec.IssuerRef,
					CommonName: "test.example.com",
					Duration: &metav1.Duration{
						Duration: 24 * time.Hour,
					},
					DNSNames:       []string{"test1.example.com", "test2.example.org"},
					IPAddresses:    []string{"1.2.3.4", "5.6.7.8"},
					EmailAddresses: []string{"test1@example.com", "test2@example.org"},
					URIs:           []string{"https://test.example.com"},
					IsCA:           true,
					Usages: []certmanagerv1.KeyUsage{
						certmanagerv1.UsageDigitalSignature,
						certmanagerv1.UsageKeyEncipherment,
					},
					PrivateKey: &certmanagerv1.CertificatePrivateKey{
						Algorithm: "RSA",
						Size:      2048,
					},
					Subject: &certmanagerv1.X509Subject{
						Organizations:       []string{"Test Org"},
						OrganizationalUnits: []string{"Test OU"},
						Countries:           []string{"Test Country"},
						Provinces:           []string{"Test Province"},
						Localities:          []string{"Test Locality"},
						StreetAddresses:     []string{"Test Street"},
						PostalCodes:         []string{"Test Postal Code"},
						SerialNumber:        "Test Serial Number",
					},
				},
			},
			opts: CreateCRPForCertificateOpts{
				MaxWaitTime: helpers.MaxWaitTime(5 * time.Minute),
			},
			expectedCRP: &policyv1alpha1.CertificateRequestPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: certName,
				},
				Spec: policyv1alpha1.CertificateRequestPolicySpec{
					Constraints: &policyv1alpha1.CertificateRequestPolicyConstraints{
						MinDuration: &metav1.Duration{
							Duration: 24 * time.Hour,
						},
						MaxDuration: &metav1.Duration{
							Duration: 24 * time.Hour,
						},
						PrivateKey: &policyv1alpha1.CertificateRequestPolicyConstraintsPrivateKey{
							Algorithm: ptr.To(certmanagerv1.PrivateKeyAlgorithm("RSA")),
							MinSize:   ptr.To(2048),
							MaxSize:   ptr.To(2048),
						},
					},
					Allowed: &policyv1alpha1.CertificateRequestPolicyAllowed{
						CommonName: &policyv1alpha1.CertificateRequestPolicyAllowedString{
							Value:    ptr.To("test.example.com"),
							Required: ptr.To(true),
						},
						DNSNames: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
							Values:   ptr.To([]string{"test1.example.com", "test2.example.org"}),
							Required: ptr.To(true),
						},
						IPAddresses: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
							Values:   ptr.To([]string{"1.2.3.4", "5.6.7.8"}),
							Required: ptr.To(true),
						},
						URIs: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
							Values:   ptr.To([]string{"https://test.example.com"}),
							Required: ptr.To(true),
						},
						EmailAddresses: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
							Values:   ptr.To([]string{"test1@example.com", "test2@example.org"}),
							Required: ptr.To(true),
						},
						IsCA: ptr.To(true),
						Usages: &[]certmanagerv1.KeyUsage{
							certmanagerv1.UsageDigitalSignature,
							certmanagerv1.UsageKeyEncipherment,
						},
						Subject: &policyv1alpha1.CertificateRequestPolicyAllowedX509Subject{
							Organizations: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test Org"}),
								Required: ptr.To(true),
							},
							OrganizationalUnits: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test OU"}),
								Required: ptr.To(true),
							},
							Countries: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test Country"}),
								Required: ptr.To(true),
							},
							Provinces: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test Province"}),
								Required: ptr.To(true),
							},
							Localities: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test Locality"}),
								Required: ptr.To(true),
							},
							StreetAddresses: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test Street"}),
								Required: ptr.To(true),
							},
							PostalCodes: &policyv1alpha1.CertificateRequestPolicyAllowedStringSlice{
								Values:   ptr.To([]string{"Test Postal Code"}),
								Required: ptr.To(true),
							},
							SerialNumber: &policyv1alpha1.CertificateRequestPolicyAllowedString{
								Value:    ptr.To("Test Serial Number"),
								Required: ptr.To(true),
							},
						},
					},
					Selector: policyv1alpha1.CertificateRequestPolicySelector{
						Namespace: &policyv1alpha1.CertificateRequestPolicySelectorNamespace{
							MatchNames: []string{certNamespace},
						},
						IssuerRef: &policyv1alpha1.CertificateRequestPolicySelectorIssuerRef{
							Group: &basicCert.Spec.IssuerRef.Group,
							Kind:  &basicCert.Spec.IssuerRef.Kind,
							Name:  &basicCert.Spec.IssuerRef.Name,
						},
					},
				},
			},
		},
		{
			desc: "error creating certificate request policy",
			cert: basicCert,
			simulateCreateCertificateRequestPolicyError: true,
		},
		{
			desc: "error waiting for ready certificate request policy",
			cert: basicCert,
			simulateWaitForReadyCertificateRequestPolicyError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			c := newMockProvider(t)

			func() {
				var crp *policyv1alpha1.CertificateRequestPolicy
				c.apClient.EXPECT().CreateCertificateRequestPolicy(ctx, tt.cert.Name, mock.Anything, approverpolicy.CreateCertificateRequestPolicyOptions{GenerateName: true}).
					RunAndReturn(func(ctx *contexts.Context, name string, spec policyv1alpha1.CertificateRequestPolicySpec, opts approverpolicy.CreateCertificateRequestPolicyOptions) (*policyv1alpha1.CertificateRequestPolicy, error) {
						crp = &policyv1alpha1.CertificateRequestPolicy{
							ObjectMeta: metav1.ObjectMeta{
								Name: name,
							},
							Spec: spec,
						}

						return th.ErrOr1Val(crp, tt.simulateCreateCertificateRequestPolicyError)
					})
				if tt.simulateCreateCertificateRequestPolicyError {
					return
				}

				c.apClient.EXPECT().WaitForReadyCertificateRequestPolicy(ctx, tt.cert.Name, approverpolicy.WaitForReadyCertificateRequestPolicyOpts{MaxWaitTime: tt.opts.MaxWaitTime}).
					RunAndReturn(func(ctx *contexts.Context, name string, wfrcrpo approverpolicy.WaitForReadyCertificateRequestPolicyOpts) (*policyv1alpha1.CertificateRequestPolicy, error) {
						return th.ErrOr1Val(crp, tt.simulateWaitForReadyCertificateRequestPolicyError)
					})
			}()

			createdCRP, err := c.CreateCRPForCertificate(ctx, tt.cert, tt.opts)
			if th.ErrExpected(tt.simulateCreateCertificateRequestPolicyError, tt.simulateWaitForReadyCertificateRequestPolicyError) {
				assert.Error(t, err)
				assert.Nil(t, createdCRP)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCRP, createdCRP)
		})
	}
}
