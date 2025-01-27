package kubecluster

import (
	context "context"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetClusterDomainNames(t *testing.T) {
	tests := []struct {
		desc        string
		clusterName string
		namespace   string
		expected    []string
	}{
		{
			desc:        "basic cluster name and namespace",
			clusterName: "test-cluster",
			namespace:   "test-ns",
			expected: []string{
				"test-cluster-r",
				"test-cluster-r.test-ns",
				"test-cluster-r.test-ns.svc",
				"test-cluster-ro",
				"test-cluster-ro.test-ns",
				"test-cluster-ro.test-ns.svc",
				"test-cluster-rw",
				"test-cluster-rw.test-ns",
				"test-cluster-rw.test-ns.svc",
			},
		},
		{
			desc:        "cluster name with numbers characters",
			clusterName: "my-special-cluster-123",
			namespace:   "prod-ns",
			expected: []string{
				"my-special-cluster-123-r",
				"my-special-cluster-123-r.prod-ns",
				"my-special-cluster-123-r.prod-ns.svc",
				"my-special-cluster-123-ro",
				"my-special-cluster-123-ro.prod-ns",
				"my-special-cluster-123-ro.prod-ns.svc",
				"my-special-cluster-123-rw",
				"my-special-cluster-123-rw.prod-ns",
				"my-special-cluster-123-rw.prod-ns.svc",
			},
		},
		{
			desc:        "short names",
			clusterName: "c1",
			namespace:   "ns",
			expected: []string{
				"c1-r",
				"c1-r.ns",
				"c1-r.ns.svc",
				"c1-ro",
				"c1-ro.ns",
				"c1-ro.ns.svc",
				"c1-rw",
				"c1-rw.ns",
				"c1-rw.ns.svc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := getClusterDomainNames(tt.clusterName, tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCredentials(t *testing.T) {
	tests := []struct {
		desc                string
		cluster             *apiv1.Cluster
		servingCertMountDir string
		clientCertMountDir  string
		expectedCredentials *cnpg.KubernetesSecretCredentials
	}{
		{
			desc: "basic credentials",
			cluster: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
			servingCertMountDir: "/certs/server",
			clientCertMountDir:  "/certs/client",
			expectedCredentials: &cnpg.KubernetesSecretCredentials{
				Host:                         "test-cluster-rw.test-ns.svc",
				ServingCertificateCAFilePath: "/certs/server/ca.crt",
				ClientCertificateFilePath:    "/certs/client/tls.crt",
				ClientPrivateKeyFilePath:     "/certs/client/tls.key",
			},
		},
		{
			desc: "different mount paths",
			cluster: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prod-db",
					Namespace: "prod",
				},
			},
			servingCertMountDir: "/var/run/secrets/server-cert",
			clientCertMountDir:  "/var/run/secrets/client-cert",
			expectedCredentials: &cnpg.KubernetesSecretCredentials{
				Host:                         "prod-db-rw.prod.svc",
				ServingCertificateCAFilePath: "/var/run/secrets/server-cert/ca.crt",
				ClientCertificateFilePath:    "/var/run/secrets/client-cert/tls.crt",
				ClientPrivateKeyFilePath:     "/var/run/secrets/client-cert/tls.key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{
				cluster: tt.cluster,
			}

			creds := cc.GetCredentials(tt.servingCertMountDir, tt.clientCertMountDir)
			assert.Equal(t, tt.expectedCredentials, creds)
		})
	}
}

func TestNewClonedCluster(t *testing.T) {
	t.Run("returns new ClonedCluster with client reference set", func(t *testing.T) {
		c := NewClient(nil, nil, nil, nil)
		cc := newClonedCluster(c)
		casted := cc.(*ClonedCluster)

		require.NotNil(t, cc)
		assert.Equal(t, c, casted.c)
	})
}

func TestCloneClusterOptions(t *testing.T) {
	th.OptStructTest[CloneClusterOptions](t)
}

func TestCloneCluster(t *testing.T) {
	tests := []struct {
		desc                             string
		opts                             CloneClusterOptions
		simulateErrorOnClusterCleanup    bool
		simulateGetExistingClusterError  bool
		simulateBackupError              bool
		simulateBackupCleanupError       bool
		simulateWaitingForBackupError    bool
		simulateServingCertCreationError bool
		simulateWaitForServingCertError  bool
		simulateClientCertCreationError  bool
		simulateWaitForClientCertError   bool
		simulateClusterCreationError     bool
		simulateWaitForClusterError      bool
	}{
		{
			desc: "basic clone",
		},
		{
			desc: "all opts set except for generate name",
			opts: CloneClusterOptions{
				WaitForBackupTimeout: helpers.MaxWaitTime(time.Minute),
				ServingCertSubject: &certmanagerv1.X509Subject{
					Organizations: []string{"test-org"},
				},
				ServingCertIssuerKind:     "ClusterIssuer",
				WaitForServingCertTimeout: helpers.MaxWaitTime(2 * time.Minute),
				ClientCertSubject: &certmanagerv1.X509Subject{
					Organizations: []string{"test-org"},
				},
				ClientCertIssuerKind:     "ClusterIssuer",
				WaitForClientCertTimeout: helpers.MaxWaitTime(3 * time.Minute),
				RecoveryTargetTime:       time.Now().Add(-time.Hour).Format(time.RFC3339),
				WaitForClusterTimeout:    helpers.MaxWaitTime(4 * time.Minute),
			},
		},
		{
			desc:                            "simulate error getting existing cluster",
			simulateGetExistingClusterError: true,
		},
		{
			desc:                            "simulate error getting existing cluster, and cleaning up",
			simulateErrorOnClusterCleanup:   true,
			simulateGetExistingClusterError: true,
		},
		{
			desc:                "simulate error backing up existing cluster",
			simulateBackupError: true,
		},
		{
			desc:                       "simulate error cleaning up backup",
			simulateBackupCleanupError: true,
		},
		{
			desc:                          "simulate error waiting for backup",
			simulateWaitingForBackupError: true,
		},
		{
			desc:                             "simulate error creating serving cert",
			simulateServingCertCreationError: true,
		},
		{
			desc:                            "simulate error waiting for serving cert",
			simulateWaitForServingCertError: true,
		},
		{
			desc:                            "simulate error creating client cert",
			simulateClientCertCreationError: true,
		},
		{
			desc:                           "simulate error waiting for client cert",
			simulateWaitForClientCertError: true,
		},
		{
			desc:                         "simulate error creating cluster",
			simulateClusterCreationError: true,
		},
		{
			desc:                        "simulate error waiting for cluster",
			simulateWaitForClusterError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			// Track creation parameters
			ctx := context.Background()
			namespace := "test-ns"
			existingClusterName := "existing-cluster"
			newClusterName := "new-cluster"
			servingIssuerName := "test-serving-issuer"
			clientIssuerName := "test-client-issuer"

			// Setup response values for mocks
			existingCluster := &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      existingClusterName,
					Namespace: namespace,
				},
				Spec: apiv1.ClusterSpec{
					StorageConfiguration: apiv1.StorageConfiguration{
						Size: "10Gi",
					},
				},
			}
			createdBackup := &apiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      existingClusterName + "-cloned",
					Namespace: namespace,
				},
			}
			createdServingCert := &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      newClusterName + "-serving-cert",
					Namespace: namespace,
				},
			}
			createdClientCert := &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      newClusterName + "-postgres-user",
					Namespace: namespace,
				},
			}
			newCluster := &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      newClusterName,
					Namespace: namespace,
				},
				Spec: apiv1.ClusterSpec{
					StorageConfiguration: apiv1.StorageConfiguration{
						Size: "10Gi",
					},
				},
			}

			// Setup parameters for mocks that may vary depending on the test
			clusterOpts := cnpg.CreateClusterOptions{
				BackupName: createdBackup.Name,
			}
			if tt.opts.RecoveryTargetTime != "" {
				clusterOpts.RecoveryTarget = &apiv1.RecoveryTarget{
					TargetTime: tt.opts.RecoveryTargetTime,
				}
			}

			errorExpected := th.ErrExpected(
				tt.simulateErrorOnClusterCleanup,
				tt.simulateGetExistingClusterError,
				tt.simulateBackupError,
				tt.simulateBackupCleanupError,
				tt.simulateWaitingForBackupError,
				tt.simulateServingCertCreationError,
				tt.simulateWaitForServingCertError,
				tt.simulateClientCertCreationError,
				tt.simulateWaitForClientCertError,
				tt.simulateClusterCreationError,
				tt.simulateWaitForClusterError,
			)

			// Setup mocks
			c := newMockClient(t)

			// This makes the logic for setting up mocks/expected calls easier, because once an error
			// becomes expected, the function can be returned from
			func() {
				if errorExpected {
					c.clonedCluster.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx context.Context) error {
						require.NotEqual(t, ctx, cleanupCtx) // This should be a different context with a timeout
						return th.ErrIfTrue(tt.simulateErrorOnClusterCleanup)
					})
				}

				c.cnpgClient.EXPECT().GetCluster(ctx, namespace, existingCluster.Name).Return(th.ErrOr1Val(existingCluster, tt.simulateGetExistingClusterError))
				if tt.simulateGetExistingClusterError {
					return
				}

				c.cnpgClient.EXPECT().CreateBackup(ctx, namespace, createdBackup.Name, existingCluster.Name, cnpg.CreateBackupOptions{GenerateName: true}).
					Return(th.ErrOr1Val(createdBackup, tt.simulateBackupError))
				if tt.simulateBackupError {
					return
				}

				c.cnpgClient.EXPECT().DeleteBackup(mock.Anything, namespace, createdBackup.Name).Return(th.ErrIfTrue(tt.simulateBackupCleanupError))
				c.cnpgClient.EXPECT().WaitForReadyBackup(ctx, namespace, createdBackup.Name, cnpg.WaitForReadyBackupOpts{MaxWaitTime: tt.opts.WaitForBackupTimeout}).
					Return(th.ErrOr1Val(createdBackup, tt.simulateWaitingForBackupError))
				if tt.simulateWaitingForBackupError {
					return
				}

				c.cmClient.EXPECT().CreateCertificate(ctx, namespace, helpers.CleanName(createdServingCert.Name), servingIssuerName, certmanager.CreateCertificateOptions{
					CommonName: createdServingCert.Name,
					DNSNames:   getClusterDomainNames(newClusterName, namespace),
					SecretLabels: map[string]string{
						"cnpg.io/reload": "true",
					},
					Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth},
					IssuerKind: tt.opts.ServingCertIssuerKind,
					Subject:    tt.opts.ServingCertSubject,
				}).Return(th.ErrOr1Val(createdServingCert, tt.simulateServingCertCreationError))
				if tt.simulateServingCertCreationError {
					return
				}

				c.clonedCluster.EXPECT().setServingCert(createdServingCert).Return()
				c.cmClient.EXPECT().WaitForReadyCertificate(ctx, namespace, createdServingCert.Name, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: tt.opts.WaitForServingCertTimeout}).
					Return(th.ErrOr1Val(createdServingCert, tt.simulateWaitForServingCertError))
				if tt.simulateWaitForServingCertError {
					return
				}
				c.clonedCluster.EXPECT().setServingCert(createdServingCert).Return()

				c.cmClient.EXPECT().CreateCertificate(ctx, namespace, helpers.CleanName(createdClientCert.Name), clientIssuerName, certmanager.CreateCertificateOptions{
					CommonName: "postgres",
					SecretLabels: map[string]string{
						"cnpg.io/reload": "true",
					},
					Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageClientAuth},
					IssuerKind: tt.opts.ClientCertIssuerKind,
					Subject:    tt.opts.ClientCertSubject,
				}).Return(th.ErrOr1Val(createdClientCert, tt.simulateClientCertCreationError))
				if tt.simulateClientCertCreationError {
					return
				}

				c.clonedCluster.EXPECT().setClientCert(createdClientCert).Return()
				c.cmClient.EXPECT().WaitForReadyCertificate(ctx, namespace, createdClientCert.Name, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: tt.opts.WaitForClientCertTimeout}).
					Return(th.ErrOr1Val(createdClientCert, tt.simulateWaitForClientCertError))
				if tt.simulateWaitForClientCertError {
					return
				}
				c.clonedCluster.EXPECT().setClientCert(createdClientCert).Return()

				c.cnpgClient.EXPECT().CreateCluster(ctx, namespace, newCluster.Name, resource.MustParse(existingCluster.Spec.StorageConfiguration.Size), createdServingCert.Name, createdClientCert.Name, clusterOpts).
					Return(th.ErrOr1Val(newCluster, tt.simulateClusterCreationError))
				if tt.simulateClusterCreationError {
					return
				}

				c.clonedCluster.EXPECT().setCluster(newCluster).Return()
				c.cnpgClient.EXPECT().WaitForReadyCluster(ctx, namespace, newCluster.Name, cnpg.WaitForReadyClusterOpts{MaxWaitTime: tt.opts.WaitForClusterTimeout}).
					Return(th.ErrOr1Val(newCluster, tt.simulateWaitForClusterError))
				if tt.simulateWaitForClusterError {
					return
				}

				c.clonedCluster.EXPECT().setCluster(newCluster).Return()
			}()

			// Run the function
			clonedCluster, err := c.CloneCluster(ctx, namespace, existingClusterName, newClusterName, servingIssuerName, clientIssuerName, tt.opts)

			// Test the results
			if errorExpected {
				require.Error(t, err)
				require.Nil(t, clonedCluster)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, clonedCluster)

			// The expected "set" functions above confirm that the clonedcluster values are set correctly
		})
	}
}

