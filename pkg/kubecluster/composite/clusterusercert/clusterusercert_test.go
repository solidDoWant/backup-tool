package clusterusercert

import (
	context "context"
	"testing"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforprofile"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewClusterUserCertOptsCRP(t *testing.T) {
	th.OptStructTest[NewClusterUserCertOptsCRP](t)
}

func TestNewClusterUserCertOpts(t *testing.T) {
	th.OptStructTest[NewClusterUserCertOpts](t)
}

func TestNewClusterUserCert(t *testing.T) {
	namespace := "test-ns"
	username := "test-user"
	issuerName := "test-issuer"
	clusterName := "test-cluster"

	tests := []struct {
		desc                                 string
		opts                                 NewClusterUserCertOpts
		simulateClusterCleanupError          bool
		simulateCreateCertError              bool
		simulateCreateCRPForCertificateError bool
		simulateReissueCertificateError      bool
		simulateWaitForReadyCertError        bool
	}{
		{
			desc: "basic test",
		},
		{
			desc: "basic test with CRP enabled",
			opts: NewClusterUserCertOpts{
				CRPOpts: NewClusterUserCertOptsCRP{
					Enabled:           true,
					WaitForCRPTimeout: helpers.ShortWaitTime,
				},
				IssuerKind: "test-kind",
				Subject: &certmanagerv1.X509Subject{
					Organizations:       []string{"test-org"},
					OrganizationalUnits: []string{"test-ou"},
					Countries:           []string{"test-country"},
					Provinces:           []string{"test-province"},
					Localities:          []string{"test-locality"},
					StreetAddresses:     []string{"test-street"},
					PostalCodes:         []string{"test-postal"},
					SerialNumber:        "test-serial",
				},
				WaitForCertTimeout: helpers.ShortWaitTime,
				CleanupTimeout:     helpers.ShortWaitTime,
			},
		},
		{
			desc:                    "simulate create cert error",
			simulateCreateCertError: true,
		},
		{
			desc:                        "simulate cluster cleanup error",
			simulateClusterCleanupError: true,
			simulateCreateCertError:     true,
		},
		{
			desc:                                 "simulate create CRP for certificate error",
			opts:                                 NewClusterUserCertOpts{CRPOpts: NewClusterUserCertOptsCRP{Enabled: true}},
			simulateCreateCRPForCertificateError: true,
		},
		{
			desc:                            "simulate reissue certificate error",
			opts:                            NewClusterUserCertOpts{CRPOpts: NewClusterUserCertOptsCRP{Enabled: true}},
			simulateReissueCertificateError: true,
		},
		{
			desc:                          "simulate wait for ready cert error",
			simulateWaitForReadyCertError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.Background()
			c := newMockProvider(t)

			createdCert := &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				},
			}

			createdCRP := &policyv1alpha1.CertificateRequestPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-crp",
				},
			}

			errExpected := th.ErrExpected(
				tt.simulateClusterCleanupError,
				tt.simulateCreateCertError,
				tt.simulateCreateCRPForCertificateError,
				tt.simulateReissueCertificateError,
				tt.simulateWaitForReadyCertError,
			)

			func() {
				if errExpected {
					c.clusterUserCert.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx context.Context) error {
						require.NotEqual(t, ctx, cleanupCtx) // This should be a different context with a timeout
						return th.ErrIfTrue(tt.simulateClusterCleanupError)
					})
				}

				// 1.
				c.cmClient.EXPECT().CreateCertificate(ctx, namespace, mock.Anything, issuerName, mock.Anything).
					RunAndReturn(func(ctx context.Context, namespace, certName, issuerName string, opts certmanager.CreateCertificateOptions) (*certmanagerv1.Certificate, error) {
						assert.Contains(t, certName, clusterName)
						assert.Contains(t, certName, username)
						assert.Equal(t, opts.CommonName, username)
						assert.Equal(t, []certmanagerv1.KeyUsage{certmanagerv1.UsageClientAuth}, opts.Usages)
						assert.Equal(t, opts.IssuerKind, tt.opts.IssuerKind)

						createdCert.ObjectMeta.Name = certName

						return th.ErrOr1Val(createdCert, tt.simulateCreateCertError)
					})
				if tt.simulateCreateCertError {
					return
				}
				c.clusterUserCert.EXPECT().setCertificate(createdCert)

				// 2.
				if tt.opts.CRPOpts.Enabled {
					c.ccfp.EXPECT().CreateCRPForCertificate(ctx, createdCert, createcrpforprofile.CreateCRPForCertificateOpts{MaxWaitTime: tt.opts.CRPOpts.WaitForCRPTimeout}).
						Return(th.ErrOr1Val(createdCRP, tt.simulateCreateCRPForCertificateError))
					if tt.simulateCreateCRPForCertificateError {
						return
					}
					c.clusterUserCert.EXPECT().setCRP(createdCRP)

					// 2.1.
					c.cmClient.EXPECT().ReissueCertificate(ctx, createdCert.Namespace, mock.Anything).Return(th.ErrOr1Val(createdCert, tt.simulateReissueCertificateError))
					if tt.simulateReissueCertificateError {
						return
					}
					c.clusterUserCert.EXPECT().setCertificate(createdCert)
				}

				// 3.
				c.cmClient.EXPECT().WaitForReadyCertificate(ctx, createdCert.Namespace, mock.Anything, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: tt.opts.WaitForCertTimeout}).Return(th.ErrOr1Val(createdCert, tt.simulateWaitForReadyCertError))
				if tt.simulateWaitForReadyCertError {
					return
				}
				c.clusterUserCert.EXPECT().setCertificate(createdCert)
			}()

			clusterUserCert, err := c.NewClusterUserCert(ctx, namespace, username, issuerName, clusterName, tt.opts)

			if errExpected {
				assert.Error(t, err)
				assert.Nil(t, clusterUserCert)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, clusterUserCert)
		})
	}
}

