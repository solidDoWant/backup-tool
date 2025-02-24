package disasterrecovery

import (
	"strings"
	"testing"
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	cnpgbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/backup"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/solidDoWant/backup-tool/pkg/s3"
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
	assert.NotNil(t, teleport.newCNPGRestore)
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
	auditSessionLogsS3Path := "s3://audit-session-logs"
	auditSessionLogsS3Credentials := s3.NewCredentials("accessKeyID", "secretAccessKey")

	tests := []struct {
		desc                                         string
		opts                                         TeleportBackupOptions
		simulateGetCoreClusterSizeError              bool
		simulateGetAuditClusterSizeError             bool
		simulateEnsurePVCError                       bool
		simulateConfigureCoreBackupError             bool
		simulateConfigureAuditBackupError            bool
		simulateConfigureAuditSessionLogsBackupError bool
		simulateRunError                             bool
		simulateSnapshotError                        bool
		simulateWaitSnapError                        bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			opts: TeleportBackupOptions{
				VolumeSize:         resource.MustParse("10Gi"),
				VolumeStorageClass: "custom-storage-class",
				CloneClusterOptions: clonedcluster.CloneClusterOptions{
					CleanupTimeout: helpers.MaxWaitTime(5 * time.Second),
				},
				AuditCluster: TeleportBackupOptionsAudit{
					TeleportOptionsAudit{
						Name:    auditClusterName,
						Enabled: true,
					},
				},
				AuditSessionLogs: TeleportOptionsS3Sync{
					S3Path:      auditSessionLogsS3Path,
					Credentials: *auditSessionLogsS3Credentials,
					Enabled:     true,
				},
				BackupSnapshot: OptionsBackupSnapshot{
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
			opts:                             TeleportBackupOptions{AuditCluster: TeleportBackupOptionsAudit{TeleportOptionsAudit{Name: auditClusterName, Enabled: true}}},
			simulateGetAuditClusterSizeError: true,
		},
		{
			desc:                   "error ensuring backup volume exists",
			simulateEnsurePVCError: true,
		},
		{
			desc:                             "error configuring core backup",
			simulateConfigureCoreBackupError: true,
		},
		{
			desc:                              "error configuring audit backup",
			opts:                              TeleportBackupOptions{AuditCluster: TeleportBackupOptionsAudit{TeleportOptionsAudit{Name: auditClusterName, Enabled: true}}},
			simulateConfigureAuditBackupError: true,
		},
		{
			desc: "error configuring audit session logs backup",
			opts: TeleportBackupOptions{AuditSessionLogs: TeleportOptionsS3Sync{S3Path: auditSessionLogsS3Path, Credentials: *auditSessionLogsS3Credentials, Enabled: true}},
			simulateConfigureAuditSessionLogsBackupError: true,
		},
		{
			desc:             "error running backup",
			simulateRunError: true,
		},
		{
			desc:                  "error creating snapshot",
			simulateSnapshotError: true,
		},
		{
			desc:                  "error waiting for snapshot",
			simulateWaitSnapError: true,
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

			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)
			mockCNPGClient := cnpg.NewMockClientInterface(t)
			mockESClient := externalsnapshotter.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()
			mockClient.EXPECT().CNPG().Return(mockCNPGClient).Maybe()
			mockClient.EXPECT().ES().Return(mockESClient).Maybe()

			mockRemoteStage := remote.NewMockRemoteStageInterface(t)
			mockCoreCNPGBackup := cnpgbackup.NewMockCNPGBackupInterface(t)
			mockAuditCNPGBackup := cnpgbackup.NewMockCNPGBackupInterface(t)
			mockAuditSessionLogsS3Sync := s3sync.NewMockS3SyncInterface(t)

			backupCount := 0

			teleport := &Teleport{
				kubeClusterClient: mockClient,
				newCNPGBackup: func() cnpgbackup.CNPGBackupInterface {
					backupCount++
					switch backupCount {
					case 1:
						return mockCoreCNPGBackup
					case 2:
						return mockAuditCNPGBackup
					default:
						assert.Fail(t, "too many calls to newCNPGBackup")
						return nil
					}
				},
				newS3Sync: func() s3sync.S3SyncInterface {
					return mockAuditSessionLogsS3Sync
				},
				newRemoteStage: func(kubeClusterClient kubecluster.ClientInterface, calledNamespace, calledEventName string, calledOpts remote.RemoteStageOptions) remote.RemoteStageInterface {
					assert.Equal(t, mockClient, kubeClusterClient)
					assert.Equal(t, namespace, calledNamespace)
					assert.True(t, strings.Contains(calledEventName, backupName))
					assert.Equal(t, tt.opts.ClusterServiceSearchDomains, calledOpts.ClusterServiceSearchDomains)
					assert.Equal(t, tt.opts.CleanupTimeout, calledOpts.CleanupTimeout)

					return mockRemoteStage
				},
			}

			rootCtx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateGetCoreClusterSizeError,
				tt.simulateGetAuditClusterSizeError,
				tt.simulateEnsurePVCError,
				tt.simulateConfigureCoreBackupError,
				tt.simulateConfigureAuditBackupError,
				tt.simulateConfigureAuditSessionLogsBackupError,
				tt.simulateRunError,
				tt.simulateSnapshotError,
				tt.simulateWaitSnapError,
			)

			// Setup mocks
			func() {
				// Get cluster sizes
				expectedPVCSize := tt.opts.VolumeSize
				if tt.opts.VolumeSize.IsZero() {
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
					// 2x sum of cluster allocated size
					expectedPVCSize = resource.MustParse("2Gi")

					if tt.opts.AuditCluster.Enabled {
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

				// Configuration
				backupOpts := cnpgbackup.CNPGBackupOptions{
					CloningOpts:    tt.opts.CloneClusterOptions,
					CleanupTimeout: tt.opts.CleanupTimeout,
				}

				mockCoreCNPGBackup.EXPECT().Configure(mockClient, namespace, coreClusterName, servingIssuerName, clientIssuerName, backupName, "backup-core.sql", mock.Anything).
					RunAndReturn(func(kubeClient kubecluster.ClientInterface, namespace, clusterName, servingCertIssuerName, clientCertIssuerName, drVolName, backupFileRelPath string, opts cnpgbackup.CNPGBackupOptions) error {
						assert.NotEmpty(t, opts.CloningOpts.RecoveryTargetTime)
						backupOpts.CloningOpts.RecoveryTargetTime = opts.CloningOpts.RecoveryTargetTime

						assert.Equal(t, backupOpts, opts)

						return th.ErrIfTrue(tt.simulateConfigureCoreBackupError)
					})
				if tt.simulateConfigureCoreBackupError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockCoreCNPGBackup).Return(mockRemoteStage)

				if tt.opts.AuditCluster.Enabled {
					mockAuditCNPGBackup.EXPECT().Configure(mockClient, namespace, auditClusterName, servingIssuerName, clientIssuerName, backupName, "backup-audit.sql", mock.Anything).
						RunAndReturn(func(kubeClient kubecluster.ClientInterface, namespace, clusterName, servingCertIssuerName, clientCertIssuerName, drVolName, backupFileRelPath string, opts cnpgbackup.CNPGBackupOptions) error {
							assert.NotEmpty(t, opts.CloningOpts.RecoveryTargetTime)
							backupOpts.CloningOpts.RecoveryTargetTime = opts.CloningOpts.RecoveryTargetTime

							assert.Equal(t, backupOpts, opts)

							return th.ErrIfTrue(tt.simulateConfigureAuditBackupError)
						})
					if tt.simulateConfigureAuditBackupError {
						return
					}
					mockRemoteStage.EXPECT().WithAction(mock.Anything, mockAuditCNPGBackup).Return(mockRemoteStage)
				}

				if tt.opts.AuditSessionLogs.Enabled {
					mockAuditSessionLogsS3Sync.EXPECT().Configure(mockClient, namespace, backupName, "audit-session-logs", auditSessionLogsS3Path, auditSessionLogsS3Credentials, s3sync.DirectionDownload, s3sync.S3SyncOptions{}).
						Return(th.ErrIfTrue(tt.simulateConfigureAuditSessionLogsBackupError))
					if tt.simulateConfigureAuditSessionLogsBackupError {
						return
					}
					mockRemoteStage.EXPECT().WithAction(mock.Anything, mockAuditSessionLogsS3Sync).Return(mockRemoteStage)
				}

				mockRemoteStage.EXPECT().Run(mock.Anything).Return(th.ErrIfTrue(tt.simulateRunError))
				if tt.simulateRunError {
					return
				}

				// Snapshot volume
				var createdSnapshotName string
				mockESClient.EXPECT().SnapshotVolume(mock.Anything, namespace, drPVC.Name, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, opts externalsnapshotter.SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Contains(t, opts.Name, helpers.CleanName(backupName))
						assert.NotEqual(t, opts.Name, helpers.CleanName(backupName))
						assert.Equal(t, tt.opts.BackupSnapshot.SnapshotClass, opts.SnapshotClass)

						createdSnapshotName = opts.Name

						return th.ErrOr1Val(&volumesnapshotv1.VolumeSnapshot{
							ObjectMeta: metav1.ObjectMeta{
								Name:      opts.Name,
								Namespace: namespace,
							},
						}, tt.simulateSnapshotError)
					})
				if tt.simulateSnapshotError {
					return
				}

				// Wait for snapshot
				mockESClient.EXPECT().WaitForReadySnapshot(mock.Anything, namespace, mock.Anything, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: tt.opts.BackupSnapshot.ReadyTimeout}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, snapsnotName string, wfrso externalsnapshotter.WaitForReadySnapshotOpts) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Equal(t, createdSnapshotName, snapsnotName)

						return th.ErrOr1Val(&volumesnapshotv1.VolumeSnapshot{}, tt.simulateWaitSnapError)
					})
			}()

			backup, err := teleport.Backup(rootCtx, namespace, backupName, coreClusterName,
				servingIssuerName, clientIssuerName, tt.opts)

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

