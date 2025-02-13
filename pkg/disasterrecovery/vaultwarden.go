package disasterrecovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const baseMountPath = string(os.PathSeparator) + "mnt"

type VaultWardenBackupOptionsBackupSnapshot struct {
	ReadyTimeout  helpers.MaxWaitTime `yaml:"snapshotReadyTimeout,omitempty"`
	SnapshotClass string              `yaml:"snapshotClass,omitempty"`
}

// TODO plumb a lot more options through to here
type VaultWardenBackupOptions struct {
	VolumeSize                   resource.Quantity                                  `yaml:"volumeSize,omitempty"`
	VolumeStorageClass           string                                             `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions          clonedcluster.CloneClusterOptions                  `yaml:"clusterCloning,omitempty"`
	BackupToolPodCreationTimeout helpers.MaxWaitTime                                `yaml:"backupToolPodCreationTimeout,omitempty"`
	BackupSnapshot               VaultWardenBackupOptionsBackupSnapshot             `yaml:"backupSnapshot,omitempty"`
	RemoteBackupToolOptions      backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains  []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
	CleanupTimeout               helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
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
func (vw *VaultWarden) Backup(ctx *contexts.Context, namespace, backupName, dataPVC, cnpgClusterName, servingCertIssuerName, clientCertIssuerName string, backupOptions VaultWardenBackupOptions) (backup *Backup, err error) {
	backup = NewBackupNow(backupName)
	ctx.Log.With("backupName", backup.GetFullName(), "namespace", namespace).Info("Starting backup process")
	defer func() {
		backup.Stop()
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Backup process failed", keyvals...)
		} else {
			ctx.Log.Info("Backup process completed", keyvals...)
		}
	}()

	// 1. Snapshot/clone PVC containing data directory
	ctx.Log.Step().Info("Cloning data PVC")
	clonedPVC, err := vw.kubernetesClient.ClonePVC(ctx.Child(), namespace, dataPVC, clonepvc.ClonePVCOptions{DestPvcNamePrefix: backup.GetFullName(), CleanupTimeout: backupOptions.CleanupTimeout, ForceBind: true})
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone data PVC")
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		return vw.kubernetesClient.Core().DeletePVC(ctx, namespace, clonedPVC.Name)
	}).WithErrMessage("failed to cleanup PVC %q", helpers.FullName(clonedPVC)).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 2. Create the DR PVC if not exists
	ctx.Log.Step().Info("Ensuring DR PVC exists")
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

	drPVC, err := vw.kubernetesClient.Core().EnsurePVCExists(ctx.Child(), namespace, backup.Name, drVolumeSize, core.CreatePVCOptions{StorageClassName: backupOptions.VolumeStorageClass})
	if err != nil {
		return backup, trace.Wrap(err, "failed to ensure backup volume exists")
	}

	// 3. Clone the CNPG cluster with PITR set to the PVC snapshot time
	ctx.Log.Step().Info("Cloning CNPG cluster")
	clonedClusterName := helpers.CleanName(fmt.Sprintf("%s-%s", cnpgClusterName, backup.GetFullName()))

	if backupOptions.CloneClusterOptions.CleanupTimeout == 0 {
		backupOptions.CloneClusterOptions.CleanupTimeout = backupOptions.CleanupTimeout
	}

	if backupOptions.CloneClusterOptions.RecoveryTargetTime == "" {
		backupOptions.CloneClusterOptions.RecoveryTargetTime = clonedPVC.CreationTimestamp.Format(time.RFC3339)
	}

	clonedCluster, err := vw.kubernetesClient.CloneCluster(ctx.Child(), namespace, cnpgClusterName,
		clonedClusterName, servingCertIssuerName, clientCertIssuerName,
		backupOptions.CloneClusterOptions)
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone cluster %q", cnpgClusterName)
	}
	defer cleanup.To(clonedCluster.Delete).WithErrMessage("failed to cleanup cloned cluster %q resources", clonedClusterName).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(10 * time.Minute)).Run()

	// 4. Spawn a new tool instance with the cloned PVC attached, and DR mount and secrets attached
	ctx.Log.Step().Info("Creating backup tool instance")
	drVolumeMountPath := filepath.Join(baseMountPath, "dr")
	clonedVolumeMountPath := filepath.Join(baseMountPath, "data")
	secretsVolumeMountPath := filepath.Join(baseMountPath, "secrets")
	servingCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "serving-cert")
	clientCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "client-cert")
	btOpts := backuptoolinstance.CreateBackupToolInstanceOptions{
		NamePrefix: fmt.Sprintf("%s-%s", constants.ToolName, backup.GetFullName()),
		Volumes: []core.SingleContainerVolume{
			core.NewSingleContainerPVC(drPVC.Name, drVolumeMountPath),
			core.NewSingleContainerPVC(clonedPVC.Name, clonedVolumeMountPath),
			core.NewSingleContainerSecret(clonedCluster.GetServingCert().Name, servingCertVolumeMountPath, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
			core.NewSingleContainerSecret(clonedCluster.GetPostgresUserCert().GetCertificate().Name, clientCertVolumeMountPath),
		},
		CleanupTimeout: backupOptions.CleanupTimeout,
	}
	mergo.MergeWithOverwrite(&btOpts, backupOptions.RemoteBackupToolOptions)
	btInstance, err := vw.kubernetesClient.CreateBackupToolInstance(ctx.Child(), namespace, backup.GetFullName(), btOpts)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		return btInstance.Delete(ctx)
	}).WithErrMessage("failed to cleanup backup tool instance %q resources", backup.GetFullName()).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 5. Sync the data directory to the DR volume
	ctx.Log.Step().Info("Syncing data directory to DR volume")
	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child(), backupOptions.ClusterServiceSearchDomains...)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	drDataVolPath := filepath.Join(drVolumeMountPath, "data-vol")
	err = backupToolClient.Files().SyncFiles(ctx.Child(), clonedVolumeMountPath, drDataVolPath)
	if err != nil {
		return backup, trace.Wrap(err, "failed to sync data directory files at %q to the disaster recovery volume at %q", clonedVolumeMountPath, drDataVolPath)
	}

	// 6. Perform a CNPG logical backup
	ctx.Log.Step().Info("Performing Postgres logical backup")
	podSQLFilePath := filepath.Join(drVolumeMountPath, "dump.sql")
	clusterCredentials := clonedCluster.GetCredentials(servingCertVolumeMountPath, clientCertVolumeMountPath)
	err = backupToolClient.Postgres().DumpAll(ctx.Child(), clusterCredentials, podSQLFilePath, postgres.DumpAllOptions{CleanupTimeout: backupOptions.CleanupTimeout})
	if err != nil {
		return backup, trace.Wrap(err, "failed to dump logical backup for postgres server at %q", postgres.GetServerAddress(clusterCredentials))
	}

	// 7. Snapshot the backup PVC
	ctx.Log.Step().Info("Snapshotting the DR volume")
	snapshot, err := vw.kubernetesClient.ES().SnapshotVolume(ctx.Child(), namespace, drPVC.Name, externalsnapshotter.SnapshotVolumeOptions{Name: helpers.CleanName(backup.GetFullName()), SnapshotClass: backupOptions.BackupSnapshot.SnapshotClass})
	if err != nil {
		return backup, trace.Wrap(err, "failed to snapshot backup volume %q", helpers.FullName(drPVC))
	}

	_, err = vw.kubernetesClient.ES().WaitForReadySnapshot(ctx.Child(), namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: backupOptions.BackupSnapshot.ReadyTimeout})
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
