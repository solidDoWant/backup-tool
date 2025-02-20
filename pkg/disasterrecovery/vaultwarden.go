package disasterrecovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
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
	// Try and come up with the most useful name for the cloned cluster fitting CNPG requirements
	// More info is better, but it needs to at least convey the backup name and still be readable
	clonedClusterName := helpers.CleanName(fmt.Sprintf("%s-%s", cnpgClusterName, backup.GetFullName()))
	if len(clonedClusterName) > 50 {
		clonedClusterName = helpers.CleanName(helpers.TruncateString(backup.GetFullName(), 50, ""))
	}

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
	mergo.MergeWithOverwrite(&btOpts, backupOptions.RemoteBackupToolOptions)
	btInstance, err := vw.kubernetesClient.CreateBackupToolInstance(ctx.Child(), namespace, backup.GetFullName(), btOpts)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", backup.GetFullName()).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(backupOptions.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 5. Sync the data directory to the DR volume
	ctx.Log.Step().Info("Syncing data directory to DR volume")
	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child(), backupOptions.ClusterServiceSearchDomains...)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	drDataVolPath := filepath.Join(drVolumeMountPath, vaultwardenDRVolPath)
	err = backupToolClient.Files().SyncFiles(ctx.Child(), clonedVolumeMountPath, drDataVolPath)
	if err != nil {
		return backup, trace.Wrap(err, "failed to sync data directory files at %q to the disaster recovery volume at %q", clonedVolumeMountPath, drDataVolPath)
	}

	// 6. Perform a CNPG logical backup
	ctx.Log.Step().Info("Performing Postgres logical backup")
	podSQLFilePath := filepath.Join(drVolumeMountPath, vaultwardenSQLFileName)
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

type vaultWardenRestoreOptionsClusterUserCert struct {
	Subject             *certmanagerv1.X509Subject                `yaml:"subject,omitempty"`
	WaitForReadyTimeout helpers.MaxWaitTime                       `yaml:"waitForReadyTimeout,omitempty"`
	CRPOpts             clusterusercert.NewClusterUserCertOptsCRP `yaml:"certificateRequestPolicy,omitempty"`
}

type vaultWardenRestoreOptionsCertificates struct {
	PostgresUserCert vaultWardenRestoreOptionsClusterUserCert `yaml:"postgresUserCert,omitempty"`
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
	if !certmanager.IsIssuerReady(clientCertIssuer) {
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
	mergo.MergeWithOverwrite(&btOpts, opts.RemoteBackupToolOptions)
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