func TestSetCertificate(t *testing.T) {
	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: "test-namespace",
		},
	}

	cuc := &ClusterUserCert{}
	cuc.setCertificate(cert)

	assert.Equal(t, cert, cuc.certificate)
}

func TestGetCertificate(t *testing.T) {
	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: "test-namespace",
		},
	}

	cuc := &ClusterUserCert{}
	cuc.certificate = cert

	assert.Equal(t, cert, cuc.GetCertificate())
}

func TestSetCRP(t *testing.T) {
	crp := &policyv1alpha1.CertificateRequestPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-crp",
		},
	}

	cuc := &ClusterUserCert{}
	cuc.setCRP(crp)

	assert.Equal(t, crp, cuc.crp)
}

func TestGetCertificateRequestPolicy(t *testing.T) {
	crp := &policyv1alpha1.CertificateRequestPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-crp",
		},
	}

	cuc := &ClusterUserCert{}
	cuc.crp = crp

	assert.Equal(t, crp, cuc.GetCertificateRequestPolicy())
}

func TestClusterUserCertDelete(t *testing.T) {
	certificate := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: "test-namespace",
		},
	}

	crp := &policyv1alpha1.CertificateRequestPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-crp",
		},
	}

	allResourcesCluster := ClusterUserCert{
		certificate: certificate,
		crp:         crp,
	}

	tests := []struct {
		desc                           string
		cuc                            ClusterUserCert
		simulateCertificateDeleteError bool
		simulateCRPDeleteError         bool
		expectedErrorsInMessage        int
	}{
		{
			desc: "delete all resources",
			cuc:  allResourcesCluster,
		},
		{
			desc: "delete just certificate",
			cuc: ClusterUserCert{
				certificate: certificate,
			},
		},
		{
			desc: "delete just CRP",
			cuc: ClusterUserCert{
				crp: crp,
			},
		},
		{
			desc: "delete nothing",
		},
		{
			desc:                           "all deletions fail",
			cuc:                            allResourcesCluster,
			simulateCertificateDeleteError: true,
			simulateCRPDeleteError:         true,
			expectedErrorsInMessage:        2,
		},
		{
			desc:                           "certificate deletions fail",
			cuc:                            allResourcesCluster,
			simulateCertificateDeleteError: true,
			expectedErrorsInMessage:        1,
		},
		{
			desc:                    "crp deletion fails",
			cuc:                     allResourcesCluster,
			simulateCRPDeleteError:  true,
			expectedErrorsInMessage: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.Background()
			p := newMockProvider(t)
			tt.cuc.p = p

			if tt.cuc.certificate != nil {
				p.cmClient.EXPECT().DeleteCertificate(ctx, tt.cuc.certificate.Namespace, tt.cuc.certificate.Name).Return(th.ErrIfTrue(tt.simulateCertificateDeleteError))
			}

			if tt.cuc.crp != nil {
				p.apClient.EXPECT().DeleteCertificateRequestPolicy(ctx, tt.cuc.crp.Name).Return(th.ErrIfTrue(tt.simulateCRPDeleteError))
			}

			err := tt.cuc.Delete(ctx)
			if tt.expectedErrorsInMessage == 0 {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			if tErr, ok := err.(trace.Error); ok {
				if oErrs, ok := tErr.OrigError().(trace.Aggregate); ok {
					assert.Equal(t, tt.expectedErrorsInMessage, len(oErrs.Errors()))
				}
			} else {
				require.Fail(t, "error is not a trace.Error")
			}
		})
	}
}
