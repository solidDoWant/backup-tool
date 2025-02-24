package backup

import (
	"path/filepath"
	"strings"
	"testing"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCNPGBackupOptions(t *testing.T) {
	th.OptStructTest[CNPGBackupOptions](t)
}

func TestConfigure(t *testing.T) {
	expectedState := &configureState{
		kubeClusterClient:     kubecluster.NewMockClientInterface(t),
		namespace:             "namespace",
		drVolName:             "drVolName",
		backupFileRelPath:     "backupFileRelPath",
		clusterName:           "clusterName",
		servingCertIssuerName: "servingCertIssuerName",
		clientCertIssuerName:  "clientCertIssuerName",
		opts: CNPGBackupOptions{
			CloningOpts: clonedcluster.CloneClusterOptions{
				Certificates: clonedcluster.CloneClusterOptionsCertificates{
					ServingCert: clonedcluster.CloneClusterOptionsExternallyIssuedCertificate{
						IssuerKind: "ClusterIssuer",
					},
				},
			},
			CleanupTimeout: helpers.ShortWaitTime,
		},
	}

	cnpgb := NewCNPGBackup()
	err := cnpgb.Configure(
		expectedState.kubeClusterClient,
		expectedState.namespace,
		expectedState.clusterName,
		expectedState.servingCertIssuerName,
		expectedState.clientCertIssuerName,
		expectedState.drVolName,
		expectedState.backupFileRelPath,
		expectedState.opts,
	)

	t.Run("successfully configures the first time", func(t *testing.T) {
		require.NoError(t, err)
	})

	t.Run("all state vars are populated", func(t *testing.T) {
		casted := cnpgb.(*CNPGBackup)

		assert.NotEqual(t, "", casted.uid)
		assert.NotEqual(t, uuid.Nil.String(), casted.uid)
		expectedState.uid = casted.uid

		assert.True(t, casted.isConfigured)
		expectedState.isConfigured = casted.isConfigured

		assert.Equal(t, expectedState, &casted.configureState)
	})

	t.Run("fails to configure because already configured", func(t *testing.T) {
		err = cnpgb.Configure(
			expectedState.kubeClusterClient,
			expectedState.namespace,
			expectedState.clusterName,
			expectedState.servingCertIssuerName,
			expectedState.clientCertIssuerName,
			expectedState.drVolName,
			expectedState.backupFileRelPath,
			expectedState.opts,
		)
		assert.Error(t, err)
	})
}

func TestValidate(t *testing.T) {
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

	readyIssuer := &certmanagerv1.Issuer{
		Status: certmanagerv1.IssuerStatus{
			Conditions: []certmanagerv1.IssuerCondition{
				{
					Type:   certmanagerv1.IssuerConditionReady,
					Status: cmmeta.ConditionTrue,
				},
			},
		},
	}
	notReadyIssuer := readyIssuer.DeepCopy()
	notReadyIssuer.Status.Conditions[0].Status = cmmeta.ConditionFalse

	notConfiguredState := &configureState{}
	configuredState := &configureState{}
	err := configuredState.Configure(
		nil,
		"namespace",
		"clusterName",
		"servingCertIssuerName",
		"clientCertIssuerName",
		"drVolName",
		"backupFileRelPath",
		CNPGBackupOptions{},
	)
	require.NoError(t, err)

	tests := []struct {
		desc                            string
		configState                     *configureState
		isAlreadyValidated              bool
		simulateGetClusterError         bool
		returnClusterNotReady           bool
		simulateGetServingCertIssuer    bool
		returnServingCertIssuerNotReady bool
		simulateGetClientCertIssuer     bool
		returnClientCertIssuerNotReady  bool
		simulateGetPVCErr               bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:               "succeeds if called multiple times",
			isAlreadyValidated: true,
		},
		{
			desc:        "fails because not configured",
			configState: notConfiguredState,
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
			desc:                         "fails to get serving cert issuer",
			simulateGetServingCertIssuer: true,
		},
		{
			desc:                            "fails because serving cert issuer is not ready",
			returnServingCertIssuerNotReady: true,
		},
		{
			desc:                        "fails to get client cert issuer",
			simulateGetClientCertIssuer: true,
		},
		{
			desc:                           "fails because client cert issuer is not ready",
			returnClientCertIssuerNotReady: true,
		},
		{
			desc:              "fails to get DR PVC",
			simulateGetPVCErr: true,
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

			if tt.configState == nil {
				tt.configState = configuredState
			}

			currentState := &validateState{
				configureState: *tt.configState,
				isValidated:    tt.isAlreadyValidated,
			}
			currentState.kubeClusterClient = mockClient

			ctx := th.NewTestContext()

			wantErr := th.ErrExpected(
				!currentState.isConfigured,
				tt.simulateGetClusterError,
				tt.returnClusterNotReady,
				tt.simulateGetServingCertIssuer,
				tt.returnServingCertIssuerNotReady,
				tt.simulateGetClientCertIssuer,
				tt.returnClientCertIssuerNotReady,
				tt.simulateGetPVCErr,
			)

			func() {
				if !currentState.isConfigured {
					return
				}

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

				mockCMClient.EXPECT().GetIssuer(mock.Anything, currentState.namespace, currentState.servingCertIssuerName).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*certmanagerv1.Issuer, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						retIssuer := readyIssuer
						if tt.returnServingCertIssuerNotReady {
							retIssuer = notReadyIssuer
						}

						return th.ErrOr1Val(retIssuer, tt.simulateGetServingCertIssuer)
					})
				if tt.simulateGetServingCertIssuer || tt.returnServingCertIssuerNotReady {
					return
				}

				mockCMClient.EXPECT().GetIssuer(mock.Anything, currentState.namespace, currentState.clientCertIssuerName).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*certmanagerv1.Issuer, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						retIssuer := readyIssuer
						if tt.returnClientCertIssuerNotReady {
							retIssuer = notReadyIssuer
						}

						return th.ErrOr1Val(retIssuer, tt.simulateGetClientCertIssuer)
					})
				if tt.simulateGetClientCertIssuer || tt.returnClientCertIssuerNotReady {
					return
				}

				mockCoreClient.EXPECT().GetPVC(mock.Anything, currentState.namespace, currentState.drVolName).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						return nil, th.ErrIfTrue(tt.simulateGetPVCErr)
					})
			}()

			err := currentState.Validate(ctx)
			if wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			expectedState := *currentState
			expectedState.cluster = readyCluster
			assert.Equal(t, &expectedState, currentState)
		})
	}
}

