package disasterrecovery

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
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
	"k8s.io/utils/ptr"
)

func TestNewVaultWarden(t *testing.T) {
	mockClient := kubecluster.NewMockClientInterface(t)
	vw := NewVaultWarden(mockClient)

	require.NotNil(t, vw)
	assert.Equal(t, mockClient, vw.kubernetesClient)
}

func TestVaultWardenBackupOptions(t *testing.T) {
	th.OptStructTest[VaultWardenBackupOptions](t)
}

func TestVaultWardenBackup(t *testing.T) {
	namespace := "test-ns"
	backupName := "test-backup"
	dataPVC := "test-data-pvc"
	clonedPVCName := "test-cloned-pvc"
	clusterName := "test-cluster"
	servingIssuerName := "serving-cert-issuer"
	clientIssuerName := "client-cert-issuer"

	tests := []struct {
		desc                           string
		backupOptions                  VaultWardenBackupOptions
		simulateClonePVCError          bool
		simulatePVCCleanupError        bool
		simulateEnsurePVCError         bool
		simulateCloneClusterErr        bool
		simulateCloneClusterCleanupErr bool
		simulateBTICreateError         bool
		simulateBTICleanupError        bool
		simulateGRPCClientErr          bool
		simulateSyncFilesErr           bool
		simulateDumpAllErr             bool
		simulateSnapshotErr            bool
		simulateWaitSnapErr            bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			backupOptions: VaultWardenBackupOptions{
				VolumeSize:         resource.MustParse("10Gi"),
				VolumeStorageClass: "custom-storage-class",
				CloneClusterOptions: clonedcluster.CloneClusterOptions{
					CleanupTimeout: helpers.MaxWaitTime(5 * time.Second),
				},
				BackupToolPodCreationTimeout: helpers.MaxWaitTime(1 * time.Second),
				BackupSnapshot: VaultWardenBackupOptionsBackupSnapshot{
					ReadyTimeout:  helpers.MaxWaitTime(2 * time.Second),
					SnapshotClass: "custom-snapshot-class",
				},
				CleanupTimeout: helpers.MaxWaitTime(3 * time.Second),
			},
		},
		{
			desc:                  "error cloning PVC",
			simulateClonePVCError: true,
		},
		{
			desc:                   "error ensuring backup volume",
			simulateEnsurePVCError: true,
		},
		{
			desc:                    "error cloning cluster",
			simulateCloneClusterErr: true,
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
			desc:                 "error syncing data directory",
			simulateSyncFilesErr: true,
		},
		{
			desc:               "error dumping logical backup",
			simulateDumpAllErr: true,
		},
		{
			desc:                "error snapshot volume",
			simulateSnapshotErr: true,
		},
		{
			desc:                "error waiting for snapshot",
			simulateWaitSnapErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			clonedPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clonedPVCName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			clonedCluster := clonedcluster.NewMockClonedClusterInterface(t)
			servingCert := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serving-cert",
					Namespace: namespace,
				},
			}
			postgresCertificate := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "postgres-cert",
					Namespace: namespace,
				},
			}
			btInstance := backuptoolinstance.NewMockBackupToolInstanceInterface(t)
			credentials := postgres.EnvironmentCredentials{
				postgres.UserVarName: "postgres",
			}

			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)
			mockESClient := externalsnapshotter.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()
			mockClient.EXPECT().ES().Return(mockESClient).Maybe()

			mockGRPCClient := clients.NewMockClientInterface(t)
			mockFilesRuntime := files.NewMockRuntime(t)
			mockPostgresRuntime := postgres.NewMockRuntime(t)
			mockGRPCClient.EXPECT().Files().Return(mockFilesRuntime).Maybe()
			mockGRPCClient.EXPECT().Postgres().Return(mockPostgresRuntime).Maybe()

			vw := &VaultWarden{
				kubernetesClient: mockClient,
			}

			rootCtx := th.NewTestContext()

			wantErr := th.ErrExpected(
				tt.simulateClonePVCError,
				tt.simulatePVCCleanupError,
				tt.simulateEnsurePVCError,
				tt.simulateCloneClusterErr,
				tt.simulateCloneClusterCleanupErr,
				tt.simulateBTICreateError,
				tt.simulateBTICleanupError,
				tt.simulateGRPCClientErr,
				tt.simulateSyncFilesErr,
				tt.simulateDumpAllErr,
				tt.simulateSnapshotErr,
				tt.simulateWaitSnapErr,
			)

			// Setup mocks
			func() {
				// Step 1
				var fullBackupName string
				mockClient.EXPECT().ClonePVC(mock.Anything, namespace, dataPVC, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, dataPVC string, opts clonepvc.ClonePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))

						fullBackupName = opts.DestPvcNamePrefix
						require.True(t, strings.HasPrefix(fullBackupName, backupName))
						require.Equal(t, tt.backupOptions.CleanupTimeout, opts.CleanupTimeout)
						return th.ErrOr1Val(clonedPVC, tt.simulateClonePVCError)
					})
				if tt.simulateClonePVCError {
					return
				}
				mockCoreClient.EXPECT().DeletePVC(mock.Anything, namespace, clonedPVCName).RunAndReturn(func(cleanupCtx *contexts.Context, _, _ string) error {
					require.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulatePVCCleanupError)
				})

				// Step 2
				mockCoreClient.EXPECT().EnsurePVCExists(mock.Anything, namespace, backupName, mock.Anything, core.CreatePVCOptions{StorageClassName: tt.backupOptions.VolumeStorageClass}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						require.Equal(t, backupName, pvcName)
						require.GreaterOrEqual(t, size.AsFloat64Slow(), ptr.To(clonedPVC.Spec.Resources.Requests[corev1.ResourceStorage]).AsFloat64Slow())
						return th.ErrOr1Val(clonedPVC, tt.simulateEnsurePVCError)
					})
				if tt.simulateEnsurePVCError {
					return
				}

				// Step 3
				mockClient.EXPECT().CloneCluster(mock.Anything, namespace, clusterName, mock.Anything, servingIssuerName, clientIssuerName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, existingClusterName, newClusterName, servingIssuerName, clientIssuerName string, opts clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.True(t, strings.Contains(newClusterName, helpers.CleanName(fullBackupName)))
						assert.LessOrEqual(t, len(newClusterName), 50)

						return th.ErrOr1Val(clonedCluster, tt.simulateCloneClusterErr)
					})
				if tt.simulateCloneClusterErr {
					return
				}
				clonedCluster.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
					require.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateCloneClusterCleanupErr)
				})

				// Step 4
				clonedCluster.EXPECT().GetServingCert().Return(&servingCert)
				userCert := clusterusercert.NewMockClusterUserCertInterface(t)
				clonedCluster.EXPECT().GetPostgresUserCert().Return(userCert)
				userCert.EXPECT().GetCertificate().Return(&postgresCertificate)
				mockClient.EXPECT().CreateBackupToolInstance(mock.Anything, namespace, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, instance string, opts backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Contains(t, opts.NamePrefix, fullBackupName)
						assert.Contains(t, opts.NamePrefix, constants.ToolName)
						// TODO add test to ensure that the secrets are attached, along with the DR and cloned data PVCs
						assert.Len(t, opts.Volumes, 4)
						return th.ErrOr1Val(btInstance, tt.simulateBTICreateError)
					})
				if tt.simulateBTICreateError {
					return
				}
				btInstance.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
					require.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateBTICleanupError)
				})

				// Step 5
				btInstance.EXPECT().GetGRPCClient(mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, searchDomains ...string) (clients.ClientInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Equal(t, tt.backupOptions.ClusterServiceSearchDomains, searchDomains)
						return th.ErrOr1Val(mockGRPCClient, tt.simulateGRPCClientErr)
					})
				if tt.simulateGRPCClientErr {
					return
				}

				mockFilesRuntime.EXPECT().SyncFiles(mock.Anything, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, src, dest string) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.NotEqual(t, src, dest)
						assert.True(t, strings.HasPrefix(src, vaultwardenBaseMountPath))
						assert.True(t, strings.HasPrefix(dest, vaultwardenBaseMountPath))
						assert.True(t, filepath.Base(dest) == "data-vol") // Important: changing this is will break restoration of old backups!

						return th.ErrIfTrue(tt.simulateSyncFilesErr)
					})
				if tt.simulateSyncFilesErr {
					return
				}

				// Step 6
				clonedCluster.EXPECT().GetCredentials(mock.Anything, mock.Anything).
					RunAndReturn(func(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials {
						assert.NotEqual(t, servingCertMountDirectory, clientCertMountDirectory)
						assert.True(t, strings.HasPrefix(servingCertMountDirectory, vaultwardenBaseMountPath))
						assert.True(t, strings.HasPrefix(clientCertMountDirectory, vaultwardenBaseMountPath))

						return credentials
					})
				mockPostgresRuntime.EXPECT().DumpAll(mock.Anything, credentials, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, creds postgres.Credentials, outputFilePath string, opts postgres.DumpAllOptions) error {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.True(t, filepath.Base(outputFilePath) == "dump.sql") // Important: changing this is will break restoration of old backups!
						return th.ErrIfTrue(tt.simulateDumpAllErr)
					})
				if tt.simulateDumpAllErr {
					return
				}

				// Step 7
				mockESClient.EXPECT().SnapshotVolume(mock.Anything, namespace, clonedPVCName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, opts externalsnapshotter.SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Equal(t, helpers.CleanName(fullBackupName), opts.Name)
						assert.Equal(t, tt.backupOptions.BackupSnapshot.SnapshotClass, opts.SnapshotClass)

						return th.ErrOr1Val(&volumesnapshotv1.VolumeSnapshot{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fullBackupName,
								Namespace: namespace,
							},
						}, tt.simulateSnapshotErr)
					})
				if tt.simulateSnapshotErr {
					return
				}

				mockESClient.EXPECT().WaitForReadySnapshot(mock.Anything, namespace, mock.Anything, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: tt.backupOptions.BackupSnapshot.ReadyTimeout}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, snapsnotName string, wfrso externalsnapshotter.WaitForReadySnapshotOpts) (*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Equal(t, fullBackupName, snapsnotName)
						return th.ErrOr1Val(&volumesnapshotv1.VolumeSnapshot{}, tt.simulateWaitSnapErr)
					})
			}()

			backup, err := vw.Backup(rootCtx, namespace, backupName, dataPVC, clusterName,
				servingIssuerName, clientIssuerName, tt.backupOptions)

			require.NotNil(t, backup)
			assert.NotEmpty(t, backup.StartTime)
			assert.NotEmpty(t, backup.EndTime)

			if wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
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
	clientIssuerName := "test-client-issuer"
	writeServiceName := "test-write-service"

	tests := []struct {
		desc                          string
		restoreOptions                VaultWardenRestoreOptions
		simulateGetDRPVCError         bool
		simulateGetDataPVCError       bool
		simulateGetClusterError       bool
		simulateClusterNotReady       bool
		simulateGetServingCertError   bool
		simulateGetIssuerError        bool
		simulateIssuerNotReady        bool
		simulateNewClusterUserCertErr bool
		simulateUserCertCleanupErr    bool
		simulateBTICreateError        bool
		simulateBTICleanupError       bool
		simulateGRPCClientErr         bool
		simulateSyncFilesErr          bool
		simulateRestoreErr            bool
	}{
		{
			desc: "success - no options set",
		},
		{
			desc: "success - all options set",
			restoreOptions: VaultWardenRestoreOptions{
				Certificates: vaultWardenRestoreOptionsCertificates{
					PostgresUserCert: vaultWardenRestoreOptionsClusterUserCert{
						Subject: &certmanagerv1.X509Subject{
							Organizations: []string{"test-org"},
						},
						WaitForReadyTimeout: helpers.MaxWaitTime(2 * time.Second),
					},
				},
				CleanupTimeout: helpers.MaxWaitTime(3 * time.Second),
			},
		},
		{
			desc:                  "error getting DR PVC",
			simulateGetDRPVCError: true,
		},
		{
			desc:                    "error getting data PVC",
			simulateGetDataPVCError: true,
		},
		{
			desc:                    "error getting cluster",
			simulateGetClusterError: true,
		},
		{
			desc:                    "cluster not ready",
			simulateClusterNotReady: true,
		},
		{
			desc:                        "error getting serving cert",
			simulateGetServingCertError: true,
		},
		{
			desc:                   "error getting issuer",
			simulateGetIssuerError: true,
		},
		{
			desc:                   "issuer not ready",
			simulateIssuerNotReady: true,
		},
		{
			desc:                          "error creating cluster user cert",
			simulateNewClusterUserCertErr: true,
		},
		{
			desc:                       "error cleaning up user cert",
			simulateUserCertCleanupErr: true,
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
			desc:                 "error syncing files",
			simulateSyncFilesErr: true,
		},
		{
			desc:               "error restoring database",
			simulateRestoreErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)
			mockCNPGClient := cnpg.NewMockClientInterface(t)
			mockCMClient := certmanager.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()
			mockClient.EXPECT().CNPG().Return(mockCNPGClient).Maybe()
			mockClient.EXPECT().CM().Return(mockCMClient).Maybe()

			vw := &VaultWarden{
				kubernetesClient: mockClient,
			}

			rootCtx := th.NewTestContext()

			drPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
			}

			dataPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataPVCName,
					Namespace: namespace,
				},
			}

			cluster := &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					WriteService: writeServiceName,
					Conditions: []metav1.Condition{
						{
							Type:   string(apiv1.ConditionClusterReady),
							Status: metav1.ConditionStatus(apiv1.ConditionTrue),
						},
					},
				},
			}

			servingCert := &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      servingCertName,
					Namespace: namespace,
				},
			}

			clientIssuer := &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clientIssuerName,
					Namespace: namespace,
				},
				Status: certmanagerv1.IssuerStatus{
					Conditions: []certmanagerv1.IssuerCondition{
						{
							Type:   certmanagerv1.IssuerConditionReady,
							Status: cmmeta.ConditionTrue,
						},
					},
				},
			}

			wantErr := th.ErrExpected(
				tt.simulateGetDRPVCError,
				tt.simulateGetDataPVCError,
				tt.simulateGetClusterError,
				tt.simulateClusterNotReady,
				tt.simulateGetServingCertError,
				tt.simulateGetIssuerError,
				tt.simulateIssuerNotReady,
				tt.simulateNewClusterUserCertErr,
				tt.simulateUserCertCleanupErr,
				tt.simulateBTICreateError,
				tt.simulateBTICleanupError,
				tt.simulateGRPCClientErr,
				tt.simulateSyncFilesErr,
				tt.simulateRestoreErr,
			)

			// Setup mocks
			func() {
				// Step 0: Resource validation
				mockCoreClient.EXPECT().GetPVC(mock.Anything, namespace, restoreName).
					Return(th.ErrOr1Val(drPVC, tt.simulateGetDRPVCError))
				if tt.simulateGetDRPVCError {
					return
				}

				mockCoreClient.EXPECT().GetPVC(mock.Anything, namespace, dataPVCName).
					Return(th.ErrOr1Val(dataPVC, tt.simulateGetDataPVCError))
				if tt.simulateGetDataPVCError {
					return
				}

				mockCNPGClient.EXPECT().GetCluster(mock.Anything, namespace, clusterName).
					Return(th.ErrOr1Val(cluster, tt.simulateGetClusterError))
				if tt.simulateGetClusterError {
					return
				}
				if tt.simulateClusterNotReady {
					cluster.Status.Conditions[0].Status = metav1.ConditionStatus(apiv1.ConditionFalse)
					return
				}

				mockCMClient.EXPECT().GetCertificate(mock.Anything, namespace, servingCertName).
					Return(th.ErrOr1Val(servingCert, tt.simulateGetServingCertError))
				if tt.simulateGetServingCertError {
					return
				}

				mockCMClient.EXPECT().GetIssuer(mock.Anything, namespace, clientIssuerName).
					Return(th.ErrOr1Val(clientIssuer, tt.simulateGetIssuerError))
				if tt.simulateGetIssuerError {
					return
				}
				if tt.simulateIssuerNotReady {
					clientIssuer.Status.Conditions[0].Status = cmmeta.ConditionFalse
					return
				}

				// Step 1: Create postgres user cert
				userCert := clusterusercert.NewMockClusterUserCertInterface(t)
				mockClient.EXPECT().NewClusterUserCert(mock.Anything, namespace, "postgres", clientIssuerName, clusterName, mock.Anything).
					Return(th.ErrOr1Val(userCert, tt.simulateNewClusterUserCertErr))
				if tt.simulateNewClusterUserCertErr {
					return
				}

				userCert.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
					require.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateUserCertCleanupErr)
				})

				// Step 2: Create backup tool instance
				userCert.EXPECT().GetCertificate().Return(servingCert)
				btInstance := backuptoolinstance.NewMockBackupToolInstanceInterface(t)
				mockClient.EXPECT().CreateBackupToolInstance(mock.Anything, namespace, mock.Anything, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, instance string, opts backuptoolinstance.CreateBackupToolInstanceOptions) (backuptoolinstance.BackupToolInstanceInterface, error) {
						assert.True(t, calledCtx.IsChildOf(rootCtx))
						assert.Contains(t, opts.NamePrefix, constants.ToolName)
						assert.Len(t, opts.Volumes, 4)
						return th.ErrOr1Val(btInstance, tt.simulateBTICreateError)
					})
				if tt.simulateBTICreateError {
					return
				}

				btInstance.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx *contexts.Context) error {
					require.NotEqual(t, rootCtx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateBTICleanupError)
				})

				// Step 3: Setup GRPC client and sync files
				mockGRPCClient := clients.NewMockClientInterface(t)
				btInstance.EXPECT().GetGRPCClient(mock.Anything).
					Return(th.ErrOr1Val(mockGRPCClient, tt.simulateGRPCClientErr))
				if tt.simulateGRPCClientErr {
					return
				}

				mockFilesRuntime := files.NewMockRuntime(t)
				mockGRPCClient.EXPECT().Files().Return(mockFilesRuntime)
				mockFilesRuntime.EXPECT().SyncFiles(mock.Anything, mock.Anything, mock.Anything).
					Return(th.ErrIfTrue(tt.simulateSyncFilesErr))
				if tt.simulateSyncFilesErr {
					return
				}

				// Step 4: Restore database
				mockPostgresRuntime := postgres.NewMockRuntime(t)
				mockGRPCClient.EXPECT().Postgres().Return(mockPostgresRuntime)
				mockPostgresRuntime.EXPECT().Restore(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(th.ErrIfTrue(tt.simulateRestoreErr))
			}()

			restore, err := vw.Restore(rootCtx, namespace, restoreName, dataPVCName,
				clusterName, servingCertName, clientIssuerName, tt.restoreOptions)

			require.NotNil(t, restore)
			assert.NotEmpty(t, restore.StartTime)
			assert.NotEmpty(t, restore.EndTime)

			if wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