func TestTeleportRestoreOptions(t *testing.T) {
	th.OptStructTest[TeleportRestoreOptions](t)
}

func TestTeleportRestore(t *testing.T) {
	namespace := "test-ns"
	restoreName := "test-restore"
	coreClusterName := "test-core-cluster"
	coreServingCertName := "test-core-serving-cert"
	coreClientCertIssuerName := "test-core-client-cert-issuer"
	auditClusterName := "test-audit-cluster"
	auditServingCertName := "test-audit-serving-cert"
	auditClientCertIssuerName := "test-audit-client-cert-issuer"
	auditSessionLogsS3Path := "s3://audit-session-logs"
	auditSessionLogsS3Credentials := s3.NewCredentials("accessKeyID", "secretAccessKey")

	auditClusterOptions := TeleportRestoreOptionsAudit{
		TeleportOptionsAudit: TeleportOptionsAudit{
			Name:    auditClusterName,
			Enabled: true,
		},
		ServingCertName:      auditServingCertName,
		ClientCertIssuerName: auditClientCertIssuerName,
	}

	auditSessionLogsOptions := TeleportOptionsS3Sync{
		S3Path:      auditSessionLogsS3Path,
		Credentials: *auditSessionLogsS3Credentials,
		Enabled:     true,
	}

	tests := []struct {
		desc                                string
		opts                                TeleportRestoreOptions
		simulateCoreConfigError             bool
		simulateAuditConfigError            bool
		simulateAuditSessionLogsConfigError bool
		simulateRunError                    bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			opts: TeleportRestoreOptions{
				AuditCluster:     auditClusterOptions,
				AuditSessionLogs: auditSessionLogsOptions,
				CleanupTimeout:   helpers.MaxWaitTime(3 * time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)

			mockRemoteStage := remote.NewMockRemoteStageInterface(t)
			mockCoreCNPGRestore := cnpgrestore.NewMockCNPGRestoreInterface(t)
			mockAuditCNPGRestore := cnpgrestore.NewMockCNPGRestoreInterface(t)
			mockAuditSessionLogsS3Sync := s3sync.NewMockS3SyncInterface(t)

			restoreCount := 0

			teleport := &Teleport{
				kubeClusterClient: mockClient,
				newCNPGRestore: func() cnpgrestore.CNPGRestoreInterface {
					restoreCount++
					switch restoreCount {
					case 1:
						return mockCoreCNPGRestore
					case 2:
						return mockAuditCNPGRestore
					default:
						assert.Fail(t, "too many calls to newCNPGRestore")
						return nil
					}
				},
				newS3Sync: func() s3sync.S3SyncInterface {
					return mockAuditSessionLogsS3Sync
				},
				newRemoteStage: func(kubeClusterClient kubecluster.ClientInterface, calledNamespace, calledEventName string, calledOpts remote.RemoteStageOptions) remote.RemoteStageInterface {
					assert.Equal(t, mockClient, kubeClusterClient)
					assert.Equal(t, namespace, calledNamespace)
					assert.True(t, strings.Contains(calledEventName, restoreName))
					assert.Equal(t, tt.opts.ClusterServiceSearchDomains, calledOpts.ClusterServiceSearchDomains)
					assert.Equal(t, tt.opts.CleanupTimeout, calledOpts.CleanupTimeout)

					return mockRemoteStage
				},
			}

			rootCtx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateCoreConfigError,
				tt.simulateAuditConfigError,
				tt.simulateAuditSessionLogsConfigError,
				tt.simulateRunError,
			)

			func() {
				mockCoreCNPGRestore.EXPECT().Configure(mockClient, namespace, coreClusterName, coreServingCertName, coreClientCertIssuerName, restoreName, "backup-core.sql", cnpgrestore.CNPGRestoreOptions{
					PostgresUserCert: tt.opts.PostgresUserCert,
					CleanupTimeout:   tt.opts.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateCoreConfigError))
				if tt.simulateCoreConfigError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockCoreCNPGRestore).Return(mockRemoteStage)

				if tt.opts.AuditCluster.Enabled {
					mockAuditCNPGRestore.EXPECT().Configure(mockClient, namespace, auditClusterName, auditServingCertName, auditClientCertIssuerName, restoreName, "backup-audit.sql", cnpgrestore.CNPGRestoreOptions{
						PostgresUserCert: tt.opts.AuditCluster.PostgresUserCert,
						CleanupTimeout:   tt.opts.CleanupTimeout,
					}).Return(th.ErrIfTrue(tt.simulateAuditConfigError))
					if tt.simulateAuditConfigError {
						return
					}
					mockRemoteStage.EXPECT().WithAction(mock.Anything, mockAuditCNPGRestore).Return(mockRemoteStage)
				}

				if tt.opts.AuditSessionLogs.Enabled {
					mockAuditSessionLogsS3Sync.EXPECT().Configure(mockClient, namespace, restoreName, "audit-session-logs", auditSessionLogsS3Path, auditSessionLogsS3Credentials, s3sync.DirectionUpload, s3sync.S3SyncOptions{}).
						Return(th.ErrIfTrue(tt.simulateAuditSessionLogsConfigError))
					if tt.simulateAuditSessionLogsConfigError {
						return
					}
					mockRemoteStage.EXPECT().WithAction(mock.Anything, mockAuditSessionLogsS3Sync).Return(mockRemoteStage)
				}

				mockRemoteStage.EXPECT().Run(mock.Anything).Return(th.ErrIfTrue(tt.simulateRunError))
			}()

			restore, err := teleport.Restore(rootCtx, namespace, restoreName, coreClusterName, coreServingCertName, coreClientCertIssuerName, tt.opts)

			require.NotNil(t, restore)
			assert.NotEmpty(t, restore.StartTime)
			assert.NotEmpty(t, restore.EndTime)

			if wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
