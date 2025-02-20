package disasterrecovery

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewTeleport(t *testing.T) {
	mockClient := kubecluster.NewMockClientInterface(t)
	teleport := NewTeleport(mockClient)

	require.NotNil(t, teleport)
	assert.Equal(t, mockClient, teleport.kubeClusterClient)
}

func TestTeleportBackupOptions(t *testing.T) {
	th.OptStructTest[TeleportBackupOptions](t)
}

func TestTeleportBackup(t *testing.T) {
	backupName := "test-backup"
	namespace := "test-ns"
	coreClusterName := "test-core-cluster"
	auditClusterName := "test-audit-cluster"
	servingIssuerName := "serving-cert-issuer"
	clientIssuerName := "client-cert-issuer"

	tests := []struct {
		desc                                string
		backupOptions                       TeleportBackupOptions
		simulateGetCoreClusterSizeError     bool
		simulateGetAuditClusterSizeError    bool
		simulateEnsurePVCError              bool
		simulateCloneCoreClusterErr         bool
		simulateCloneCoreClusterCleanupErr  bool
		simulateCloneAuditClusterErr        bool
		simulateCloneAuditClusterCleanupErr bool
		simulateBTICreateError              bool
		simulateBTICleanupError             bool
		simulateGRPCClientErr               bool
		simulateCoreDumpAllErr              bool
		simulateAuditDumpAllErr             bool
		simulateSnapshotErr                 bool
		simulateWaitSnapErr                 bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			backupOptions: TeleportBackupOptions{
				VolumeSize:         resource.MustParse("10Gi"),
				VolumeStorageClass: "custom-storage-class",
				CloneClusterOptions: clonedcluster.CloneClusterOptions{
					CleanupTimeout: helpers.MaxWaitTime(5 * time.Second),
				},
				BackupAuditCluster:           true,
				BackupToolPodCreationTimeout: helpers.MaxWaitTime(1 * time.Second),
				BackupSnapshot: VaultWardenBackupOptionsBackupSnapshot{
					ReadyTimeout:  helpers.MaxWaitTime(2 * time.Second),
					SnapshotClass: "custom-snapshot-class",
				},
				CleanupTimeout: helpers.MaxWaitTime(3 * time.Second),
			},
		},
		{
			desc:                            "error getting core cluster size",
			simulateGetCoreClusterSizeError: true,
		},
		{
			desc:                             "error getting audit cluster size",
			simulateGetAuditClusterSizeError: true,
		},
		{
			desc:                   "error ensuring backup volume exists",
			simulateEnsurePVCError: true,
		},
		{
			desc:                        "error cloning core cluster",
			simulateCloneCoreClusterErr: true,
		},
		{
			desc:                         "error cloning audit cluster",
			backupOptions:                TeleportBackupOptions{BackupAuditCluster: true},
			simulateCloneAuditClusterErr: true,
		},
		{
			desc:                   "error creating backup tool instance",
			simulateBTICreateError: true,
		},
		{
			desc:                    "error cleaning up backup tool instance",
			simulateBTICleanupError: true,
		},
		{
			desc:                  "error creating GRPC client",
			simulateGRPCClientErr: true,
		},
		{
			desc:                   "error dumping core logical backup",
			simulateCoreDumpAllErr: true,
		},
		{
			desc:                    "error dumping audit logical backup",
			backupOptions:           TeleportBackupOptions{BackupAuditCluster: true},
			simulateAuditDumpAllErr: true,
		},
		{
			desc:                "error creating snapshot",
			simulateSnapshotErr: true,
		},
		{
			desc:                "error waiting for snapshot",
			simulateWaitSnapErr: true,
		},
		{
			desc:                               "error cleaning up core cluster",
			simulateCloneCoreClusterCleanupErr: true,
		},
		{
			desc:                                "error cleaning up audit cluster",
			backupOptions:                       TeleportBackupOptions{BackupAuditCluster: true},
			simulateCloneAuditClusterCleanupErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			drPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-pvc",
					Namespace: namespace,
				},
			}

			clonedCoreCluster := clonedcluster.NewMockClonedClusterInterface(t)
			coreServingCert := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "core-serving-cert",
					Namespace: namespace,
				},
			}
			coreUserCertificate := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "core-user-cert",
					Namespace: namespace,
				},
			}
			coreCredentials := postgres.EnvironmentCredentials{
				postgres.UserVarName: "core-postgres",
			}

			clonedAuditCluster := clonedcluster.NewMockClonedClusterInterface(t)
			auditServingCert := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "core-serving-cert",
					Namespace: namespace,
				},
			}
			auditUserCertificate := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "audit-user-cert",
					Namespace: namespace,
				},
			}
			auditCredentials := postgres.EnvironmentCredentials{
				postgres.UserVarName: "audit-postgres",
			}

			btInstance := backuptoolinstance.NewMockBackupToolInstanceInterface(t)

			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)
			mockCNPGClient := cnpg.NewMockClientInterface(t)
			mockESClient := externalsnapshotter.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()
			mockClient.EXPECT().CNPG().Return(mockCNPGClient).Maybe()
			mockClient.EXPECT().ES().Return(mockESClient).Maybe()

			mockGRPCClient := clients.NewMockClientInterface(t)
			mockPostgresRuntime := postgres.NewMockRuntime(t)
			mockGRPCClient.EXPECT().Postgres().Return(mockPostgresRuntime).Maybe()

			teleport := &Teleport{
				kubeClusterClient: mockClient,
			}

			rootCtx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateGetCoreClusterSizeError,
				tt.simulateGetAuditClusterSizeError,
				tt.simulateEnsurePVCError,
				tt.simulateCloneCoreClusterErr,
				tt.simulateCloneCoreClusterCleanupErr,
				tt.simulateCloneAuditClusterErr,
				tt.simulateCloneAuditClusterCleanupErr,
				tt.simulateBTICreateError,
				tt.simulateBTICleanupError,
				tt.simulateGRPCClientErr,
				tt.simulateCoreDumpAllErr,
				tt.simulateAuditDumpAllErr,
				tt.simulateSnapshotErr,
				tt.simulateWaitSnapErr,
			)

			// Setup mocks
			func() {
				// Get cluster sizes
				expectedPVCSize := tt.backupOptions.VolumeSize
				if tt.backupOptions.VolumeSize.IsZero() {
					mockCNPGClient.EXPECT().GetCluster(mock.Anything, namespace, coreClusterName).
						RunAndReturn(func(calledCtx *contexts.Context, namespace, coreClusterName string) (*apiv1.Cluster, error) {
							assert.True(t, calledCtx.IsChildOf(rootCtx))

							return th.ErrOr1Val(&apiv1.Cluster{
								Spec: apiv1.ClusterSpec{
									StorageConfiguration: apiv1.StorageConfiguration{
										Size: "1Gi",
									},
								},
							}, tt.simulateGetCoreClusterSizeError)
						})
					if tt.simulateGetCoreClusterSizeError {
						return
					}

					mockCNPGClient.EXPECT().GetCluster(mock.Anything, namespace, auditClusterName).
						RunAndReturn(func(calledCtx *contexts.Context, namespace, auditClusterName string) (*apiv1.Cluster, error) {
							assert.True(t, calledCtx.IsChildOf(rootCtx))

							return th.ErrOr1Val(&apiv1.Cluster{
								Spec: apiv1.ClusterSpec{
									StorageConfiguration: apiv1.StorageConfiguration{
										Size: "1Gi",
									},
								},
							}, tt.simulateGetAuditClusterSizeError)
						})
					if tt.simulateGetAuditClusterSizeError {
						return
					}

					// 2x sum of cluster allocated size
					expectedPVCSize = resource.MustParse("4Gi")
				}

				// Ensure PVC exists
				mockCoreClient.EXPECT().EnsurePVCExists(mock.Anything, namespace, backupName, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.True(t, expectedPVCSize.Equal(size))

						return th.ErrOr1Val(drPVC, tt.simulateEnsurePVCError)
					})
				if tt.simulateEnsurePVCError {
					return
				}

				// Clone core cluster
				mockClient.EXPECT().CloneCluster(mock.Anything, namespace, coreClusterName, mock.Anything, servingIssuerName, clientIssuerName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, coreClusterName, newClusterName, servingIssuerName, clientIssuerName string, opts clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.True(t, strings.Contains(newClusterName, "core"))
						assert.True(t, strings.Contains(newClusterName, helpers.CleanName(backupName)))
						assert.LessOrEqual(t, len(newClusterName), 50)

						return th.ErrOr1Val(clonedCoreCluster, tt.simulateCloneCoreClusterErr)
					})
				if tt.simulateCloneCoreClusterErr {
					return
				}
				clonedCoreCluster.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
					assert.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateCloneCoreClusterCleanupErr)
				})

				// Clone audit cluster if enabled
				if tt.backupOptions.BackupAuditCluster {
					mockClient.EXPECT().CloneCluster(mock.Anything, namespace, auditClusterName, mock.Anything, servingIssuerName, clientIssuerName, mock.Anything).
						RunAndReturn(func(calledCtx *contexts.Context, namespace, auditClusterName, newClusterName, servingIssuerName, clientIssuerName string, opts clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error) {
							assert.True(t, calledCtx.IsChildOf(rootCtx))
							assert.True(t, strings.Contains(newClusterName, "audit"))
							assert.LessOrEqual(t, len(newClusterName), 50)

							return th.ErrOr1Val(clonedAuditCluster, tt.simulateCloneAuditClusterErr)
						})
					if tt.simulateCloneAuditClusterErr {
						return
					}
					clonedAuditCluster.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
						assert.NotEqual(t, rootCtx, cleanupCtx)
						return th.ErrIfTrue(tt.simulateCloneAuditClusterCleanupErr)
					})
				}

				// Create backup tool instance
				clonedCoreCluster.EXPECT().GetServingCert().Return(&coreServingCert)
				coreClusterUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
				coreClusterUserCert.EXPECT().GetCertificate().Return(&coreUserCertificate)
				clonedCoreCluster.EXPECT().GetPostgresUserCert().Return(coreClusterUserCert)
				if tt.backupOptions.BackupAuditCluster {
					clonedAuditCluster.EXPECT().GetServingCert().Return(&auditServingCert)
					auditClusterUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
					auditClusterUserCert.EXPECT().GetCertificate().Return(&auditUserCertificate)
					clonedAuditCluster.EXPECT().GetPostgresUserCert().Return(auditClusterUserCert)
				}

				mockClient.EXPECT().CreateBackupToolInstance(mock.Anything, namespace, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, instance string, opts backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Contains(t, instance, backupName)
						assert.Contains(t, opts.NamePrefix, constants.ToolName)
						// TODO add test to ensure that the secrets specifically are attached, as well as the DR volume
						expectedVolCount := 3
						if tt.backupOptions.BackupAuditCluster {
							expectedVolCount += 2
						}
						assert.Len(t, opts.Volumes, expectedVolCount)

						return th.ErrOr1Val(btInstance, tt.simulateBTICreateError)
					})
				if tt.simulateBTICreateError {
					return
				}

				btInstance.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
					require.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateBTICleanupError)
				})
				if tt.simulateBTICleanupError {
					btInstance.EXPECT().Delete(mock.Anything).Return(assert.AnError)
				}

				// Get GRPC client
				btInstance.EXPECT().GetGRPCClient(mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, searchDomains ...string) (clients.ClientInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Equal(t, tt.backupOptions.ClusterServiceSearchDomains, searchDomains)
						return th.ErrOr1Val(mockGRPCClient, tt.simulateGRPCClientErr)
					})
				if tt.simulateGRPCClientErr {
					return
				}

				// Core cluster dump
				var coreServingCertMountDirectory string
				var coreClientCertMountDirectory string
				clonedCoreCluster.EXPECT().GetCredentials(mock.Anything, mock.Anything).
					RunAndReturn(func(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials {
						assert.NotEqual(t, servingCertMountDirectory, clientCertMountDirectory)
						assert.True(t, strings.HasPrefix(servingCertMountDirectory, teleportBaseMountPath))
						assert.True(t, strings.HasPrefix(clientCertMountDirectory, teleportBaseMountPath))

						coreServingCertMountDirectory = servingCertMountDirectory
						coreClientCertMountDirectory = clientCertMountDirectory

						return coreCredentials
					})

				mockPostgresRuntime.EXPECT().DumpAll(mock.Anything, coreCredentials, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, creds postgres.Credentials, outputFilePath string, opts postgres.DumpAllOptions) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.True(t, filepath.Base(outputFilePath) == "backup-core.sql") // Important: changing this is will break restoration of old backups!
						return th.ErrIfTrue(tt.simulateCoreDumpAllErr)
					})
				if tt.simulateCoreDumpAllErr {
					return
				}

				// Audit cluster dump if enabled
				if tt.backupOptions.BackupAuditCluster {
					clonedAuditCluster.EXPECT().GetCredentials(mock.Anything, mock.Anything).
						RunAndReturn(func(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials {
							assert.NotEqual(t, servingCertMountDirectory, clientCertMountDirectory)
							assert.True(t, strings.HasPrefix(servingCertMountDirectory, teleportBaseMountPath))
							assert.True(t, strings.HasPrefix(clientCertMountDirectory, teleportBaseMountPath))

							assert.NotEqual(t, coreServingCertMountDirectory, servingCertMountDirectory)
							assert.NotEqual(t, coreClientCertMountDirectory, clientCertMountDirectory)

							return auditCredentials
						})
					mockPostgresRuntime.EXPECT().DumpAll(mock.Anything, auditCredentials, mock.Anything, mock.Anything).
						RunAndReturn(func(calledCtx *contexts.Context, creds postgres.Credentials, outputFilePath string, opts postgres.DumpAllOptions) error {
							assert.True(t, calledCtx.IsChildOf(rootCtx))
							assert.True(t, filepath.Base(outputFilePath) == "backup-audit.sql") // Important: changing this is will break restoration of old backups!
							return th.ErrIfTrue(tt.simulateAuditDumpAllErr)
						})
					if tt.simulateAuditDumpAllErr {
						return
					}
				}

				// Snapshot volume
				var createdSnapshotName string
				mockESClient.EXPECT().SnapshotVolume(mock.Anything, namespace, drPVC.Name, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, opts externalsnapshotter.SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Contains(t, opts.Name, helpers.CleanName(backupName))
						assert.NotEqual(t, opts.Name, helpers.CleanName(backupName))
						assert.Equal(t, tt.backupOptions.BackupSnapshot.SnapshotClass, opts.SnapshotClass)

						createdSnapshotName = opts.Name

						return th.ErrOr1Val(&volumesnapshotv1.VolumeSnapshot{
							ObjectMeta: metav1.ObjectMeta{
								Name:      opts.Name,
								Namespace: namespace,
							},
						}, tt.simulateSnapshotErr)
					})
				if tt.simulateSnapshotErr {
					return
				}

				// Wait for snapshot
				mockESClient.EXPECT().WaitForReadySnapshot(mock.Anything, namespace, mock.Anything, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: tt.backupOptions.BackupSnapshot.ReadyTimeout}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, snapsnotName string, wfrso externalsnapshotter.WaitForReadySnapshotOpts) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Equal(t, createdSnapshotName, snapsnotName)

						return th.ErrOr1Val(&volumesnapshotv1.VolumeSnapshot{}, tt.simulateWaitSnapErr)
					})
			}()

			backup, err := teleport.Backup(rootCtx, namespace, backupName, coreClusterName,
				auditClusterName, servingIssuerName, clientIssuerName, tt.backupOptions)

			require.NotNil(t, backup)
			assert.NotEmpty(t, backup.StartTime)
			assert.NotEmpty(t, backup.EndTime)

			if wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
