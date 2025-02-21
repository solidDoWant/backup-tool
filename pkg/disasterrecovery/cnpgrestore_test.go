package disasterrecovery

import (
	"testing"

	certmanagerV1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCNPGRestoreOpts(t *testing.T) {
	th.OptStructTest[CNPGRestoreOpts](t)
}

func TestNewCNPGRestore(t *testing.T) {
	// State vars should not be populated yet
	assert.Equal(t, &CNPGRestore{}, NewCNPGRestore())
}

func TestCNPGRestoreConfigure(t *testing.T) {
	expectedState := &CNPGRestore{
		kubeClusterClient:    kubecluster.NewMockClientInterface(t),
		namespace:            "namespace",
		clusterName:          "clusterName",
		servingCertName:      "servingCertName",
		clientCertIssuerName: "clientCertIssuerName",
		drVolName:            "drVolName",
		fullRestoreName:      "fullRestoreName",
		backupFileRelPath:    "backupFileRelPath",
		opts: CNPGRestoreOpts{
			PostgresUserCert: OptionsClusterUserCert{
				Subject: &certmanagerV1.X509Subject{
					Organizations: []string{"test-org"},
				},
				WaitForReadyTimeout: helpers.ShortWaitTime,
				CRPOpts: clusterusercert.NewClusterUserCertOptsCRP{
					Enabled: true,
				},
			},
			RemoteBackupToolOptions: backuptoolinstance.CreateBackupToolInstanceOptions{
				PodWaitTimeout: 2 * helpers.ShortWaitTime,
			},
			CleanupTimeout: 3 * helpers.ShortWaitTime,
		},
	}

	cnpgr := NewCNPGRestore()
	cnpgr.Configure(
		expectedState.kubeClusterClient,
		expectedState.namespace,
		expectedState.clusterName,
		expectedState.servingCertName,
		expectedState.clientCertIssuerName,
		expectedState.drVolName,
		expectedState.fullRestoreName,
		expectedState.backupFileRelPath,
		expectedState.opts,
	)

	assert.Equal(t, expectedState, cnpgr)
}

func TestCNPGRestoreCheckResourcesReady(t *testing.T) {
	readyCluster := &apiv1.Cluster{
		Status: apiv1.ClusterStatus{
			Conditions: []metav1.Condition{
				{
					Type:   string(apiv1.ConditionClusterReady),
					Status: metav1.ConditionTrue,
				},
			},
		},
	}
	notReadyCluster := readyCluster.DeepCopy()
	notReadyCluster.Status.Conditions[0].Status = metav1.ConditionFalse

	cert := &certmanagerV1.Certificate{}

	readyIssuer := &certmanagerV1.Issuer{
		Status: certmanagerV1.IssuerStatus{
			Conditions: []certmanagerV1.IssuerCondition{
				{
					Type:   certmanagerV1.IssuerConditionReady,
					Status: cmmeta.ConditionTrue,
				},
			},
		},
	}
	notReadyIssuer := readyIssuer.DeepCopy()
	notReadyIssuer.Status.Conditions[0].Status = cmmeta.ConditionFalse

	tests := []struct {
		desc                           string
		simulateGetClusterError        bool
		returnClusterNotReady          bool
		simulateGetServingCert         bool
		simulateGetClientCertIssuer    bool
		returnClientCertIssuerNotReady bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:                    "fails to get cluster",
			simulateGetClusterError: true,
		},
		{
			desc:                  "fails because cluster is not ready",
			returnClusterNotReady: true,
		},
		{
			desc:                   "fails to get serving cert",
			simulateGetServingCert: true,
		},
		{
			desc:                        "fails to get client cert issuer",
			simulateGetClientCertIssuer: true,
		},
		{
			desc:                           "fails because client cert issuer is not ready",
			returnClientCertIssuerNotReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)
			mockCMClient := certmanager.NewMockClientInterface(t)
			mockCNPGClient := cnpg.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()
			mockClient.EXPECT().CM().Return(mockCMClient).Maybe()
			mockClient.EXPECT().CNPG().Return(mockCNPGClient).Maybe()

			currentState := &CNPGRestore{
				kubeClusterClient:    mockClient,
				namespace:            "namespace",
				clusterName:          "clusterName",
				servingCertName:      "servingCertName",
				clientCertIssuerName: "clientCertIssuerName",
				drVolName:            "drVolName",
				fullRestoreName:      "fullRestoreName",
				backupFileRelPath:    "backupFileRelPath",
			}

			ctx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateGetClusterError,
				tt.returnClusterNotReady,
				tt.simulateGetServingCert,
				tt.simulateGetClientCertIssuer,
				tt.returnClientCertIssuerNotReady,
			)

			func() {
				mockCNPGClient.EXPECT().GetCluster(mock.Anything, currentState.namespace, currentState.clusterName).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*apiv1.Cluster, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						retCluster := readyCluster
						if tt.returnClusterNotReady {
							retCluster = notReadyCluster
						}

						return th.ErrOr1Val(retCluster, tt.simulateGetClusterError)
					})
				if tt.simulateGetClusterError || tt.returnClusterNotReady {
					return
				}

				mockCMClient.EXPECT().GetCertificate(mock.Anything, currentState.namespace, currentState.servingCertName).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*certmanagerV1.Certificate, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						return th.ErrOr1Val(cert, tt.simulateGetServingCert)
					})
				if tt.simulateGetServingCert {
					return
				}

				mockCMClient.EXPECT().GetIssuer(mock.Anything, currentState.namespace, currentState.clientCertIssuerName).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*certmanagerV1.Issuer, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						retIssuer := readyIssuer
						if tt.returnClientCertIssuerNotReady {
							retIssuer = notReadyIssuer
						}

						return th.ErrOr1Val(retIssuer, tt.simulateGetClientCertIssuer)
					})
			}()

			err := currentState.CheckResourcesReady(ctx)
			if wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			expectedState := *currentState
			expectedState.cluster = readyCluster
			expectedState.servingCert = cert
			expectedState.clientCertIssuer = readyIssuer
			assert.Equal(t, &expectedState, currentState)
		})
	}
}

