package disasterrecovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const baseMountPath = string(os.PathListSeparator) + "mnt"

// TODO plumb a lot more options through to here
type VaultWardenBackupOptions struct {
	VolumeSize                   resource.Quantity                           `yaml:"volumeSize,omitempty"`
	VolumeStorageClass           string                                      `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions          kubecluster.CloneClusterOptions             `yaml:"clusterCloning,omitempty"`
	BackupToolPodCreationTimeout helpers.MaxWaitTime                         `yaml:"backupToolPodCreationTimeout,omitempty"`
	SnapshotReadyTimeout         helpers.MaxWaitTime                         `yaml:"snapshotReadyTimeout,omitempty"`
	RemoteBackupToolOptions      kubecluster.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains  []string                                    `yaml:"clusterServiceSearchDomains,omitempty"`
	CleanupTimeout               helpers.MaxWaitTime                         `yaml:"cleanupTimeout,omitempty"`
}

type VaultWarden struct {
	kubernetesClient kubecluster.ClientInterface
}

func NewVaultWarden(client kubecluster.ClientInterface) *VaultWarden {
	return &VaultWarden{
		kubernetesClient: client,
	}
}

// Backup process:
// 1. Create the DR PVC if not exists
// 2. Snapshot/clone PVC containing data directory
// 3. Spawn a new tool instance with the cloned PVC attached, and DR mount attached
// 4. Sync the data directory to the DR volume
// 5. Perform a CNPG logical backup with PITR set to the PVC snapshot time
// 6. Copy the logical backup to the DR mount
// 7. Take a snapshot of the DR volume
// 8. Exit the tool instance, delete all created resources except for DR volume snapshot
func (vw *VaultWarden) Backup(ctx context.Context, namespace, backupName, dataPVC, cnpgClusterName, servingCertIssuerName, clientCertIssuerName string, backupOptions VaultWardenBackupOptions) (backup *Backup, err error) {
	backup = NewBackupNow(backupName)
	defer backup.Stop()

	// 1. Snapshot/clone PVC containing data directory
	clonedPVC, err := vw.kubernetesClient.ClonePVC(ctx, namespace, dataPVC, kubecluster.ClonePVCOptions{DestPvcNamePrefix: backup.GetFullName(), CleanupTimeout: backupOptions.CleanupTimeout})
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone data PVC")
	}
	defer cleanup.WithTimeoutTo(backupOptions.CleanupTimeout.MaxWait(time.Minute), func(ctx context.Context) error {
		return vw.kubernetesClient.Core().DeleteVolume(ctx, namespace, clonedPVC.Name)
	}).WithErrMessage("failed to cleanup PVC %q", helpers.FullName(clonedPVC)).WithOriginalErr(&err).Run()

	// 2. Create the DR PVC if not exists
	drVolumeSize := backupOptions.VolumeSize
	if drVolumeSize.IsZero() {
		snapshotSize, ok := clonedPVC.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			return backup, trace.Wrap(err, "failed to get the size of the cloned PVC")
		}
		drVolumeSize = snapshotSize
		// Default to roughly twice the initial snapshot size
		drVolumeSize.Mul(2)
	}

	drPVC, err := vw.kubernetesClient.Core().EnsurePVCExists(ctx, namespace, backup.Name, drVolumeSize, core.CreatePVCOptions{StorageClassName: backupOptions.VolumeStorageClass})
	if err != nil {
		return backup, trace.Wrap(err, "failed to ensure backup volume exists")
	}

	// 3. Clone the CNPG cluster
	clonedClusterName := helpers.CleanName(fmt.Sprintf("%s-%s", cnpgClusterName, backup.GetFullName()))
	if backupOptions.CloneClusterOptions.CleanupTimeout == 0 {
		backupOptions.CloneClusterOptions.CleanupTimeout = backupOptions.CleanupTimeout
	}
	clonedCluster, err := vw.kubernetesClient.CloneCluster(ctx, namespace, cnpgClusterName,
		clonedClusterName, servingCertIssuerName, clientCertIssuerName,
		backupOptions.CloneClusterOptions)
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone cluster %q", cnpgClusterName)
	}
	defer cleanup.WithTimeoutTo(backupOptions.CleanupTimeout.MaxWait(10*time.Minute), clonedCluster.Delete).WithErrMessage("failed to cleanup cloned cluster %q resources", clonedClusterName).WithOriginalErr(&err).Run()

	// 4. Spawn a new tool instance with the cloned PVC attached, and DR mount and secrets attached
	drVolumeMountPath := filepath.Join(baseMountPath, "dr")
	clonedVolumeMountPath := filepath.Join(baseMountPath, "data")
	secretsVolumeMountPath := filepath.Join(baseMountPath, "secrets")
	servingCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "serving-cert")
	clientCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "client-cert")
	btOpts := kubecluster.CreateBackupToolInstanceOptions{
		NamePrefix: backup.GetFullName(),
		Volumes: []kubecluster.SingleContainerVolume{
			kubecluster.NewSingleContainerPVC(drPVC.Name, drVolumeMountPath),
			kubecluster.NewSingleContainerPVC(clonedPVC.Name, clonedVolumeMountPath),
			kubecluster.NewSingleContainerSecret(clonedCluster.GetServingCert().Name, servingCertVolumeMountPath),
			kubecluster.NewSingleContainerSecret(clonedCluster.GetClientCert().Name, clientCertVolumeMountPath),
		},
		CleanupTimeout: backupOptions.CleanupTimeout,
	}
	mergo.MergeWithOverwrite(&btOpts, backupOptions.RemoteBackupToolOptions)
	btInstance, err := vw.kubernetesClient.CreateBackupToolInstance(ctx, namespace, backup.GetFullName(), btOpts)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.WithTimeoutTo(backupOptions.CleanupTimeout.MaxWait(time.Minute), func(ctx context.Context) error {
		return btInstance.Delete(ctx)
	}).WithErrMessage("failed to cleanup backup tool instance %q resources", backup.GetFullName()).
		WithOriginalErr(&err).Run()

	// 5. Sync the data directory to the DR volume
	backupToolClient, err := btInstance.GetGRPCClient(ctx, backupOptions.ClusterServiceSearchDomains...)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	err = backupToolClient.Files().SyncFiles(ctx, clonedVolumeMountPath, drVolumeMountPath)
	if err != nil {
		return backup, trace.Wrap(err, "failed to sync data directory files at %q to the disaster recovery volume at %q", clonedVolumeMountPath, drVolumeMountPath)
	}

	// 6. Perform a CNPG logical backup with PITR set to the PVC snapshot time
	podSQLFilePath := filepath.Join(drVolumeMountPath, "dump.sql")
	clusterCredentials := clonedCluster.GetCredentials(servingCertVolumeMountPath, clientCertVolumeMountPath)
	err = backupToolClient.Postgres().DumpAll(ctx, clusterCredentials, podSQLFilePath, postgres.DumpAllOptions{CleanupTimeout: backupOptions.CleanupTimeout.MaxWait(10 * time.Second)})
	if err != nil {
		return backup, trace.Wrap(err, "failed to dump logical backup for postgres server at %q", postgres.GetServerAddress(clusterCredentials))
	}

	// 7. Snapshot the backup PVC
	snapshot, err := vw.kubernetesClient.ES().SnapshotVolume(ctx, namespace, drPVC.Name, externalsnapshotter.SnapshotVolumeOptions{Name: backup.GetFullName()})
	if err != nil {
		return backup, trace.Wrap(err, "failed to snapshot backup volume %q", helpers.FullName(drPVC))
	}

	_, err = vw.kubernetesClient.ES().WaitForReadySnapshot(ctx, namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: backupOptions.SnapshotReadyTimeout})
	if err != nil {
		return backup, trace.Wrap(err, "failed to wait for backup snapshot %q to become ready", helpers.FullName(snapshot))
	}

	// Backup complete!
	return backup, nil
}

// // Restore requirements:
// // * The DR PVC must exist
// // * Data PVC must already exist, but not in use
// // * Replacement cluster must be already deployed
// // Restore process:
// // 1. Spawn a new backup-tool pod with data directory PVC attached, and DR mount attached
// // 2. Sync the data files from the DR mount to the data directory PVC
// // 3. Perform a CNPG logical recovery
// // 4. Exit the backup-tool pod
// func (vw *VaultWarden) Restore() error {

// }
