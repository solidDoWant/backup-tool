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
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
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
				KeyAlgorithm: certmanagerv1.RSAKeyAlgorithm,
			},
			expected: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: certName,
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
					PrivateKey: &certmanagerv1.CertificatePrivateKey{
						Algorithm: certmanagerv1.RSAKeyAlgorithm,
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

			ctx := th.NewTestContext()
			cert, err := client.CreateCertificate(ctx, namespace, certName, issuer, tt.opts)

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
			client, fakeClientset := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialCert != nil {
				_, err := fakeClientset.CertmanagerV1().Certificates(certNamespace).Create(ctx, tt.initialCert, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var cert *certmanagerv1.Certificate
			wg.Add(1)
			go func() {
				cert, waitErr = client.WaitForReadyCertificate(ctx, certNamespace, certName, WaitForReadyCertificateOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, fakeClientset)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, cert)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, cert)
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
			ctx := th.NewTestContext()

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

			err := client.DeleteCertificate(ctx, namespace, certName)
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

func TestCreateIssuer(t *testing.T) {
	namespace := "test-ns"
	issuerName := "test-issuer"
	caCertSecretName := "test-ca-secret"

	standardIssuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				CA: &certmanagerv1.CAIssuer{
					SecretName: caCertSecretName,
				},
			},
		},
	}

	tests := []struct {
		desc                  string
		opts                  CreateIssuerOptions
		expected              *certmanagerv1.Issuer
		simulateClientFailure bool
	}{
		{
			desc: "no options",
			expected: &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name: issuerName,
				},
			},
		},
		{
			desc: "with generate name",
			opts: CreateIssuerOptions{
				GenerateName: true,
			},
			expected: &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: issuerName,
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
				fakeClientset.PrependReactor("create", "issuers", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			ctx := th.NewTestContext()
			issuer, err := client.CreateIssuer(ctx, namespace, issuerName, caCertSecretName, tt.opts)

			if tt.simulateClientFailure {
				assert.Error(t, err)
				assert.Nil(t, issuer)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, issuer)

			expectedIssuer := standardIssuer.DeepCopy()
			require.NoError(t, mergo.MergeWithOverwrite(expectedIssuer, tt.expected))
			assert.Equal(t, expectedIssuer, issuer)

			retrievedIssuer, err := client.client.CertmanagerV1().Issuers(namespace).Get(ctx, issuer.Name, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.Equal(t, expectedIssuer, retrievedIssuer)
		})
	}
}

