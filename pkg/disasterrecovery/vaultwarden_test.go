package disasterrecovery

import (
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"strings"
	"testing"
	"time"

	v1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	cnpgbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/backup"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	filesbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/backup"
	filesrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/restore"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewVaultWarden(t *testing.T) {
	mockClient := kubecluster.NewMockClientInterface(t)
	vw := NewVaultWarden(mockClient)

	require.NotNil(t, vw)
	assert.Equal(t, mockClient, vw.kubeClusterClient)
	assert.NotNil(t, vw.newCNPGBackup)
	assert.NotNil(t, vw.newCNPGRestore)
	assert.NotNil(t, vw.newFilesBackup)
	assert.NotNil(t, vw.newFilesRestore)
	assert.NotNil(t, vw.newRemoteStage)
}

func TestVaultWardenBackupOptions(t *testing.T) {
	th.OptStructTest[VaultWardenBackupOptions](t)
}

func TestVaultWardenBackup(t *testing.T) {
	backupName := "test-backup"
	namespace := "test-ns"
	dataPVCName := "test-data-pvc"
	clusterName := "test-cluster"

	tests := []struct {
		desc                             string
		opts                             VaultWardenBackupOptions
		simulateGetDataPVCError          bool
		simulateNewDRVolumeError         bool
		simulateConfigureCNPGBackupError bool
		simulateConfigureFilesBackupErr  bool
		simulateRunError                 bool
		simulateSnapshotError            bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			opts: VaultWardenBackupOptions{
				VolumeSize:         resource.MustParse("10Gi"),
				VolumeStorageClass: "custom-storage-class",
				CloneClusterOptions: clonedcluster.CloneClusterOptions{
					CleanupTimeout: helpers.MaxWaitTime(5 * time.Second),
				},
				BackupSnapshot: OptionsBackupSnapshot{
					ReadyTimeout:  helpers.MaxWaitTime(2 * time.Second),
					SnapshotClass: "custom-snapshot-class",
				},
				CleanupTimeout: helpers.MaxWaitTime(3 * time.Second),
			},
		},
		{
			desc:                    "error getting data PVC for sizing",
			simulateGetDataPVCError: true,
		},
		{
			desc:                     "error creating DR volume",
			simulateNewDRVolumeError: true,
		},
		{
			desc:                             "error configuring CNPG backup",
			simulateConfigureCNPGBackupError: true,
		},
		{
			desc:                            "error configuring files backup",
			simulateConfigureFilesBackupErr: true,
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
			mockCoreClient := core.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()

			mockDRVolume := drvolume.NewMockDRVolumeInterface(t)

			mockRemoteStage := remote.NewMockRemoteStageInterface(t)
			mockCNPGBackup := cnpgbackup.NewMockCNPGBackupInterface(t)
			mockFilesBackup := filesbackup.NewMockFilesBackupInterface(t)

			vw := &VaultWarden{
				kubeClusterClient: mockClient,
				newCNPGBackup: func() cnpgbackup.CNPGBackupInterface {
					return mockCNPGBackup
				},
				newFilesBackup: func() filesbackup.FilesBackupInterface {
					return mockFilesBackup
				},
				newRemoteStage: func(kubeClusterClient kubecluster.ClientInterface, calledNamespace, calledEventName string, calledOpts remote.RemoteStageOptions) remote.RemoteStageInterface {
					assert.Equal(t, mockClient, kubeClusterClient)
					assert.Equal(t, namespace, calledNamespace)
					assert.True(t, strings.Contains(calledEventName, backupName))
					assert.Equal(t, tt.opts.CleanupTimeout, calledOpts.CleanupTimeout)

					return mockRemoteStage
				},
			}

			rootCtx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateGetDataPVCError,
				tt.simulateNewDRVolumeError,
				tt.simulateConfigureCNPGBackupError,
				tt.simulateConfigureFilesBackupErr,
				tt.simulateRunError,
				tt.simulateSnapshotError,
			)

			// Setup mocks
			func() {
				// DR volume sizing: only reads the data PVC when no explicit size was configured.
				expectedDRVolumeSize := tt.opts.VolumeSize
				if tt.opts.VolumeSize.IsZero() {
					dataPVCSize := resource.MustParse("5Gi")
					mockCoreClient.EXPECT().GetPVC(mock.Anything, namespace, dataPVCName).
						RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
							assert.True(t, calledCtx.IsChildOf(rootCtx))

							return th.ErrOr1Val(&corev1.PersistentVolumeClaim{
								Spec: corev1.PersistentVolumeClaimSpec{
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{corev1.ResourceStorage: dataPVCSize},
									},
								},
							}, tt.simulateGetDataPVCError)
						})
					if tt.simulateGetDataPVCError {
						return
					}

					expectedDRVolumeSize = dataPVCSize
					expectedDRVolumeSize.Mul(2)
				}

				mockClient.EXPECT().NewDRVolume(mock.Anything, namespace, backupName, expectedDRVolumeSize, drvolume.DRVolumeCreateOptions{
					VolumeStorageClass: tt.opts.VolumeStorageClass,
					CNPGClusterNames:   []string{clusterName},
				}).RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, configuredSize resource.Quantity, opts drvolume.DRVolumeCreateOptions) (drvolume.DRVolumeInterface, error) {
					assert.True(t, calledCtx.IsChildOf(rootCtx))

					return th.ErrOr1Val(mockDRVolume, tt.simulateNewDRVolumeError)
				})
				if tt.simulateNewDRVolumeError {
					return
				}

				// Configuration - the CNPG capture is registered before the data directory capture so the
				// base backup is taken before the data PVC clone that pins the consistency point. The issuers
				// are now part of the cloning options (clusterCloning.certificates.*), passed through verbatim.
				mockCNPGBackup.EXPECT().Configure(mockClient, namespace, clusterName, backupName, "dump.sql", cnpgbackup.CNPGBackupOptions{
					CloningOpts:    tt.opts.CloneClusterOptions,
					CleanupTimeout: tt.opts.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigureCNPGBackupError))
				if tt.simulateConfigureCNPGBackupError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockCNPGBackup).Return(mockRemoteStage)

				mockFilesBackup.EXPECT().Configure(mockClient, namespace, dataPVCName, backupName, "data-vol", filesbackup.FilesBackupOptions{
					CleanupTimeout: tt.opts.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigureFilesBackupErr))
				if tt.simulateConfigureFilesBackupErr {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockFilesBackup).Return(mockRemoteStage)

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
						assert.Equal(t, tt.opts.BackupSnapshot.SnapshotClass, opts.SnapshotClass)
						assert.Equal(t, tt.opts.BackupSnapshot.ReadyTimeout, opts.ReadyTimeout)

						return th.ErrIfTrue(tt.simulateSnapshotError)
					})
				if tt.simulateSnapshotError {
					return
				}
			}()

			backup, err := vw.Backup(rootCtx, namespace, backupName, dataPVCName, clusterName, tt.opts)

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

