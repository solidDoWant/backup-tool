package certmanager

import (
	"context"
	"sync"
	"testing"
	"time"

	"dario.cat/mergo"
	"github.com/cert-manager/cert-manager/pkg/apis/certmanager"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/cert-manager/cert-manager/pkg/client/clientset/versioned"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestCreateCertificate(t *testing.T) {
	namespace := "test-ns"
	certName := "test-cert"
	issuer := "test-issuer"

	standardCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			Duration:              &metav1.Duration{Duration: time.Hour},
			EncodeUsagesInRequest: ptr.To(true),
			IssuerRef: cmmeta.ObjectReference{
				Group: certmanager.GroupName,
				Kind:  "Issuer",
				Name:  issuer,
			},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm:      certmanagerv1.Ed25519KeyAlgorithm,
				Encoding:       certmanagerv1.PKCS8,
				RotationPolicy: certmanagerv1.RotationPolicyAlways,
			},
			SecretName: certName,
		},
	}

	tests := []struct {
		desc                  string
		opts                  CreateCertificateOptions
		expected              *certmanagerv1.Certificate
		simulateClientFailure bool
	}{
		{
			desc: "no options",
			expected: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: certName,
				},
			},
		},
		{
			desc: "all options",
			opts: CreateCertificateOptions{
				GenerateName: true,
				CommonName:   "test.example.com",
				DNSNames:     []string{"test1.example.com", "test2.example.com"},
				Duration:     ptr.To(24 * time.Hour),
				IssuerKind:   "ClusterIssuer",
				SecretName:   "secret-name-override",
				Subject: &certmanagerv1.X509Subject{
					Organizations:       []string{"Test Org"},
					Countries:           []string{"Test Country"},
					OrganizationalUnits: []string{"Test OU"},
					Localities:          []string{"Test Locality"},
					Provinces:           []string{"Test Province"},
					StreetAddresses:     []string{"Test Street"},
					PostalCodes:         []string{"12345"},
					SerialNumber:        "12345",
				},
				SecretLabels: map[string]string{"env": "test"},
				Usages: []certmanagerv1.KeyUsage{
					certmanagerv1.UsageEmailProtection,
					certmanagerv1.UsageNetscapeSGC,
				},
			},
			expected: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: certName + "-",
				},
				Spec: certmanagerv1.CertificateSpec{
					CommonName: "test.example.com",
					DNSNames:   []string{"test1.example.com", "test2.example.com"},
					Duration:   &metav1.Duration{Duration: 24 * time.Hour},
					IssuerRef: cmmeta.ObjectReference{
						Kind: "ClusterIssuer",
					},
					SecretTemplate: &certmanagerv1.CertificateSecretTemplate{
						Labels: map[string]string{"env": "test"},
					},
					SecretName: "secret-name-override",
					Subject: &certmanagerv1.X509Subject{
						Organizations:       []string{"Test Org"},
						Countries:           []string{"Test Country"},
						OrganizationalUnits: []string{"Test OU"},
						Localities:          []string{"Test Locality"},
						Provinces:           []string{"Test Province"},
						StreetAddresses:     []string{"Test Street"},
						PostalCodes:         []string{"12345"},
						SerialNumber:        "12345",
					},
					Usages: []certmanagerv1.KeyUsage{
						certmanagerv1.UsageEmailProtection,
						certmanagerv1.UsageNetscapeSGC,
					},
				},
			},
		},
		{
			desc:                  "simulate client failure",
			simulateClientFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, fakeClientset := createTestClient()
			if tt.simulateClientFailure {
				fakeClientset.PrependReactor("create", "certificates", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			ctx := context.Background()
			cert, err := client.CreateCertificate(ctx, certName, namespace, issuer, tt.opts)

			if tt.simulateClientFailure {
				assert.Error(t, err)
				assert.Nil(t, cert)
				return
			}

			expectedCert := standardCert.DeepCopy()
			require.NoError(t, mergo.MergeWithOverwrite(expectedCert, tt.expected))
			assert.NoError(t, err)
			assert.Equal(t, expectedCert, cert)

			retrievedCert, err := client.client.CertmanagerV1().Certificates(namespace).Get(ctx, cert.Name, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Equal(t, expectedCert, retrievedCert)
		})
	}
}

func TestWaitForReadyCertificate(t *testing.T) {
	certName := "test-cert"
	certNamespace := "test-ns"

	noStatusCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: certNamespace,
			Name:      certName,
		},
	}

	notReadyCert := noStatusCert.DeepCopy()
	notReadyCondition := certmanagerv1.CertificateCondition{Type: certmanagerv1.CertificateConditionReady, Status: cmmeta.ConditionFalse}
	notReadyCert.Status.Conditions = append(notReadyCert.Status.Conditions, notReadyCondition)

	readyCert := notReadyCert.DeepCopy()
	readyCondition := notReadyCondition.DeepCopy()
	readyCondition.Status = cmmeta.ConditionTrue
	readyCert.Status.Conditions[0] = *readyCondition

	multipleConditionsCert := readyCert.DeepCopy()
	issuingCondition := certmanagerv1.CertificateCondition{Type: certmanagerv1.CertificateConditionIssuing, Status: cmmeta.ConditionFalse}
	multipleConditionsCert.Status.Conditions = []certmanagerv1.CertificateCondition{issuingCondition, *readyCondition}

	tests := []struct {
		desc                string
		initialCert         *certmanagerv1.Certificate
		shouldError         bool
		afterStartedWaiting func(*testing.T, context.Context, versioned.Interface)
	}{
		{
			desc:        "certificate starts ready",
			initialCert: readyCert,
		},
		{
			desc:        "certificate not ready",
			initialCert: notReadyCert,
			shouldError: true,
		},
		{
			desc:        "certificate has no status",
			initialCert: noStatusCert,
			shouldError: true,
		},
		{
			desc:        "certificate does not exist",
			shouldError: true,
		},
		{
			desc:        "certificate becomes ready",
			initialCert: notReadyCert,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.CertmanagerV1().Certificates(certNamespace).Update(ctx, readyCert, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:        "multiple conditions",
			initialCert: notReadyCert,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.CertmanagerV1().Certificates(certNamespace).Update(ctx, multipleConditionsCert, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			client, fakeClientset := createTestClient()
			ctx := context.Background()

			if tt.initialCert != nil {
				_, err := fakeClientset.CertmanagerV1().Certificates(certNamespace).Create(ctx, tt.initialCert, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			wg.Add(1)
			go func() {
				waitErr = client.WaitForReadyCertificate(ctx, certNamespace, certName, WaitForReadyCertificateOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, fakeClientset)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				return
			}
			assert.NoError(t, waitErr)
		})
	}
}

func TestDeleteCertificate(t *testing.T) {
	namespace := "test-ns"
	certName := "test-cert"

	tests := []struct {
		desc            string
		shouldSetupCert bool
		wantErr         bool
	}{
		{
			desc:            "delete existing certificate",
			shouldSetupCert: true,
		},
		{
			desc:    "delete non-existent certificate",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, _ := createTestClient()
			ctx := context.Background()

			var existingCert *certmanagerv1.Certificate
			if tt.shouldSetupCert {
				existingCert = &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      certName,
						Namespace: namespace,
					},
				}
				_, err := client.client.CertmanagerV1().Certificates(namespace).Create(ctx, existingCert, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteCertificate(ctx, certName, namespace)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the certificate was deleted
			certList, err := client.client.CertmanagerV1().Certificates(namespace).List(ctx, metav1.SingleObject(existingCert.ObjectMeta))
			assert.NoError(t, err)
			assert.Empty(t, certList.Items)
		})
	}
}