func TestWaitForReadyIssuer(t *testing.T) {
	issuerName := "test-issuer"
	issuerNamespace := "test-ns"

	noStatusIssuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: issuerNamespace,
			Name:      issuerName,
		},
	}

	notReadyIssuer := noStatusIssuer.DeepCopy()
	notReadyCondition := certmanagerv1.IssuerCondition{Type: certmanagerv1.IssuerConditionReady, Status: cmmeta.ConditionFalse}
	notReadyIssuer.Status.Conditions = append(notReadyIssuer.Status.Conditions, notReadyCondition)

	readyIssuer := notReadyIssuer.DeepCopy()
	readyCondition := notReadyCondition.DeepCopy()
	readyCondition.Status = cmmeta.ConditionTrue
	readyIssuer.Status.Conditions[0] = *readyCondition

	multipleConditionsIssuer := readyIssuer.DeepCopy()
	// cert manager does not have multiple issuer conditions (yet)
	acmeCondition := certmanagerv1.IssuerCondition{Type: "DummyCondition", Status: cmmeta.ConditionFalse}
	multipleConditionsIssuer.Status.Conditions = []certmanagerv1.IssuerCondition{acmeCondition, *readyCondition}

	tests := []struct {
		desc                string
		initialIssuer       *certmanagerv1.Issuer
		shouldError         bool
		afterStartedWaiting func(*testing.T, context.Context, versioned.Interface)
	}{
		{
			desc:          "issuer starts ready",
			initialIssuer: readyIssuer,
		},
		{
			desc:          "issuer not ready",
			initialIssuer: notReadyIssuer,
			shouldError:   true,
		},
		{
			desc:          "issuer has no status",
			initialIssuer: noStatusIssuer,
			shouldError:   true,
		},
		{
			desc:        "issuer does not exist",
			shouldError: true,
		},
		{
			desc:          "issuer becomes ready",
			initialIssuer: notReadyIssuer,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.CertmanagerV1().Issuers(issuerNamespace).Update(ctx, readyIssuer, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:          "multiple conditions",
			initialIssuer: notReadyIssuer,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.CertmanagerV1().Issuers(issuerNamespace).Update(ctx, multipleConditionsIssuer, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			client, fakeClientset := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialIssuer != nil {
				_, err := fakeClientset.CertmanagerV1().Issuers(issuerNamespace).Create(ctx, tt.initialIssuer, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var issuer *certmanagerv1.Issuer
			wg.Add(1)
			go func() {
				issuer, waitErr = client.WaitForReadyIssuer(ctx, issuerNamespace, issuerName, WaitForReadyIssuerOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, fakeClientset)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, issuer)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, issuer)
		})
	}
}

func TestReissueCertificate(t *testing.T) {
	namespace := "test-ns"
	certName := "test-cert"

	existingCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  namespace,
			Name:       certName,
			Generation: 1,
		},
	}

	tests := []struct {
		desc                  string
		setupCert             *certmanagerv1.Certificate
		simulateGetFailure    bool
		simulateUpdateFailure bool
		wantErr               bool
	}{
		{
			desc:      "successfully reissue certificate",
			setupCert: existingCert,
		},
		{
			desc:               "get certificate fails",
			setupCert:          existingCert,
			simulateGetFailure: true,
			wantErr:            true,
		},
		{
			desc:                  "update status fails",
			setupCert:             existingCert,
			simulateUpdateFailure: true,
			wantErr:               true,
		},
		{
			desc:    "certificate does not exist",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, fakeClientset := createTestClient()
			ctx := th.NewTestContext()

			if tt.setupCert != nil {
				_, err := client.client.CertmanagerV1().Certificates(namespace).Create(ctx, tt.setupCert, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.simulateGetFailure {
				fakeClientset.PrependReactor("get", "certificates", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			if tt.simulateUpdateFailure {
				fakeClientset.PrependReactor("update", "certificates", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			cert, err := client.ReissueCertificate(ctx, namespace, certName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cert)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, cert)

			for _, condition := range cert.Status.Conditions {
				if condition.Type == certmanagerv1.CertificateConditionIssuing {
					assert.Equal(t, cmmeta.ConditionTrue, condition.Status)
					assert.Equal(t, "ManuallyTriggered", condition.Reason)
					return
				}
			}

			t.Error("Issuing condition not found")
		})
	}
}

func TestDeleteIssuer(t *testing.T) {
	namespace := "test-ns"
	issuerName := "test-issuer"

	tests := []struct {
		desc              string
		shouldSetupIssuer bool
		wantErr           bool
	}{
		{
			desc:              "delete existing issuer",
			shouldSetupIssuer: true,
		},
		{
			desc:    "delete non-existent issuer",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, _ := createTestClient()
			ctx := th.NewTestContext()

			var existingIssuer *certmanagerv1.Issuer
			if tt.shouldSetupIssuer {
				existingIssuer = &certmanagerv1.Issuer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      issuerName,
						Namespace: namespace,
					},
				}
				_, err := client.client.CertmanagerV1().Issuers(namespace).Create(ctx, existingIssuer, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteIssuer(ctx, namespace, issuerName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the issuer was deleted
			issuerList, err := client.client.CertmanagerV1().Issuers(namespace).List(ctx, metav1.SingleObject(metav1.ObjectMeta{Name: issuerName, Namespace: namespace}))
			assert.NoError(t, err)
			assert.Empty(t, issuerList.Items)
		})
	}
}
