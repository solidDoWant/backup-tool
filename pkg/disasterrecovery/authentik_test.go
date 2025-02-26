package disasterrecovery

import (
	"strings"
	"testing"
	"time"

	v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	cnpgbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/backup"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewAuthentik(t *testing.T) {
	mockClient := kubecluster.NewMockClientInterface(t)
	authentik := NewAuthentik(mockClient)

	require.NotNil(t, authentik)
	assert.Equal(t, mockClient, authentik.kubeClusterClient)
	assert.NotNil(t, authentik.newCNPGRestore)
}

func TestAuthentikBackupOptions(t *testing.T) {
	th.OptStructTest[AuthentikBackupOptions](t)
}

func TestAuthentikBackup(t *testing.T) {
	backupName := "test-backup"
	namespace := "test-ns"
	clusterName := "test-cluster"
	servingIssuerName := "serving-cert-issuer"
	clientIssuerName := "client-cert-issuer"
	mediaS3Path := "s3://media"
	mediaS3Credentials := s3.NewCredentials("accessKeyID", "secretAccessKey")

	tests := []struct {
		desc                             string
		opts                             AuthentikBackupOptions
		simulateEnsurePVCError           bool
		simulateConfigureCNPGBackupError bool
		simulateConfigureS3SyncError     bool
		simulateRunError                 bool
		simulateSnapshotError            bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			opts: AuthentikBackupOptions{
				VolumeSize:         resource.MustParse("10Gi"),
				VolumeStorageClass: "custom-storage-class",
				CloneClusterOptions: clonedcluster.CloneClusterOptions{
					Certificates: clonedcluster.CloneClusterOptionsCertificates{
						ServingCert: clonedcluster.CloneClusterOptionsExternallyIssuedCertificate{
							IssuerKind: "ClusterIssuer",
						},
						ClientCACert: clonedcluster.CloneClusterOptionsExternallyIssuedCertificate{
							IssuerKind: "Issuer",
						},
					},
					CleanupTimeout: helpers.MaxWaitTime(5 * time.Second),
				},
				RemoteBackupToolOptions: backuptoolinstance.CreateBackupToolInstanceOptions{
					ServiceWaitTimeout: helpers.ShortWaitTime,
				},
				ClusterServiceSearchDomains: []string{"cluster.local"},
				BackupSnapshot: OptionsBackupSnapshot{
					ReadyTimeout:  helpers.MaxWaitTime(2 * time.Second),
					SnapshotClass: "custom-snapshot-class",
				},
				CleanupTimeout: helpers.MaxWaitTime(3 * time.Second),
			},
		},
		{
			desc:                   "error ensuring backup volume exists",
			simulateEnsurePVCError: true,
		},
		{
			desc:                             "error configuring CNPG backup",
			simulateConfigureCNPGBackupError: true,
		},
		{
			desc:                         "error configuring S3 sync",
			simulateConfigureS3SyncError: true,
		},
		{
			desc:             "error running backup",
			simulateRunError: true,
		},
		{
			desc:                  "error creating snapshot",
			simulateSnapshotError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)

			mockDRVolume := drvolume.NewMockDRVolumeInterface(t)

			mockRemoteStage := remote.NewMockRemoteStageInterface(t)
			mockCNPGBackup := cnpgbackup.NewMockCNPGBackupInterface(t)
			mockS3Sync := s3sync.NewMockS3SyncInterface(t)

			authentik := &Authentik{
				kubeClusterClient: mockClient,
				newCNPGBackup: func() cnpgbackup.CNPGBackupInterface {
					return mockCNPGBackup
				},
				newS3Sync: func() s3sync.S3SyncInterface {
					return mockS3Sync
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
				tt.simulateEnsurePVCError,
				tt.simulateConfigureCNPGBackupError,
				tt.simulateConfigureS3SyncError,
				tt.simulateRunError,
				tt.simulateSnapshotError,
			)

			// Setup mocks
			func() {
				// DR PVC
				mockClient.EXPECT().NewDRVolume(mock.Anything, namespace, backupName, tt.opts.VolumeSize, drvolume.DRVolumeCreateOptions{
					VolumeStorageClass: tt.opts.VolumeStorageClass,
					CNPGClusterNames:   []string{clusterName},
				}).RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, configuredSize resource.Quantity, opts drvolume.DRVolumeCreateOptions) (drvolume.DRVolumeInterface, error) {
					assert.True(t, calledCtx.IsChildOf(rootCtx))

					return th.ErrOr1Val(mockDRVolume, tt.simulateEnsurePVCError)
				})
				if tt.simulateEnsurePVCError {
					return
				}

				// Configuration
				mockCNPGBackup.EXPECT().Configure(mockClient, namespace, clusterName, servingIssuerName, clientIssuerName, backupName, "dump.sql", cnpgbackup.CNPGBackupOptions{
					CloningOpts:    tt.opts.CloneClusterOptions,
					CleanupTimeout: tt.opts.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigureCNPGBackupError))
				if tt.simulateConfigureCNPGBackupError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockCNPGBackup).Return(mockRemoteStage)

				mockS3Sync.EXPECT().Configure(mockClient, namespace, backupName, "media", mediaS3Path, mediaS3Credentials, s3sync.DirectionDownload, s3sync.S3SyncOptions{}).
					Return(th.ErrIfTrue(tt.simulateConfigureS3SyncError))
				if tt.simulateConfigureS3SyncError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockS3Sync).Return(mockRemoteStage)

				mockRemoteStage.EXPECT().Run(mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))

						return th.ErrIfTrue(tt.simulateRunError)
					})
				if tt.simulateRunError {
					return
				}

				// Snapshot volume
				mockDRVolume.EXPECT().SnapshotAndWaitReady(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, snapshotName string, opts drvolume.DRVolumeSnapshotAndWaitOptions) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Contains(t, snapshotName, helpers.CleanName(backupName))
						assert.NotEqual(t, snapshotName, helpers.CleanName(backupName))
						assert.Equal(t, tt.opts.BackupSnapshot.SnapshotClass, opts.SnapshotClass)
						assert.Equal(t, tt.opts.BackupSnapshot.ReadyTimeout, opts.ReadyTimeout)

						return th.ErrIfTrue(tt.simulateSnapshotError)
					})
				if tt.simulateSnapshotError {
					return
				}
			}()

			backup, err := authentik.Backup(rootCtx, namespace, backupName, clusterName,
				servingIssuerName, clientIssuerName, mediaS3Path, mediaS3Credentials, tt.opts)

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

