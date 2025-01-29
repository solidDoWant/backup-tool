package clonedcluster

import (
	context "context"
	"fmt"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
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
		p := NewProvider(nil, nil, nil)
		cc := newClonedCluster(p)
		casted := cc.(*ClonedCluster)

		require.NotNil(t, cc)
		assert.Equal(t, p, casted.p)
	})
}

func TestCloneClusterOptions(t *testing.T) {
	th.OptStructTest[CloneClusterOptions](t)
}

func TestCloneCluster(t *testing.T) {
	tests := []struct {
		desc                                          string
		opts                                          CloneClusterOptions
		simulateErrorOnClusterCleanup                 bool
		simulateGetExistingClusterError               bool
		simulateBackupError                           bool
		simulateBackupCleanupError                    bool
		simulateWaitingForBackupError                 bool
		simulateServingCertCreationError              bool
		simulateWaitForServingCertError               bool
		simulateClientCACertCreationError             bool
		simulateWaitForClientCACertError              bool
		simulateClientCAIssuerCreationError           bool
		simulateWaitForClientCAIssuerError            bool
		simulatePostgresUserCertCreationError         bool
		simulateStreamingReplicaUserCertCreationError bool
		simulateClusterCreationError                  bool
		simulateWaitForClusterError                   bool
	}{
		{
			desc: "basic clone",
		},
		{
			desc: "all opts set except for generate name",
			opts: CloneClusterOptions{
				WaitForBackupTimeout: helpers.MaxWaitTime(time.Minute),
				Certificates: CloneClusterOptionsCertificates{
					ServingCert: CloneClusterOptionsExternallyIssuedCertificate{
						IssuerKind: "ClusterIssuer",
						CloneClusterOptionsCertificate: CloneClusterOptionsCertificate{
							Subject: &certmanagerv1.X509Subject{
								Organizations: []string{"test-org"},
							},
							WaitForReadyTimeout: helpers.MaxWaitTime(2 * time.Minute),
						},
					},
					ClientCACert: CloneClusterOptionsExternallyIssuedCertificate{
						IssuerKind: "ClusterIssuer2",
						CloneClusterOptionsCertificate: CloneClusterOptionsCertificate{
							Subject: &certmanagerv1.X509Subject{
								OrganizationalUnits: []string{"test-ou"},
							},
							WaitForReadyTimeout: helpers.MaxWaitTime(3 * time.Minute),
						},
					},
					PostgresUserCert: CloneClusterOptionsInternallyIssuedCertificate{
						CloneClusterOptionsCertificate: CloneClusterOptionsCertificate{
							Subject: &certmanagerv1.X509Subject{
								Countries: []string{"test-country"},
							},
							WaitForReadyTimeout: helpers.MaxWaitTime(4 * time.Minute),
						},
						CRPOpts: clusterusercert.NewClusterUserCertOptsCRP{
							Enabled:           true,
							WaitForCRPTimeout: helpers.MaxWaitTime(5 * time.Minute),
						},
					},
					StreamingReplicaUserCert: CloneClusterOptionsInternallyIssuedCertificate{
						CloneClusterOptionsCertificate: CloneClusterOptionsCertificate{
							Subject: &certmanagerv1.X509Subject{
								Provinces: []string{"test-province"},
							},
							WaitForReadyTimeout: helpers.MaxWaitTime(6 * time.Minute),
						},
						CRPOpts: clusterusercert.NewClusterUserCertOptsCRP{
							Enabled:           true,
							WaitForCRPTimeout: helpers.MaxWaitTime(7 * time.Minute),
						},
					},
				},
				RecoveryTargetTime:    time.Now().Add(-time.Hour).Format(time.RFC3339),
				WaitForClusterTimeout: helpers.MaxWaitTime(8 * time.Minute),
				CleanupTimeout:        helpers.MaxWaitTime(9 * time.Minute),
				ClientCAIssuer: CloneClusterOptionsCAIssuer{
					WaitForReadyTimeout: helpers.MaxWaitTime(10 * time.Minute),
				},
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
			desc:                              "simulate error creating client cert",
			simulateClientCACertCreationError: true,
		},
		{
			desc:                             "simulate error waiting for client cert",
			simulateWaitForClientCACertError: true,
		},
		{
			desc:                                "simulate error creating client CA issuer",
			simulateClientCAIssuerCreationError: true,
		},
		{
			desc:                               "simulate error waiting for client CA issuer",
			simulateWaitForClientCAIssuerError: true,
		},
		{
			desc:                                  "simulate error creating postgres user cert",
			simulatePostgresUserCertCreationError: true,
		},
		{
			desc: "simulate error creating streaming replica user cert",
			simulateStreamingReplicaUserCertCreationError: true,
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
			clientIssuerName := "test-client-ca-cert-issuer"

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
			createdClientCACert := &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      newClusterName + "-client-ca",
					Namespace: namespace,
				},
			}
			createdClientCAIssuer := &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      newClusterName + "-client-ca-issuer",
					Namespace: namespace,
				},
			}
			postgresUserCert := &clusterusercert.ClusterUserCert{}
			streamingReplicaUserCert := &clusterusercert.ClusterUserCert{}
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
				tt.simulateClientCACertCreationError,
				tt.simulateWaitForClientCACertError,
				tt.simulateClientCAIssuerCreationError,
				tt.simulateWaitForClientCAIssuerError,
				tt.simulatePostgresUserCertCreationError,
				tt.simulateStreamingReplicaUserCertCreationError,
				tt.simulateClusterCreationError,
				tt.simulateWaitForClusterError,
			)

			// Setup mocks
			p := newMockProvider(t)

			// This makes the logic for setting up mocks/expected calls easier, because once an error
			// becomes expected, the function can be returned from
			func() {
				// 0. Setup
				if errorExpected {
					p.clonedCluster.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx context.Context) error {
						require.NotEqual(t, ctx, cleanupCtx) // This should be a different context with a timeout
						return th.ErrIfTrue(tt.simulateErrorOnClusterCleanup)
					})
				}

				p.cnpgClient.EXPECT().GetCluster(ctx, namespace, existingCluster.Name).Return(th.ErrOr1Val(existingCluster, tt.simulateGetExistingClusterError))
				if tt.simulateGetExistingClusterError {
					return
				}

				// 1.
				p.cnpgClient.EXPECT().CreateBackup(ctx, namespace, createdBackup.Name, existingCluster.Name, cnpg.CreateBackupOptions{GenerateName: true}).
					Return(th.ErrOr1Val(createdBackup, tt.simulateBackupError))
				if tt.simulateBackupError {
					return
				}

				p.cnpgClient.EXPECT().DeleteBackup(mock.Anything, namespace, createdBackup.Name).Return(th.ErrIfTrue(tt.simulateBackupCleanupError))
				p.cnpgClient.EXPECT().WaitForReadyBackup(ctx, namespace, createdBackup.Name, cnpg.WaitForReadyBackupOpts{MaxWaitTime: tt.opts.WaitForBackupTimeout}).
					Return(th.ErrOr1Val(createdBackup, tt.simulateWaitingForBackupError))
				if tt.simulateWaitingForBackupError {
					return
				}

				// 2.
				p.cmClient.EXPECT().CreateCertificate(ctx, namespace, helpers.CleanName(createdServingCert.Name), servingIssuerName, certmanager.CreateCertificateOptions{
					CommonName: createdServingCert.Name,
					DNSNames:   getClusterDomainNames(newClusterName, namespace),
					SecretLabels: map[string]string{
						"cnpg.io/reload": "true",
					},
					Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth},
					IssuerKind: tt.opts.Certificates.ServingCert.IssuerKind,
					Subject:    tt.opts.Certificates.ServingCert.Subject,
				}).Return(th.ErrOr1Val(createdServingCert, tt.simulateServingCertCreationError))
				if tt.simulateServingCertCreationError {
					return
				}
				p.clonedCluster.EXPECT().setServingCert(createdServingCert)

				p.cmClient.EXPECT().WaitForReadyCertificate(ctx, namespace, createdServingCert.Name, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: tt.opts.Certificates.ServingCert.WaitForReadyTimeout}).
					Return(th.ErrOr1Val(createdServingCert, tt.simulateWaitForServingCertError))
				if tt.simulateWaitForServingCertError {
					return
				}
				p.clonedCluster.EXPECT().setServingCert(createdServingCert)

				// 3.
				// 3.1
				clientCACertName := helpers.CleanName(createdClientCACert.Name)
				p.cmClient.EXPECT().CreateCertificate(ctx, namespace, clientCACertName, clientIssuerName, certmanager.CreateCertificateOptions{
					IsCA: true,
					CAConstraints: &certmanagerv1.NameConstraints{
						Critical: true,
						Excluded: &certmanagerv1.NameConstraintItem{
							DNSDomains:     []string{},
							IPRanges:       []string{},
							EmailAddresses: []string{},
							URIDomains:     []string{},
						},
					},
					CommonName: fmt.Sprintf("%s CNPG CA", newClusterName),
					Subject:    tt.opts.Certificates.ClientCACert.Subject,
					Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageCertSign},
					SecretLabels: map[string]string{
						utils.WatchedLabelName: "true",
					},
					IssuerKind: tt.opts.Certificates.ClientCACert.IssuerKind,
				}).Return(th.ErrOr1Val(createdClientCACert, tt.simulateClientCACertCreationError))
				if tt.simulateClientCACertCreationError {
					return
				}
				p.clonedCluster.EXPECT().setClientCACert(createdClientCACert)

				p.cmClient.EXPECT().WaitForReadyCertificate(ctx, namespace, createdClientCACert.Name, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: tt.opts.Certificates.ClientCACert.WaitForReadyTimeout}).
					Return(th.ErrOr1Val(createdClientCACert, tt.simulateWaitForClientCACertError))
				if tt.simulateWaitForClientCACertError {
					return
				}
				p.clonedCluster.EXPECT().setClientCACert(createdClientCACert)

				// 3.2
				p.cmClient.EXPECT().CreateIssuer(ctx, namespace, mock.Anything, clientCACertName, certmanager.CreateIssuerOptions{}).
					RunAndReturn(func(ctx context.Context, namespace, name, clientCACertName string, opts certmanager.CreateIssuerOptions) (*certmanagerv1.Issuer, error) {
						assert.Contains(t, name, clientCACertName)
						return th.ErrOr1Val(createdClientCAIssuer, tt.simulateClientCAIssuerCreationError)
					})
				if tt.simulateClientCAIssuerCreationError {
					return
				}
				p.clonedCluster.EXPECT().setClientCAIssuer(createdClientCAIssuer)

				p.cmClient.EXPECT().WaitForReadyIssuer(ctx, namespace, mock.Anything, certmanager.WaitForReadyIssuerOpts{MaxWaitTime: tt.opts.ClientCAIssuer.WaitForReadyTimeout}).
					Return(th.ErrOr1Val(createdClientCAIssuer, tt.simulateWaitForClientCAIssuerError))
				if tt.simulateWaitForClientCAIssuerError {
					return
				}
				p.clonedCluster.EXPECT().setClientCAIssuer(createdClientCAIssuer)

				// 4.
				p.cucp.EXPECT().NewClusterUserCert(ctx, namespace, "postgres", createdClientCAIssuer.Name, newClusterName, clusterusercert.NewClusterUserCertOpts{
					Subject:            tt.opts.Certificates.PostgresUserCert.Subject,
					CRPOpts:            tt.opts.Certificates.PostgresUserCert.CRPOpts,
					WaitForCertTimeout: tt.opts.Certificates.PostgresUserCert.WaitForReadyTimeout,
					CleanupTimeout:     tt.opts.CleanupTimeout,
				}).Return(th.ErrOr1Val(postgresUserCert, tt.simulatePostgresUserCertCreationError))
				if tt.simulatePostgresUserCertCreationError {
					return
				}
				p.clonedCluster.EXPECT().setPostgresUserCert(postgresUserCert)

				// 5.
				p.cucp.EXPECT().NewClusterUserCert(ctx, namespace, "streaming_replica", createdClientCAIssuer.Name, newClusterName, clusterusercert.NewClusterUserCertOpts{
					Subject:            tt.opts.Certificates.StreamingReplicaUserCert.Subject,
					CRPOpts:            tt.opts.Certificates.StreamingReplicaUserCert.CRPOpts,
					WaitForCertTimeout: tt.opts.Certificates.StreamingReplicaUserCert.WaitForReadyTimeout,
					CleanupTimeout:     tt.opts.CleanupTimeout,
				}).Return(th.ErrOr1Val(streamingReplicaUserCert, tt.simulateStreamingReplicaUserCertCreationError))
				if tt.simulateStreamingReplicaUserCertCreationError {
					return
				}
				p.clonedCluster.EXPECT().setStreamingReplicaUserCert(streamingReplicaUserCert)

				// 6.
				p.cnpgClient.EXPECT().CreateCluster(ctx, namespace, newCluster.Name, resource.MustParse(existingCluster.Spec.StorageConfiguration.Size), createdServingCert.Name, createdClientCACert.Name, clusterOpts).
					Return(th.ErrOr1Val(newCluster, tt.simulateClusterCreationError))
				if tt.simulateClusterCreationError {
					return
				}

				p.clonedCluster.EXPECT().setCluster(newCluster).Return()
				p.cnpgClient.EXPECT().WaitForReadyCluster(ctx, namespace, newCluster.Name, cnpg.WaitForReadyClusterOpts{MaxWaitTime: tt.opts.WaitForClusterTimeout}).
					Return(th.ErrOr1Val(newCluster, tt.simulateWaitForClusterError))
				if tt.simulateWaitForClusterError {
					return
				}

				p.clonedCluster.EXPECT().setCluster(newCluster).Return()
			}()

			// Run the function
			clonedCluster, err := p.CloneCluster(ctx, namespace, existingClusterName, newClusterName, servingIssuerName, clientIssuerName, tt.opts)

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
	p := newMockProvider(t)
	ctx := context.Background()

	p.cnpgClient.EXPECT().GetCluster(ctx, "test-ns", "existing-cluster").Return(&apiv1.Cluster{
		Spec: apiv1.ClusterSpec{
			StorageConfiguration: apiv1.StorageConfiguration{
				Size: "not-a-size",
			},
		},
	}, nil)
	p.clonedCluster.EXPECT().Delete(mock.Anything).Return(nil)

	clonedCluster, err := p.CloneCluster(ctx, "test-ns", "existing-cluster", "new-cluster", "issuer-1", "issuer-2", CloneClusterOptions{})
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

	clientCACert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "client-ca-cert", Namespace: "test-ns"},
	}

	clientCAIssuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{Name: "client-ca-issuer", Namespace: "test-ns"},
	}

	allResourcesCluster := ClonedCluster{
		cluster:             cluster,
		servingCertificate:  servingCert,
		clientCACertificate: clientCACert,
		clientCAIssuer:      clientCAIssuer,
	}

	tests := []struct {
		desc                                        string
		cc                                          ClonedCluster
		includePostgresUserCert                     bool // Workaround as these are mocked types that require test-specific t
		includeStreamingReplicaUserCert             bool
		simulateClusterDeleteError                  bool
		simulateStreamingReplicaUserCertDeleteError bool
		simulatePostgresUserCertDeleteError         bool
		simulateClientCAIssuerDeleteError           bool
		simulateClientCACertDeleteError             bool
		simulateServingCertDeleteError              bool
		expectedErrorsInMessage                     int
	}{
		{
			desc:                            "delete all resources",
			cc:                              allResourcesCluster,
			includePostgresUserCert:         true,
			includeStreamingReplicaUserCert: true,
		},
		{
			desc: "delete with no cluster",
			cc: ClonedCluster{
				servingCertificate:  servingCert,
				clientCACertificate: clientCACert,
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
				servingCertificate:  servingCert,
				clientCACertificate: clientCACert,
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
			desc:                            "all deletions fail",
			cc:                              allResourcesCluster,
			includePostgresUserCert:         true,
			includeStreamingReplicaUserCert: true,
			simulateClusterDeleteError:      true,
			simulateStreamingReplicaUserCertDeleteError: true,
			simulatePostgresUserCertDeleteError:         true,
			simulateClientCAIssuerDeleteError:           true,
			simulateClientCACertDeleteError:             true,
			simulateServingCertDeleteError:              true,
			expectedErrorsInMessage:                     6,
		},
		{
			desc:                       "cluster deletion fails",
			cc:                         allResourcesCluster,
			simulateClusterDeleteError: true,
			expectedErrorsInMessage:    1,
		},
		{
			desc:                            "certificate deletions fail",
			cc:                              allResourcesCluster,
			simulateClientCACertDeleteError: true,
			simulateServingCertDeleteError:  true,
			expectedErrorsInMessage:         2,
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
			p := newMockProvider(t)
			tt.cc.p = p

			if tt.cc.cluster != nil {
				p.cnpgClient.EXPECT().DeleteCluster(ctx, tt.cc.cluster.Namespace, tt.cc.cluster.Name).Return(th.ErrIfTrue(tt.simulateClusterDeleteError))
			}

			if tt.includeStreamingReplicaUserCert {
				streamingReplicaUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
				tt.cc.streamingReplicaUserCertificate = streamingReplicaUserCert
				streamingReplicaUserCert.EXPECT().Delete(ctx).Return(th.ErrIfTrue(tt.simulateStreamingReplicaUserCertDeleteError))
			}

			if tt.includePostgresUserCert {
				postgresUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
				tt.cc.postgresUserCertificate = postgresUserCert
				postgresUserCert.EXPECT().Delete(ctx).Return(th.ErrIfTrue(tt.simulatePostgresUserCertDeleteError))
			}

			if tt.cc.clientCAIssuer != nil {
				p.cmClient.EXPECT().DeleteIssuer(ctx, tt.cc.clientCAIssuer.Namespace, tt.cc.clientCAIssuer.Name).Return(th.ErrIfTrue(tt.simulateClientCAIssuerDeleteError))
			}

			if tt.cc.clientCACertificate != nil {
				p.cmClient.EXPECT().DeleteCertificate(ctx, tt.cc.clientCACertificate.Namespace, tt.cc.clientCACertificate.Name).Return(th.ErrIfTrue(tt.simulateClientCACertDeleteError))
			}

			if tt.cc.servingCertificate != nil {
				p.cmClient.EXPECT().DeleteCertificate(ctx, tt.cc.servingCertificate.Namespace, tt.cc.servingCertificate.Name).Return(th.ErrIfTrue(tt.simulateServingCertDeleteError))
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

func TestClonedClusterSetClientCACert(t *testing.T) {
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
			cc.setClientCACert(tt.cert)
			assert.Equal(t, tt.cert, cc.clientCACertificate)
		})
	}
}

func TestClonedClusterGetClientCACert(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want *certmanagerv1.Certificate
	}{
		{
			desc: "get existing client CA certificate",
			cc: &ClonedCluster{
				clientCACertificate: &certmanagerv1.Certificate{
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
			desc: "get nil client CA certificate",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetClientCACert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetClientCAIssuer(t *testing.T) {
	tests := []struct {
		desc   string
		issuer *certmanagerv1.Issuer
	}{
		{
			desc: "set non-nil issuer",
			issuer: &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-issuer",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil issuer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setClientCAIssuer(tt.issuer)
			assert.Equal(t, tt.issuer, cc.clientCAIssuer)
		})
	}
}

func TestClonedClusterGetClientCAIssuer(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want *certmanagerv1.Issuer
	}{
		{
			desc: "get existing client CA issuer",
			cc: &ClonedCluster{
				clientCAIssuer: &certmanagerv1.Issuer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-issuer",
						Namespace: "test-ns",
					},
				},
			},
			want: &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-issuer",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil client CA issuer",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetClientCAIssuer()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetPostgresUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cert clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "set non-nil certificate",
			cert: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setPostgresUserCert(tt.cert)
			assert.Equal(t, tt.cert, cc.postgresUserCertificate)
		})
	}
}

func TestClonedClusterGetPostgresUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "get existing postgres user certificate",
			cc: &ClonedCluster{
				postgresUserCertificate: &clusterusercert.ClusterUserCert{},
			},
			want: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "get nil postgres user certificate",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetPostgresUserCert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetStreamingReplicaUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cert clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "set non-nil certificate",
			cert: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setStreamingReplicaUserCert(tt.cert)
			assert.Equal(t, tt.cert, cc.streamingReplicaUserCertificate)
		})
	}
}

func TestClonedClusterGetStreamingReplicaUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "get existing streaming replica user certificate",
			cc: &ClonedCluster{
				streamingReplicaUserCertificate: &clusterusercert.ClusterUserCert{},
			},
			want: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "get nil streaming replica user certificate",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetStreamingReplicaUserCert()
			assert.Equal(t, tt.want, got)
		})
	}
}
