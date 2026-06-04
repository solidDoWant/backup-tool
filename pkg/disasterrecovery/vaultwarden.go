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
	cnpgbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/backup"
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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	vaultwardenBaseMountPath = string(os.PathSeparator) + "mnt"
	vaultwardenDRVolPath     = "data-vol" // Important: changing this is will break restoration of old backups!
	vaultwardenSQLFileName   = "dump.sql" // Important: changing this is will break restoration of old backups!
)

// TODO plumb a lot more options through to here
type VaultWardenBackupOptions struct {
	VolumeSize                   resource.Quantity                                  `yaml:"volumeSize,omitempty"`
	VolumeStorageClass           string                                             `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions          clonedcluster.CloneClusterOptions                  `yaml:"clusterCloning,omitempty"`
	BackupToolPodCreationTimeout helpers.MaxWaitTime                                `yaml:"backupToolPodCreationTimeout,omitempty"`
	BackupSnapshot               OptionsBackupSnapshot                              `yaml:"backupSnapshot,omitempty"`
	RemoteBackupToolOptions      backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
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
// 1. Take the CNPG base backup (establishes the DB consistency point, before the other captures)
// 2. Snapshot/clone the PVC containing the data directory (its clone time is the recovery target T_dr)
// 3. Create the DR PVC if not exists
// 4. Create the cloned CNPG cluster, recovering forward to T_dr (idle source falls back to the consistency point)
// 5. Spawn a tool instance with the cloned PVC, cloned-cluster certs, and DR mount attached
// 6. Sync the data directory to the DR volume
// 7. Perform a CNPG logical backup (pg_dumpall) to the DR mount
// 8. Take a snapshot of the DR volume
// 9. Exit the tool instance, delete all created resources except for the DR volume snapshot
func (vw *VaultWarden) Backup(ctx *contexts.Context, namespace, backupName, dataPVC, cnpgClusterName, servingCertIssuerName, clientCertIssuerName string, backupOptions VaultWardenBackupOptions) (backup *DREvent, err error) {
	backup = NewDREventNow(backupName)
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

	if backupOptions.CloneClusterOptions.CleanupTimeout == 0 {
		backupOptions.CloneClusterOptions.CleanupTimeout = backupOptions.CleanupTimeout
	}

	// 1. Take the CNPG base backup first. Postgres can only be recovered forward from its base
	// backup, so for cross-resource PITR the base backup must be the earliest capture in the event:
	// its consistency point then precedes the filesystem/DR captures below, and the clone can be
	// recovered forward from it to line up with them.
	ctx.Log.Step().Info("Backing up the CNPG cluster")
	cnpgBackup, err := vw.kubernetesClient.CreateClusterBackup(ctx.Child(), namespace, cnpgClusterName, backupOptions.CloneClusterOptions)
	if err != nil {
		return backup, trace.Wrap(err, "failed to back up cluster %q", cnpgClusterName)
	}
	// The backup, and the volume snapshots it owns, must outlive the clone recovery below, so it is
	// only deleted at the very end of the event.
	defer cleanup.To(func(ctx *contexts.Context) error {
		return vw.kubernetesClient.CNPG().DeleteBackup(ctx, namespace, cnpgBackup.Name)
	}).WithErrMessage("failed to cleanup CNPG backup %q", helpers.FullNameStr(namespace, cnpgBackup.Name)).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 2. Snapshot/clone the PVC containing the data directory. The clone freezes the filesystem; its
	// creation time is T_dr, the instant the DB is recovered forward to so it aligns with the frozen
	// filesystem.
	ctx.Log.Step().Info("Cloning data PVC")
	clonedPVC, err := vw.kubernetesClient.ClonePVC(ctx.Child(), namespace, dataPVC, clonepvc.ClonePVCOptions{DestPvcNamePrefix: backup.GetFullName(), CleanupTimeout: backupOptions.CleanupTimeout, ForceBind: true})
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone data PVC")
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		return vw.kubernetesClient.Core().DeletePVC(ctx, namespace, clonedPVC.Name)
	}).WithErrMessage("failed to cleanup PVC %q", helpers.FullName(clonedPVC)).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 3. Create the DR PVC if not exists
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

	// 4. Create the cloned CNPG cluster from the base backup, recovering forward to the data PVC
	// clone time (T_dr) so the database lines up with the frozen filesystem.
	ctx.Log.Step().Info("Cloning CNPG cluster")
	backupOptions.CloneClusterOptions.RecoveryTargetTime = clonedPVC.CreationTimestamp.Format(time.RFC3339)

	// 4a. Write the source recovery fence after T_dr is fixed and before the clone, so the clone's
	// forward recovery to T_dr can reach it. A no-op for non-plugin sources. See ForceSourceWALArchive.
	ctx.Log.Step().Info("Forcing source WAL archive")
	if err := cnpgbackup.ForceSourceWALArchive(ctx.Child(), vw.kubernetesClient, namespace, cnpgClusterName); err != nil {
		return backup, trace.Wrap(err, "failed to force source WAL archive for cluster %q", cnpgClusterName)
	}

	// Try and come up with the most useful name for the cloned cluster fitting CNPG requirements.
	// More info is better, but it needs to at least convey the backup name and still be readable.
	clonedClusterName := helpers.CleanName(fmt.Sprintf("%s-%s", cnpgClusterName, backup.GetFullName()))
	if len(clonedClusterName) > 40 { // Max length that CNPG allows for cloned cluster names, see https://github.com/cloudnative-pg/cloudnative-pg/pull/6755
		clonedClusterName = helpers.CleanName(helpers.TruncateString(backup.GetFullName(), 40, ""))
	}

	clonedCluster, err := vw.kubernetesClient.CloneClusterFromBackup(ctx.Child(), namespace, cnpgClusterName, clonedClusterName, servingCertIssuerName, clientCertIssuerName, cnpgBackup, backupOptions.CloneClusterOptions)
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone cluster %q", cnpgClusterName)
	}
	defer cleanup.To(clonedCluster.Delete).WithErrMessage("failed to cleanup cloned cluster %q resources", clonedClusterName).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(10 * time.Minute)).Run()

	// 5. Spawn a new tool instance with the cloned PVC attached, and DR mount and secrets attached
	ctx.Log.Step().Info("Creating backup tool instance")
	drVolumeMountPath := filepath.Join(vaultwardenBaseMountPath, "dr")
	clonedVolumeMountPath := filepath.Join(vaultwardenBaseMountPath, "data")
	secretsVolumeMountPath := filepath.Join(vaultwardenBaseMountPath, "secrets")
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

	if err := mergo.Merge(&btOpts, backupOptions.RemoteBackupToolOptions, mergo.WithOverride); err != nil {
		return backup, trace.Wrap(err, "failed to merge backup tool options with defaults")
	}

	btInstance, err := vw.kubernetesClient.CreateBackupToolInstance(ctx.Child(), namespace, backup.GetFullName(), btOpts)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", backup.GetFullName()).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 6. Sync the data directory to the DR volume
	ctx.Log.Step().Info("Syncing data directory to DR volume")
	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child())
	if err != nil {
		return backup, trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	drDataVolPath := filepath.Join(drVolumeMountPath, vaultwardenDRVolPath)
	err = backupToolClient.Files().SyncFiles(ctx.Child(), clonedVolumeMountPath, drDataVolPath)
	if err != nil {
		return backup, trace.Wrap(err, "failed to sync data directory files at %q to the disaster recovery volume at %q", clonedVolumeMountPath, drDataVolPath)
	}

	// 7. Perform a CNPG logical backup
	ctx.Log.Step().Info("Performing Postgres logical backup")
	podSQLFilePath := filepath.Join(drVolumeMountPath, vaultwardenSQLFileName)
	clusterCredentials := clonedCluster.GetCredentials(servingCertVolumeMountPath, clientCertVolumeMountPath)
	err = backupToolClient.Postgres().DumpAll(ctx.Child(), clusterCredentials, podSQLFilePath, postgres.DumpAllOptions{CleanupTimeout: backupOptions.CleanupTimeout})
	if err != nil {
		return backup, trace.Wrap(err, "failed to dump logical backup for postgres server at %q", postgres.GetServerAddress(clusterCredentials))
	}

	// 8. Snapshot the backup PVC
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

type vaultWardenRestoreOptionsCertificates struct {
	PostgresUserCert OptionsClusterUserCert `yaml:"postgresUserCert,omitempty"`
}

type VaultWardenRestoreOptions struct {
	Certificates            vaultWardenRestoreOptionsCertificates              `yaml:"certificates,omitempty"`
	CleanupTimeout          helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
	RemoteBackupToolOptions backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
}

// Restore requirements:
// * The DR PVC must exist
// * Data PVC must already exist, but not be in use
// * Replacement cluster must be already deployed
// * The CNPG cluster must already exist, but not be in use
// * The CNPG client CA issuer must already exist
// * The CNPG cluster must support TLS auth for the postgres user
// * The CNPG cluster serving cert must already exist
// Restore process:
// 1. Ensure that the provided resources exist and are ready
// 2. Spawn a new backup-tool pod with data directory PVC attached, and DR mount attached
// 3. Sync the data files from the DR mount to the data directory PVC
// 4. Perform a CNPG logical recovery
// 5. Exit the backup-tool pod
func (vw *VaultWarden) Restore(ctx *contexts.Context, namespace, restoreName, dataPVCName, cnpgClusterName, servingCertName, clientCertIssuerName string, opts VaultWardenRestoreOptions) (restore *DREvent, err error) {
	restore = NewDREventNow(restoreName)
	ctx.Log.With("restoreName", restore.GetFullName(), "namespace", namespace).Info("Starting restore process")
	defer func() {
		restore.Stop()
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Restore process failed", keyvals...)
		} else {
			ctx.Log.Info("Restore process completed", keyvals...)
		}
	}()

	// 1. Ensure the require resources already exist
	ctx.Log.Step().Info("Ensuring required resources exist")
	drPVC, err := vw.kubernetesClient.Core().GetPVC(ctx.Child(), namespace, restoreName)
	if err != nil {
		return restore, trace.Wrap(err, "failed to get DR PVC %q", restoreName)
	}

	dataPVC, err := vw.kubernetesClient.Core().GetPVC(ctx.Child(), namespace, dataPVCName)
	if err != nil {
		return restore, trace.Wrap(err, "failed to get data PVC %q", dataPVCName)
	}

	cluster, err := vw.kubernetesClient.CNPG().GetCluster(ctx.Child(), namespace, cnpgClusterName)
	if err != nil {
		return restore, trace.Wrap(err, "failed to get CNPG cluster %q", cnpgClusterName)
	}
	if !cnpg.IsClusterReady(cluster) {
		return restore, trace.Errorf("CNPG cluster %q is not ready", cnpgClusterName)
	}

	servingCert, err := vw.kubernetesClient.CM().GetCertificate(ctx.Child(), namespace, servingCertName)
	if err != nil {
		return restore, trace.Wrap(err, "failed to get CNPG cluster serving cert %q", servingCertName)
	}

	clientCertIssuer, err := vw.kubernetesClient.CM().GetIssuer(ctx.Child(), namespace, clientCertIssuerName)
	if err != nil {
		return restore, trace.Wrap(err, "failed to get CNPG cluster client cert issuer %q", clientCertIssuerName)
	}
	if !certmanager.IsIssuerReady(&clientCertIssuer.Status) {
		return restore, trace.Errorf("CNPG cluster client cert issuer %q is not ready", clientCertIssuerName)
	}

	// 2. Create the postgres user cert
	ctx.Log.Step().Info("Creating CNPG cluster client cert")
	cucOptions := clusterusercert.NewClusterUserCertOpts{
		Subject:            opts.Certificates.PostgresUserCert.Subject,
		CRPOpts:            opts.Certificates.PostgresUserCert.CRPOpts,
		WaitForCertTimeout: opts.Certificates.PostgresUserCert.WaitForReadyTimeout,
		CleanupTimeout:     opts.CleanupTimeout,
	}
	postgresUserCert, err := vw.kubernetesClient.NewClusterUserCert(ctx.Child(), namespace, "postgres", clientCertIssuerName, cnpgClusterName, cucOptions)
	if err != nil {
		return restore, trace.Wrap(err, "failed to create postgres user CNPG cluster client cert")
	}
	defer cleanup.To(postgresUserCert.Delete).WithErrMessage("failed to cleanup postgres user CNPG cluster client cert resources").WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 3. Spawn a new backup-tool pod with data directory PVC attached, and DR mount attached
	ctx.Log.Step().Info("Creating backup tool instance")
	drVolumeMountPath := filepath.Join(vaultwardenBaseMountPath, "dr")
	dataVolumeMountPath := filepath.Join(vaultwardenBaseMountPath, "data")
	secretsVolumeMountPath := filepath.Join(vaultwardenBaseMountPath, "secrets")
	servingCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "serving-cert")
	clientCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "client-cert")
	btOpts := backuptoolinstance.CreateBackupToolInstanceOptions{
		NamePrefix: fmt.Sprintf("%s-%s", constants.ToolName, restore.GetFullName()),
		Volumes: []core.SingleContainerVolume{
			core.NewSingleContainerPVC(drPVC.Name, drVolumeMountPath),
			core.NewSingleContainerPVC(dataPVC.Name, dataVolumeMountPath),
			core.NewSingleContainerSecret(servingCert.Name, servingCertVolumeMountPath, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
			core.NewSingleContainerSecret(postgresUserCert.GetCertificate().Name, clientCertVolumeMountPath),
		},
		CleanupTimeout: opts.CleanupTimeout,
	}

	if err := mergo.Merge(&btOpts, opts.RemoteBackupToolOptions, mergo.WithOverride); err != nil {
		return restore, trace.Wrap(err, "failed to merge backup tool options with defaults")
	}

	btInstance, err := vw.kubernetesClient.CreateBackupToolInstance(ctx.Child(), namespace, restore.GetFullName(), btOpts)
	if err != nil {
		return restore, trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", restore.GetFullName()).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 4. Sync the data files from the DR mount to the data directory PVC
	ctx.Log.Step().Info("Syncing data directory to data PVC")
	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child())
	if err != nil {
		return restore, trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	drDataVolPath := filepath.Join(drVolumeMountPath, vaultwardenDRVolPath)
	err = backupToolClient.Files().SyncFiles(ctx.Child(), drDataVolPath, dataVolumeMountPath)
	if err != nil {
		return restore, trace.Wrap(err, "failed to sync data directory files at %q to the data PVC at %q", drDataVolPath, dataVolumeMountPath)
	}

	// 5. Perform a CNPG logical recovery
	ctx.Log.Step().Info("Performing Postgres logical recovery")
	podSQLFilePath := filepath.Join(drVolumeMountPath, vaultwardenSQLFileName)
	clusterCredentials := &postgres.EnvironmentCredentials{
		postgres.HostVarName:        fmt.Sprintf("%s.%s.svc", cluster.Status.WriteService, namespace),
		postgres.UserVarName:        "postgres",
		postgres.RequireAuthVarName: "none",        // Require TLS auth. Don't allow the server to ask the client for a password/similar.
		postgres.SSLModeVarName:     "verify-full", // Check the server hostname against the cert, and validate the cert chain
		postgres.SSLCertVarName:     filepath.Join(clientCertVolumeMountPath, "tls.crt"),
		postgres.SSLKeyVarName:      filepath.Join(clientCertVolumeMountPath, "tls.key"),
		postgres.SSLRootCertVarName: filepath.Join(servingCertVolumeMountPath, "tls.crt"),
	}
	err = backupToolClient.Postgres().Restore(ctx.Child(), clusterCredentials, podSQLFilePath, postgres.RestoreOptions{})
	if err != nil {
		return restore, trace.Wrap(err, "failed to restore logical backup for postgres server at %q", postgres.GetServerAddress(clusterCredentials))
	}

	return restore, nil
}