func TestAuthentikRestoreOptions(t *testing.T) {
	th.OptStructTest[AuthentikRestoreOptions](t)
}

func TestAuthentikRestore(t *testing.T) {
	namespace := "test-ns"
	restoreName := "test-restore"
	clusterName := "test-cluster"
	servingCertName := "test-serving-cert"
	clientCertIssuerName := "test-client-cert-issuer"
	mediaS3Path := "s3://media"
	mediaS3Credentials := s3.NewCredentials("accessKeyID", "secretAccessKey")

	tests := []struct {
		desc                     string
		opts                     AuthentikRestoreOptions
		simulateCNPGRestoreError bool
		simulateS3SyncError      bool
		simulateRunError         bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			opts: AuthentikRestoreOptions{
				PostgresUserCert: cnpgrestore.CNPGRestoreOptionsCert{
					Subject: &v1.X509Subject{
						Organizations: []string{"test-org"},
					},
				},
				IssuerKind: "ClusterIssuer",
				RemoteBackupToolOptions: backuptoolinstance.CreateBackupToolInstanceOptions{
					ServiceWaitTimeout: helpers.ShortWaitTime,
				},
				ClusterServiceSearchDomains: []string{"cluster.local"},
				CleanupTimeout:              helpers.MaxWaitTime(3 * time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)

			mockRemoteStage := remote.NewMockRemoteStageInterface(t)
			mockCNPGRestore := cnpgrestore.NewMockCNPGRestoreInterface(t)
			mockS3Sync := s3sync.NewMockS3SyncInterface(t)

			authentik := &Authentik{
				kubeClusterClient: mockClient,
				newCNPGRestore: func() cnpgrestore.CNPGRestoreInterface {
					return mockCNPGRestore
				},
				newS3Sync: func() s3sync.S3SyncInterface {
					return mockS3Sync
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
				tt.simulateCNPGRestoreError,
				tt.simulateS3SyncError,
				tt.simulateRunError,
			)

			func() {
				mockCNPGRestore.EXPECT().Configure(mockClient, namespace, clusterName, servingCertName, clientCertIssuerName, restoreName, "dump.sql", cnpgrestore.CNPGRestoreOptions{
					PostgresUserCert: tt.opts.PostgresUserCert,
					CleanupTimeout:   tt.opts.CleanupTimeout,
					IssuerKind:       tt.opts.IssuerKind,
				}).Return(th.ErrIfTrue(tt.simulateCNPGRestoreError))
				if tt.simulateCNPGRestoreError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockCNPGRestore).Return(mockRemoteStage)

				mockS3Sync.EXPECT().Configure(mockClient, namespace, restoreName, "media", mediaS3Path, mediaS3Credentials, s3sync.DirectionUpload, s3sync.S3SyncOptions{}).
					Return(th.ErrIfTrue(tt.simulateS3SyncError))
				if tt.simulateS3SyncError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockS3Sync).Return(mockRemoteStage)

				mockRemoteStage.EXPECT().Run(mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))

						return th.ErrIfTrue(tt.simulateRunError)
					})
			}()

			restore, err := authentik.Restore(rootCtx, namespace, restoreName, clusterName, servingCertName, clientCertIssuerName, mediaS3Path, mediaS3Credentials, tt.opts)

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