func TestCNPGRestoreRestore(t *testing.T) {
	tests := []struct {
		desc                                string
		opts                                CNPGRestoreOpts
		simulateNewClusterUserCertError     bool
		simulateClusterUserCertCleanupError bool
		simulateCreateBTIError              bool
		simulateBTICleanupError             bool
		simulateGetGRPCClientError          bool
		simulatePostgresRestoreError        bool
	}{
		{
			desc: "succeeds",
			opts: CNPGRestoreOpts{
				PostgresUserCert: OptionsClusterUserCert{
					Subject: &certmanagerV1.X509Subject{
						Organizations: []string{"test-org"},
					},
					WaitForReadyTimeout: helpers.ShortWaitTime,
					CRPOpts: clusterusercert.NewClusterUserCertOptsCRP{
						Enabled: true,
					},
				},
				RemoteBackupToolOptions: backuptoolinstance.CreateBackupToolInstanceOptions{
					PodWaitTimeout: 2 * helpers.ShortWaitTime,
				},
				CleanupTimeout:              3 * helpers.ShortWaitTime,
				ClusterServiceSearchDomains: []string{"cluster.local"},
			},
		},
		{
			desc:                            "fails to create postgres user cert",
			simulateNewClusterUserCertError: true,
		},
		{
			desc:                                "fails to cleanup postgres user cert",
			simulateClusterUserCertCleanupError: true,
		},
		{
			desc:                   "fails to create backup-tool instance",
			simulateCreateBTIError: true,
		},
		{
			desc:                    "fails to cleanup backup-tool instance",
			simulateBTICleanupError: true,
		},
		{
			desc:                       "fails to get grpc client",
			simulateGetGRPCClientError: true,
		},
		{
			desc:                         "fails to restore postgres",
			simulatePostgresRestoreError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClusterUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
			mockClient := kubecluster.NewMockClientInterface(t)
			mockBackupToolInstance := backuptoolinstance.NewMockBackupToolInstanceInterface(t)
			mockGRPCClient := clients.NewMockClientInterface(t)
			mockPostgresClient := postgres.NewMockRuntime(t)

			currentState := &CNPGRestore{
				kubeClusterClient:    mockClient,
				namespace:            "namespace",
				clusterName:          "cluster",
				fullRestoreName:      "restore-name",
				backupFileRelPath:    "backup/path",
				clientCertIssuerName: "cert-issuer",
				drVolName:            "dr-vol",
				opts:                 tt.opts,
				cluster: &apiv1.Cluster{
					Status: apiv1.ClusterStatus{
						WriteService: "write-service",
					},
				},
				servingCert: &certmanagerV1.Certificate{
					Spec: certmanagerV1.CertificateSpec{
						SecretName: "serving-cert-secret",
					},
				},
			}

			ctx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateNewClusterUserCertError,
				tt.simulateClusterUserCertCleanupError,
				tt.simulateCreateBTIError,
				tt.simulateBTICleanupError,
				tt.simulateGetGRPCClientError,
				tt.simulatePostgresRestoreError,
			)

			func() {
				// 1. Create postgres user certs for the cluster
				mockClient.EXPECT().NewClusterUserCert(mock.Anything, currentState.namespace, "postgres", currentState.clientCertIssuerName, currentState.clusterName, clusterusercert.NewClusterUserCertOpts{
					Subject:            tt.opts.PostgresUserCert.Subject,
					CRPOpts:            tt.opts.PostgresUserCert.CRPOpts,
					WaitForCertTimeout: tt.opts.PostgresUserCert.WaitForReadyTimeout,
					CleanupTimeout:     tt.opts.CleanupTimeout,
				}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, username, issuerName, clusterName string, opts clusterusercert.NewClusterUserCertOpts) (clusterusercert.ClusterUserCertInterface, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						return th.ErrOr1Val(mockClusterUserCert, tt.simulateNewClusterUserCertError)
					})
				if tt.simulateNewClusterUserCertError {
					return
				}

				mockClusterUserCert.EXPECT().Delete(mock.Anything).Return(th.ErrIfTrue(tt.simulateClusterUserCertCleanupError))

				// 2. Spawn a new backup-tool pod with postgres auth and serving certs, and DR mounts attached
				mockClusterUserCert.EXPECT().GetCertificate().Return(&certmanagerV1.Certificate{
					ObjectMeta: metav1.ObjectMeta{Name: "postgres-client-cert"},
					Spec: certmanagerV1.CertificateSpec{
						SecretName: "postgres-client-cert-secret",
					},
				})

				mockClient.EXPECT().CreateBackupToolInstance(mock.Anything, currentState.namespace, currentState.fullRestoreName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, opts backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						// TODO pull more checks from VW restore

						return th.ErrOr1Val(mockBackupToolInstance, tt.simulateCreateBTIError)
					})
				if tt.simulateCreateBTIError {
					return
				}

				mockBackupToolInstance.EXPECT().Delete(mock.Anything).Return(th.ErrIfTrue(tt.simulateBTICleanupError))

				mockBackupToolInstance.EXPECT().GetGRPCClient(mock.Anything, mock.Anything).RunAndReturn(func(calledCtx *contexts.Context, searchDomains ...string) (clients.ClientInterface, error) {
					assert.True(t, calledCtx.IsChildOf(ctx))
					assert.Equal(t, tt.opts.ClusterServiceSearchDomains, searchDomains)

					return th.ErrOr1Val(mockGRPCClient, tt.simulateGetGRPCClientError)
				})
				if tt.simulateGetGRPCClientError {
					return
				}

				mockGRPCClient.EXPECT().Postgres().Return(mockPostgresClient)
				mockPostgresClient.EXPECT().Restore(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, creds postgres.Credentials, filePath string, opts postgres.RestoreOptions) error {
						assert.True(t, calledCtx.IsChildOf(ctx))

						return th.ErrIfTrue(tt.simulatePostgresRestoreError)
					})
			}()

			err := currentState.Restore(ctx)
			if wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