func TestClonedClusterWhenFailToParseExistingClusterStorageSize(t *testing.T) {
	c := newMockClient(t)
	ctx := context.Background()

	c.cnpgClient.EXPECT().GetCluster(ctx, "test-ns", "existing-cluster").Return(&apiv1.Cluster{
		Spec: apiv1.ClusterSpec{
			StorageConfiguration: apiv1.StorageConfiguration{
				Size: "not-a-size",
			},
		},
	}, nil)
	c.clonedCluster.EXPECT().Delete(mock.Anything).Return(nil)

	clonedCluster, err := c.CloneCluster(ctx, "test-ns", "existing-cluster", "new-cluster", "issuer-1", "issuer-2", CloneClusterOptions{})
	require.Error(t, err)
	require.Nil(t, clonedCluster)
}

func TestClonedClusterDelete(t *testing.T) {
	cluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
	}

	servingCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "serving-cert", Namespace: "test-ns"},
	}

	clientCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "client-cert", Namespace: "test-ns"},
	}

	allResourcesCluster := ClonedCluster{
		cluster:            cluster,
		servingCertificate: servingCert,
		clientCertificate:  clientCert,
	}

	tests := []struct {
		desc                           string
		cc                             ClonedCluster
		simulateClusterDeleteError     bool
		simulateClientCertDeleteError  bool
		simulateServingCertDeleteError bool
		expectedErrorsInMessage        int
	}{
		{
			desc: "delete all resources",
			cc:   allResourcesCluster,
		},
		{
			desc: "delete with no cluster",
			cc: ClonedCluster{
				servingCertificate: servingCert,
				clientCertificate:  clientCert,
			},
		},
		{
			desc: "delete with just serving cert",
			cc: ClonedCluster{
				servingCertificate: servingCert,
			},
		},
		{
			desc: "delete with just client cert",
			cc: ClonedCluster{
				servingCertificate: servingCert,
				clientCertificate:  clientCert,
			},
		},
		{
			desc: "delete with just cluster",
			cc: ClonedCluster{
				cluster: cluster,
			},
		},
		{
			desc: "delete empty cloned cluster",
		},
		{
			desc:                           "all deletions fail",
			cc:                             allResourcesCluster,
			simulateClusterDeleteError:     true,
			simulateClientCertDeleteError:  true,
			simulateServingCertDeleteError: true,
			expectedErrorsInMessage:        3,
		},
		{
			desc:                       "cluster deletion fails",
			cc:                         allResourcesCluster,
			simulateClusterDeleteError: true,
			expectedErrorsInMessage:    1,
		},
		{
			desc:                           "certificate deletions fail",
			cc:                             allResourcesCluster,
			simulateClientCertDeleteError:  true,
			simulateServingCertDeleteError: true,
			expectedErrorsInMessage:        2,
		},
		{
			desc:                           "serving certificate deletion fails",
			cc:                             allResourcesCluster,
			simulateServingCertDeleteError: true,
			expectedErrorsInMessage:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.Background()
			c := newMockClient(t)
			tt.cc.c = c

			if tt.cc.cluster != nil {
				c.cnpgClient.EXPECT().DeleteCluster(ctx, tt.cc.cluster.Namespace, tt.cc.cluster.Name).Return(th.ErrIfTrue(tt.simulateClusterDeleteError))
			}

			if tt.cc.clientCertificate != nil {
				c.cmClient.EXPECT().DeleteCertificate(ctx, tt.cc.clientCertificate.Namespace, tt.cc.clientCertificate.Name).Return(th.ErrIfTrue(tt.simulateClientCertDeleteError))
			}

			if tt.cc.servingCertificate != nil {
				c.cmClient.EXPECT().DeleteCertificate(ctx, tt.cc.clientCertificate.Namespace, tt.cc.servingCertificate.Name).Return(th.ErrIfTrue(tt.simulateServingCertDeleteError))
			}

			err := tt.cc.Delete(ctx)
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

func TestClonedClusterSetServingCert(t *testing.T) {
	tests := []struct {
		desc string
		cert *certmanagerv1.Certificate
	}{
		{
			desc: "set non-nil certificate",
			cert: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setServingCert(tt.cert)
			assert.Equal(t, tt.cert, cc.servingCertificate)
		})
	}
}

func TestClonedClusterGetServingCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   ClonedCluster
		want *certmanagerv1.Certificate
	}{
		{
			desc: "get existing serving certificate",
			cc: ClonedCluster{
				servingCertificate: &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cert",
						Namespace: "test-ns",
					},
				},
			},
			want: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil serving certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetServingCert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetClientCert(t *testing.T) {
	tests := []struct {
		desc string
		cert *certmanagerv1.Certificate
	}{
		{
			desc: "set non-nil certificate",
			cert: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setClientCert(tt.cert)
			assert.Equal(t, tt.cert, cc.clientCertificate)
		})
	}
}

func TestClonedClusterGetClientCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   ClonedCluster
		want *certmanagerv1.Certificate
	}{
		{
			desc: "get existing client certificate",
			cc: ClonedCluster{
				clientCertificate: &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cert",
						Namespace: "test-ns",
					},
				},
			},
			want: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil client certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetClientCert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetCluster(t *testing.T) {
	tests := []struct {
		desc    string
		cluster *apiv1.Cluster
	}{
		{
			desc: "set non-nil cluster",
			cluster: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setCluster(tt.cluster)
			assert.Equal(t, tt.cluster, cc.cluster)
		})
	}
}

func TestClonedClusterGetCluster(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want *apiv1.Cluster
	}{
		{
			desc: "get existing cluster",
			cc: &ClonedCluster{
				cluster: &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-ns",
					},
				},
			},
			want: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil cluster",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetCluster()
			assert.Equal(t, tt.want, got)
		})
	}
}