func TestSetup(t *testing.T) {
	tests := []struct {
		desc                      string
		hasBeenNotBeenValidated   bool
		isAlreadySetup            bool
		simulateCloneClusterError bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:                    "fails because not validated first",
			hasBeenNotBeenValidated: true,
		},
		{
			desc:           "fails if called multiple times",
			isAlreadySetup: true,
		},
		{
			desc:                      "fails to clone cluster",
			simulateCloneClusterError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockCloneCluster := clonedcluster.NewMockClonedClusterInterface(t)
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCUC := clusterusercert.NewMockClusterUserCertInterface(t)

			servingCert := certmanagerv1.Certificate{
				Spec: certmanagerv1.CertificateSpec{
					SecretName: "serving-cert-secret",
				},
			}

			clientCert := certmanagerv1.Certificate{
				Spec: certmanagerv1.CertificateSpec{
					SecretName: "client-cert-secret",
				},
			}

			currentState := &setupState{
				validateState: validateState{
					configureState: configureState{
						uid:                   "uid",
						isConfigured:          true,
						kubeClusterClient:     mockClient,
						namespace:             "namespace",
						clusterName:           "clusterName",
						servingCertIssuerName: "servingCertIssuerName",
						clientCertIssuerName:  "clientCertIssuerName",
						drVolName:             "drVolName",
						backupFileRelPath:     "backupFileRelPath",
						opts: CNPGBackupOptions{
							CloningOpts: clonedcluster.CloneClusterOptions{
								Certificates: clonedcluster.CloneClusterOptionsCertificates{
									ServingCert: clonedcluster.CloneClusterOptionsExternallyIssuedCertificate{
										IssuerKind: "ClusterIssuer",
									},
								},
							},
							CleanupTimeout: helpers.ShortWaitTime,
						},
					},
					isValidated: !tt.hasBeenNotBeenValidated,
					cluster:     &apiv1.Cluster{},
				},
				isSetup: tt.isAlreadySetup,
			}

			ctx := th.NewTestContext()

			func() {
				if tt.hasBeenNotBeenValidated || currentState.isSetup {
					return
				}

				mockClient.EXPECT().CloneCluster(mock.Anything, currentState.namespace, currentState.clusterName, mock.Anything, currentState.servingCertIssuerName, currentState.clientCertIssuerName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, existingClusterName, newClusterName, servingCertIssuerNAme, clientCertIssuerName string, opts clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						assert.Contains(t, newClusterName, currentState.uid)
						assert.LessOrEqual(t, len(newClusterName), 50)

						if currentState.opts.CloningOpts.CleanupTimeout == 0 {
							assert.Equal(t, currentState.opts.CleanupTimeout, opts.CleanupTimeout)
							opts.CleanupTimeout = currentState.opts.CleanupTimeout
						}
						assert.Equal(t, currentState.opts.CloningOpts, opts)

						return th.ErrOr1Val(mockCloneCluster, tt.simulateCloneClusterError)
					})
				if tt.simulateCloneClusterError {
					return
				}

				mockCloneCluster.EXPECT().GetServingCert().Return(&servingCert)
				mockCUC.EXPECT().GetCertificate().Return(&clientCert)
				mockCloneCluster.EXPECT().GetPostgresUserCert().Return(mockCUC)
			}()

			btiOpts := &backuptoolinstance.CreateBackupToolInstanceOptions{}
			err := currentState.Setup(ctx, btiOpts)
			if th.ErrExpected(tt.hasBeenNotBeenValidated, tt.isAlreadySetup, tt.simulateCloneClusterError) {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Contains(t, currentState.mountPaths.drVolume, currentState.uid)
			assert.Contains(t, currentState.mountPaths.servingCert, currentState.uid)
			assert.Contains(t, currentState.mountPaths.clientCert, currentState.uid)

			assert.Len(t, btiOpts.Volumes, 3)

			// DR vol
			drVols := lo.Filter(btiOpts.Volumes, func(v core.SingleContainerVolume, _ int) bool {
				return strings.HasPrefix(v.Name, currentState.drVolName)
			})
			require.Len(t, drVols, 1)
			assert.Equal(t, []string{currentState.mountPaths.drVolume}, drVols[0].MountPaths)
			require.NotNil(t, drVols[0].VolumeSource.PersistentVolumeClaim)
			assert.Equal(t, currentState.drVolName, drVols[0].VolumeSource.PersistentVolumeClaim.ClaimName)

			// Serving cert vol
			servingCertVols := lo.Filter(btiOpts.Volumes, func(v core.SingleContainerVolume, _ int) bool {
				return strings.HasPrefix(v.Name, servingCert.Spec.SecretName)
			})
			require.Len(t, servingCertVols, 1)
			assert.Equal(t, []string{currentState.mountPaths.servingCert}, servingCertVols[0].MountPaths)
			require.NotNil(t, servingCertVols[0].VolumeSource.Secret)
			assert.Equal(t, servingCert.Spec.SecretName, servingCertVols[0].VolumeSource.Secret.SecretName)
			assert.Len(t, servingCertVols[0].VolumeSource.Secret.Items, 1)
			assert.Equal(t, "tls.crt", servingCertVols[0].VolumeSource.Secret.Items[0].Key) // Verify that the private key was not mounted

			// Client cert vol
			clientCertVols := lo.Filter(btiOpts.Volumes, func(v core.SingleContainerVolume, _ int) bool {
				return strings.HasPrefix(v.Name, clientCert.Spec.SecretName)
			})
			require.Len(t, clientCertVols, 1)
			assert.Equal(t, []string{currentState.mountPaths.clientCert}, clientCertVols[0].MountPaths)
			require.NotNil(t, clientCertVols[0].VolumeSource.Secret)
			assert.Equal(t, clientCert.Spec.SecretName, clientCertVols[0].VolumeSource.Secret.SecretName)
		})
	}
}