func TestVaultWardenRestoreOptions(t *testing.T) {
	th.OptStructTest[VaultWardenRestoreOptions](t)
}

func TestVaultWardenRestore(t *testing.T) {
	namespace := "test-ns"
	restoreName := "test-restore"
	dataPVCName := "test-data-pvc"
	clusterName := "test-cluster"
	servingCertName := "test-serving-cert"
	clientCAIssuer := cmmeta.IssuerReference{Name: "test-client-cert-issuer", Kind: "ClusterIssuer"}

	tests := []struct {
		desc                             string
		opts                             VaultWardenRestoreOptions
		simulateConfigureFilesRestoreErr bool
		simulateCNPGRestoreError         bool
		simulateRunError                 bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			opts: VaultWardenRestoreOptions{
				PostgresUserCert: cnpgrestore.CNPGRestoreOptionsCert{
					Subject: &v1.X509Subject{
						Organizations: []string{"test-org"},
					},
					WaitForCertTimeout: helpers.MaxWaitTime(4 * time.Second),
				},
				RemoteBackupToolOptions: backuptoolinstance.CreateBackupToolInstanceOptions{},
				CleanupTimeout:          helpers.MaxWaitTime(3 * time.Second),
			},
		},
		{
			desc:                     "error configuring CNPG restore",
			simulateCNPGRestoreError: true,
		},
		{
			desc:                             "error configuring files restore",
			simulateConfigureFilesRestoreErr: true,
		},
		{
			desc:             "error running restore",
			simulateRunError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)

			mockRemoteStage := remote.NewMockRemoteStageInterface(t)
			mockCNPGRestore := cnpgrestore.NewMockCNPGRestoreInterface(t)
			mockFilesRestore := filesrestore.NewMockFilesRestoreInterface(t)

			vw := &VaultWarden{
				kubeClusterClient: mockClient,
				newCNPGRestore: func() cnpgrestore.CNPGRestoreInterface {
					return mockCNPGRestore
				},
				newFilesRestore: func() filesrestore.FilesRestoreInterface {
					return mockFilesRestore
				},
				newRemoteStage: func(kubeClusterClient kubecluster.ClientInterface, calledNamespace, calledEventName string, calledOpts remote.RemoteStageOptions) remote.RemoteStageInterface {
					assert.Equal(t, mockClient, kubeClusterClient)
					assert.Equal(t, namespace, calledNamespace)
					assert.True(t, strings.Contains(calledEventName, restoreName))
					assert.Equal(t, tt.opts.CleanupTimeout, calledOpts.CleanupTimeout)

					return mockRemoteStage
				},
			}

			rootCtx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateCNPGRestoreError,
				tt.simulateConfigureFilesRestoreErr,
				tt.simulateRunError,
			)

			func() {
				mockCNPGRestore.EXPECT().Configure(mockClient, namespace, clusterName, servingCertName, clientCAIssuer, restoreName, "dump.sql", cnpgrestore.CNPGRestoreOptions{
					PostgresUserCert: tt.opts.PostgresUserCert,
					CleanupTimeout:   tt.opts.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateCNPGRestoreError))
				if tt.simulateCNPGRestoreError {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockCNPGRestore).Return(mockRemoteStage)

				mockFilesRestore.EXPECT().Configure(mockClient, namespace, dataPVCName, restoreName, "data-vol", filesrestore.FilesRestoreOptions{}).
					Return(th.ErrIfTrue(tt.simulateConfigureFilesRestoreErr))
				if tt.simulateConfigureFilesRestoreErr {
					return
				}
				mockRemoteStage.EXPECT().WithAction(mock.Anything, mockFilesRestore).Return(mockRemoteStage)

				mockRemoteStage.EXPECT().Run(mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))

						return th.ErrIfTrue(tt.simulateRunError)
					})
			}()

			restore, err := vw.Restore(rootCtx, namespace, restoreName, dataPVCName, clusterName, servingCertName, clientCAIssuer, tt.opts)

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
