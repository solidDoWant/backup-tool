package disasterrecovery

import (
	"context"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
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
				CloneClusterOptions: kubecluster.CloneClusterOptions{
					CleanupTimeout: helpers.MaxWaitTime(5 * time.Second),
				},
				BackupToolPodCreationTimeout: helpers.MaxWaitTime(1 * time.Second),
				SnapshotReadyTimeout:         helpers.MaxWaitTime(2 * time.Second),
				CleanupTimeout:               helpers.MaxWaitTime(3 * time.Second),
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
			clonedCluster := kubecluster.NewMockClonedClusterInterface(t)
			servingCert := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serving-cert",
					Namespace: namespace,
				},
			}
			clientCert := certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-cert",
					Namespace: namespace,
				},
			}
			btInstance := kubecluster.NewMockBackupToolInstanceInterface(t)
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

			ctx := context.Background()

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
				mockClient.EXPECT().ClonePVC(ctx, namespace, dataPVC, mock.Anything).
					RunAndReturn(func(ctx context.Context, namespace, dataPVC string, opts kubecluster.ClonePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						fullBackupName = opts.DestPvcNamePrefix
						require.True(t, strings.HasPrefix(fullBackupName, backupName))
						require.Equal(t, tt.backupOptions.CleanupTimeout, opts.CleanupTimeout)
						return th.ErrOr1Val(clonedPVC, tt.simulateClonePVCError)
					})
				if tt.simulateClonePVCError {
					return
				}
				mockCoreClient.EXPECT().DeleteVolume(mock.Anything, namespace, clonedPVCName).RunAndReturn(func(cleanupCtx context.Context, _, _ string) error {
					require.NotEqual(t, ctx, cleanupCtx)
					return th.ErrIfTrue(tt.simulatePVCCleanupError)
				})

				// Step 2
				mockCoreClient.EXPECT().EnsurePVCExists(ctx, namespace, backupName, mock.Anything, core.CreatePVCOptions{StorageClassName: tt.backupOptions.VolumeStorageClass}).
					RunAndReturn(func(ctx context.Context, namespace, pvcName string, size resource.Quantity, opts core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						require.Equal(t, backupName, pvcName)
						require.GreaterOrEqual(t, size.AsFloat64Slow(), ptr.To(clonedPVC.Spec.Resources.Requests[corev1.ResourceStorage]).AsFloat64Slow())
						return th.ErrOr1Val(clonedPVC, tt.simulateEnsurePVCError)
					})
				if tt.simulateEnsurePVCError {
					return
				}

				// Step 3
				mockClient.EXPECT().CloneCluster(ctx, namespace, clusterName, mock.Anything, servingIssuerName, clientIssuerName, mock.Anything).
					RunAndReturn(func(ctx context.Context, namespace, existingClusterName, newClusterName, servingIssuerName, clientIssuerName string, opts kubecluster.CloneClusterOptions) (kubecluster.ClonedClusterInterface, error) {
						require.True(t, strings.Contains(newClusterName, existingClusterName))
						require.True(t, strings.Contains(newClusterName, fullBackupName))

						return th.ErrOr1Val(clonedCluster, tt.simulateCloneClusterErr)
					})
				if tt.simulateCloneClusterErr {
					return
				}
				clonedCluster.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx context.Context) error {
					require.NotEqual(t, ctx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateCloneClusterCleanupErr)
				})

				// Step 4
				clonedCluster.EXPECT().GetServingCert().Return(&servingCert)
				clonedCluster.EXPECT().GetClientCert().Return(&clientCert)
				mockClient.EXPECT().CreateBackupToolInstance(ctx, namespace, mock.Anything, mock.Anything).
					RunAndReturn(func(ctx context.Context, namespace, instance string, opts kubecluster.CreateBackupToolInstanceOptions) (kubecluster.BackupToolInstanceInterface, error) {
						require.Equal(t, fullBackupName, opts.NamePrefix)
						// TODO add test to ensure that the secrets are attached, along with the DR and cloned data PVCs
						require.Len(t, opts.Volumes, 4)
						return th.ErrOr1Val(btInstance, tt.simulateBTICreateError)
					})
				if tt.simulateBTICreateError {
					return
				}
				btInstance.EXPECT().Delete(mock.Anything).RunAndReturn(func(cleanupCtx context.Context) error {
					require.NotEqual(t, ctx, cleanupCtx)
					return th.ErrIfTrue(tt.simulateBTICleanupError)
				})

				// Step 5
				btInstance.EXPECT().GetGRPCClient(ctx, mock.Anything).RunAndReturn(func(ctx context.Context, searchDomains ...string) (clients.ClientInterface, error) {
					require.Equal(t, tt.backupOptions.ClusterServiceSearchDomains, searchDomains)
					return th.ErrOr1Val(mockGRPCClient, tt.simulateGRPCClientErr)
				})
				if tt.simulateGRPCClientErr {
					return
				}

				mockFilesRuntime.EXPECT().SyncFiles(ctx, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, src, dest string) error {
					require.NotEqual(t, src, dest)
					require.True(t, strings.HasPrefix(src, baseMountPath))
					require.True(t, strings.HasPrefix(dest, baseMountPath))

					return th.ErrIfTrue(tt.simulateSyncFilesErr)
				})
				if tt.simulateSyncFilesErr {
					return
				}

				// Step 6
				clonedCluster.EXPECT().GetCredentials(mock.Anything, mock.Anything).RunAndReturn(func(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials {
					require.NotEqual(t, servingCertMountDirectory, clientCertMountDirectory)
					require.True(t, strings.HasPrefix(servingCertMountDirectory, baseMountPath))
					require.True(t, strings.HasPrefix(clientCertMountDirectory, baseMountPath))

					return credentials
				})
				mockPostgresRuntime.EXPECT().DumpAll(ctx, credentials, mock.Anything, mock.Anything).Return(th.ErrIfTrue(tt.simulateDumpAllErr))
				if tt.simulateDumpAllErr {
					return
				}

				// Step 7
				mockESClient.EXPECT().SnapshotVolume(ctx, namespace, clonedPVCName, mock.Anything).
					RunAndReturn(func(ctx context.Context, namespace, pvcName string, opts externalsnapshotter.SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error) {
						require.Equal(t, fullBackupName, opts.Name)

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

				mockESClient.EXPECT().WaitForReadySnapshot(ctx, namespace, mock.Anything, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: tt.backupOptions.SnapshotReadyTimeout}).
					RunAndReturn(func(ctx context.Context, namespace, snapsnotName string, wfrso externalsnapshotter.WaitForReadySnapshotOpts) error {
						require.Equal(t, fullBackupName, snapsnotName)
						return th.ErrIfTrue(tt.simulateWaitSnapErr)
					})
			}()

			backup, err := vw.Backup(ctx, namespace, backupName, dataPVC, clusterName,
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