func TestCleanup(t *testing.T) {
	tests := []struct {
		desc                           string
		hasNotBeenSetup                bool
		simulateClonedClusterDeleteErr bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:            "succeeds and does nothing if not setup first",
			hasNotBeenSetup: true,
		},
		{
			desc:                           "fails to cleanup cloned cluster",
			simulateClonedClusterDeleteErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCloneCluster := clonedcluster.NewMockClonedClusterInterface(t)

			currentState := &setupState{
				validateState: validateState{
					configureState: configureState{
						uid:                   "uid",
						isConfigured:          true,
						kubeClusterClient:     mockClient,
						namespace:             "namespace",
						clusterName:           "clusterName",
						servingCertIssuerName: "servingCertIssuerName",
						clientCertIssuerName:  "clientCertIssuerName",
						drVolName:             "drVolName",
						backupFileRelPath:     "backupFileRelPath",
						opts: CNPGBackupOptions{
							CloningOpts: clonedcluster.CloneClusterOptions{
								Certificates: clonedcluster.CloneClusterOptionsCertificates{
									ServingCert: clonedcluster.CloneClusterOptionsExternallyIssuedCertificate{
										IssuerKind: "ClusterIssuer",
									},
								},
							},
							CleanupTimeout: helpers.ShortWaitTime,
						},
					},
					isValidated: true,
					cluster: &apiv1.Cluster{
						Status: apiv1.ClusterStatus{
							WriteService: "write-service",
						},
					},
				},
				clonedCluster: mockCloneCluster,
				isSetup:       !tt.hasNotBeenSetup,
			}

			if !tt.hasNotBeenSetup {
				mockCloneCluster.EXPECT().Delete(mock.Anything).Return(th.ErrIfTrue(tt.simulateClonedClusterDeleteErr))
			}

			err := currentState.Cleanup(th.NewTestContext())
			if tt.simulateClonedClusterDeleteErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		desc                   string
		hasNotBeenSetup        bool
		simulateClusterUserErr bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:            "fails if not setup first",
			hasNotBeenSetup: true,
		},
		{
			desc:                   "fails to execute cluster user cert",
			simulateClusterUserErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCloneCluster := clonedcluster.NewMockClonedClusterInterface(t)
			mockPGR := postgres.NewMockRuntime(t)
			mockGRPC := clients.NewMockClientInterface(t)
			mockGRPC.EXPECT().Postgres().Return(mockPGR).Maybe()

			currentState := &executeState{
				setupState: setupState{
					validateState: validateState{
						configureState: configureState{
							uid:                   "uid",
							isConfigured:          true,
							kubeClusterClient:     mockClient,
							namespace:             "namespace",
							clusterName:           "clusterName",
							servingCertIssuerName: "servingCertIssuerName",
							clientCertIssuerName:  "clientCertIssuerName",
							drVolName:             "drVolName",
							backupFileRelPath:     "backupFileRelPath",
							opts: CNPGBackupOptions{
								CloningOpts: clonedcluster.CloneClusterOptions{
									Certificates: clonedcluster.CloneClusterOptionsCertificates{
										ServingCert: clonedcluster.CloneClusterOptionsExternallyIssuedCertificate{
											IssuerKind: "ClusterIssuer",
										},
									},
								},
								CleanupTimeout: helpers.ShortWaitTime,
							},
						},
						isValidated: true,
						cluster: &apiv1.Cluster{
							Status: apiv1.ClusterStatus{
								WriteService: "write-service",
							},
						},
					},
					mountPaths: setupStateMountPaths{
						drVolume:    "/dr-volume",
						servingCert: "/serving-cert",
						clientCert:  "/client-cert",
					},
					clonedCluster: mockCloneCluster,
					isSetup:       !tt.hasNotBeenSetup,
				},
			}

			credentials := &postgres.EnvironmentCredentials{
				postgres.HostVarName: "write-service.namespace.svc",
				postgres.UserVarName: "postgres",
			}

			ctx := th.NewTestContext()
			if currentState.isSetup {
				drFilePath := filepath.Join(currentState.mountPaths.drVolume, currentState.backupFileRelPath) // Important: Changing this is a breaking change!

				mockCloneCluster.EXPECT().GetCredentials(currentState.mountPaths.servingCert, currentState.mountPaths.clientCert).Return(credentials)

				mockPGR.EXPECT().DumpAll(mock.Anything, credentials, drFilePath, postgres.DumpAllOptions{CleanupTimeout: currentState.opts.CleanupTimeout}).
					RunAndReturn(func(calledCtx *contexts.Context, credentials postgres.Credentials, backupFilePath string, opts postgres.DumpAllOptions) error {
						assert.True(t, calledCtx.IsChildOf(ctx))

						return th.ErrIfTrue(tt.simulateClusterUserErr)
					})
			}

			err := currentState.Execute(ctx, mockGRPC)
			if tt.hasNotBeenSetup || tt.simulateClusterUserErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCNPGBackup(t *testing.T) {
	assert.Implements(t, (*CNPGBackupInterface)(nil), (*CNPGBackup)(nil))
	assert.Implements(t, (*remote.RemoteAction)(nil), (*CNPGBackup)(nil))
}

func TestNewCNPGRestore(t *testing.T) {
	// State vars should not be populated yet
	assert.Equal(t, &CNPGBackup{}, NewCNPGBackup())
}
