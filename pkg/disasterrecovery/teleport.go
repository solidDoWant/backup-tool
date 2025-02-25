package disasterrecovery

import (
	"os"
	"time"

	"github.com/gravitational/trace"
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
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	teleportBaseMountPath                 = string(os.PathSeparator) + "mnt"
	teleportCoreSQLFileName               = "backup-core.sql"
	teleportAuditSQLFileName              = "backup-audit.sql"
	teleportAuditSessionLogsDirectoryName = "audit-session-logs"
)

type TeleportOptionsS3Sync struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	S3Path  string `yaml:"s3Path,omitempty"`
	// TODO accept values from env, file, or k8s secret
	// TODO if I switch to COSI, remove this and generate a BucketAccess resource instead
	Credentials s3.Credentials `yaml:"credentials,omitempty"`
}

type TeleportOptionsAudit struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Name    string `yaml:"name,omitempty"`
}

type TeleportBackupOptionsAudit struct {
	TeleportOptionsAudit
}

type TeleportBackupOptions struct {
	VolumeSize                  resource.Quantity                                  `yaml:"volumeSize,omitempty"`
	VolumeStorageClass          string                                             `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions         clonedcluster.CloneClusterOptions                  `yaml:"clusterCloning,omitempty"`
	AuditCluster                TeleportBackupOptionsAudit                         `yaml:"auditCluster,omitempty"`
	AuditSessionLogs            TeleportOptionsS3Sync                              `yaml:"auditSessionLogs,omitempty"`
	RemoteBackupToolOptions     backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
	BackupSnapshot              OptionsBackupSnapshot                              `yaml:"backupSnapshot,omitempty"`
	CleanupTimeout              helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
}

type Teleport struct {
	kubeClusterClient kubecluster.ClientInterface
	// Testing injection
	newCNPGBackup  func() cnpgbackup.CNPGBackupInterface
	newCNPGRestore func() cnpgrestore.CNPGRestoreInterface
	newS3Sync      func() s3sync.S3SyncInterface
	newRemoteStage func(kubeClusterClient kubecluster.ClientInterface, namespace, eventName string, opts remote.RemoteStageOptions) remote.RemoteStageInterface
}

func NewTeleport(kubeClusterClient kubecluster.ClientInterface) *Teleport {
	return &Teleport{
		kubeClusterClient: kubeClusterClient,
		newCNPGBackup:     cnpgbackup.NewCNPGBackup,
		newCNPGRestore:    cnpgrestore.NewCNPGRestore,
		newS3Sync:         s3sync.NewS3Sync,
		newRemoteStage:    remote.NewRemoteStage,
	}
}

// Backup process:
// 1. Create the DR PVC if not exists
// 2. Clone the Core cluster
// 3. Clone the Audit cluster (if enabled) with PITR set to the same time as the Core cluster clone
// 4. Deploy a backup-tool instance with access to both the Core and Audit cloned clusters
// 5. Perform a logical backup of the Core cluster
// 6. Perform a logical backup of the Audit cluster (if enabled)
// 7. Sync the audit session logs from object storage (if enabled)
// 8. Snapshot the backup PVC
func (t *Teleport) Backup(ctx *contexts.Context, namespace, backupName, coreClusterName, servingCertIssuerName, clientCertIssuerName string, opts TeleportBackupOptions) (backup *DREvent, err error) {
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

	// Create the DR PVC if not exists
	ctx.Log.Step()
	clusterNames := []string{coreClusterName}
	if opts.AuditCluster.Enabled {
		clusterNames = append(clusterNames, opts.AuditCluster.Name)
	}

	drv, err := t.kubeClusterClient.NewDRVolume(ctx.Child(), namespace, backup.Name, opts.VolumeSize, drvolume.DRVolumeCreateOptions{
		VolumeStorageClass: opts.VolumeStorageClass,
		CNPGClusterNames:   clusterNames,
	})
	if err != nil {
		return backup, trace.Wrap(err, "failed to create the DR volume")
	}

	// Configuration
	ctx.Log.Step().Info("Configuring backup actions")
	stage := t.newRemoteStage(t.kubeClusterClient, namespace, backup.GetFullName(), remote.RemoteStageOptions{
		ClusterServiceSearchDomains: opts.ClusterServiceSearchDomains,
		CleanupTimeout:              opts.CleanupTimeout,
	})

	if opts.CloneClusterOptions.RecoveryTargetTime == "" {
		// Backup postgres clusters with their states at the same point in time
		opts.CloneClusterOptions.RecoveryTargetTime = backup.StartTime.Format(time.RFC3339)
	}

	backupOpts := cnpgbackup.CNPGBackupOptions{
		CloningOpts:    opts.CloneClusterOptions,
		CleanupTimeout: opts.CleanupTimeout,
	}

	coreBackup := t.newCNPGBackup()
	if err := coreBackup.Configure(t.kubeClusterClient, namespace, coreClusterName, servingCertIssuerName, clientCertIssuerName, backupName, teleportCoreSQLFileName, backupOpts); err != nil {
		return backup, trace.Wrap(err, "failed to configure core cluster backup")
	}
	stage.WithAction("Teleport core CNPG backup", coreBackup)

	auditBackup := t.newCNPGBackup()
	if opts.AuditCluster.Enabled {
		if err := auditBackup.Configure(t.kubeClusterClient, namespace, opts.AuditCluster.Name, servingCertIssuerName, clientCertIssuerName, backupName, teleportAuditSQLFileName, backupOpts); err != nil {
			return backup, trace.Wrap(err, "failed to configure audit cluster backup")
		}
		stage.WithAction("Teleport audit CNPG backup", auditBackup)
	}

	auditSessionLogsBackup := t.newS3Sync()
	if opts.AuditSessionLogs.Enabled {
		if err := auditSessionLogsBackup.Configure(t.kubeClusterClient, namespace, backupName, teleportAuditSessionLogsDirectoryName, opts.AuditSessionLogs.S3Path, &opts.AuditSessionLogs.Credentials, s3sync.DirectionDownload, s3sync.S3SyncOptions{}); err != nil {
			return backup, trace.Wrap(err, "failed to configure audit session logs backup")
		}
		stage.WithAction("Teleport audit session logs S3 sync", auditSessionLogsBackup)
	}

	// Run
	ctx.Log.Step().Info("Running backup actions")
	err = stage.Run(ctx.Child())
	if err != nil {
		return backup, trace.Wrap(err, "failed to run backup actions")
	}

	// Snapshot the backup PVC
	ctx.Log.Step()
	if err := drv.SnapshotAndWaitReady(ctx.Child(), backup.GetFullName(), drvolume.DRVolumeSnapshotAndWaitOptions{
		SnapshotClass: opts.BackupSnapshot.SnapshotClass,
		ReadyTimeout:  opts.BackupSnapshot.ReadyTimeout,
	}); err != nil {
		return backup, trace.Wrap(err, "failed to snapshot the backup volume")
	}

	return backup, nil
}

type TeleportRestoreOptionsAudit struct {
	TeleportOptionsAudit
	ServingCertName      string                             `yaml:"servingCertName,omitempty"`
	ClientCertIssuerName string                             `yaml:"clientCertIssuerName,omitempty"`
	PostgresUserCert     cnpgrestore.CNPGRestoreOptionsCert `yaml:"postgresUserCert,omitempty"`
}

type TeleportRestoreOptions struct {
	AuditCluster                TeleportRestoreOptionsAudit                        `yaml:"auditCluster,omitempty"`
	PostgresUserCert            cnpgrestore.CNPGRestoreOptionsCert                 `yaml:"postgresUserCert,omitempty"`
	AuditSessionLogs            TeleportOptionsS3Sync                              `yaml:"auditSessionLogs,omitempty"`
	RemoteBackupToolOptions     backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
	CleanupTimeout              helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
}

// Restore requirements:
// * The DR PVC must exist
// * Replacement clusters must be already deployed
// * The enabled CNPG cluster must already exist, but not be in use
// * The enabled CNPG client CA issuer must already exist
// * The enabled CNPG cluster must support TLS auth for the postgres user
// * The enabled CNPG cluster serving cert must already exist
// Restore process:
// 1. Ensure that the provided resources exist and are ready
// 2. Restore the core CNPG cluster
// 2. 1. Create postgres user cert
// 2. 2. Spawn a new backup-tool pod with postgres auth and serving certs, and DR mount attached
// 2. 3. Perform a Postgres logical recovery of the cluster
// 3. Restore the audit CNPG cluster (if enabled)
// 3. 1. Create postgres user cert
// 3. 2. Spawn a new backup-tool pod with postgres auth and serving certs, and DR mount attached
// 3. 3. Perform a Postgres logical recovery of the cluster
// 4. Restore the audit session logs (if enabled)
func (t *Teleport) Restore(ctx *contexts.Context, namespace, restoreName, coreClusterName, coreServingCertName, coreClientCertIssuerName string, opts TeleportRestoreOptions) (restore *DREvent, err error) {
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

	// 1. Configuration
	ctx.Log.Step().Info("Configuring restoration actions")
	stage := t.newRemoteStage(t.kubeClusterClient, namespace, restore.GetFullName(), remote.RemoteStageOptions{
		ClusterServiceSearchDomains: opts.ClusterServiceSearchDomains,
		CleanupTimeout:              opts.CleanupTimeout,
	})

	coreRestore := t.newCNPGRestore()
	if err := coreRestore.Configure(t.kubeClusterClient, namespace, coreClusterName, coreServingCertName, coreClientCertIssuerName, restoreName, teleportCoreSQLFileName, cnpgrestore.CNPGRestoreOptions{
		PostgresUserCert: opts.PostgresUserCert,
		CleanupTimeout:   opts.CleanupTimeout,
	}); err != nil {
		return restore, trace.Wrap(err, "failed to configure core cluster restoration")
	}
	stage.WithAction("Teleport core CNPG restore", coreRestore)

	auditRestore := t.newCNPGRestore()
	if opts.AuditCluster.Enabled {
		if err := auditRestore.Configure(t.kubeClusterClient, namespace, opts.AuditCluster.Name, opts.AuditCluster.ServingCertName, opts.AuditCluster.ClientCertIssuerName, restoreName, teleportAuditSQLFileName, cnpgrestore.CNPGRestoreOptions{
			PostgresUserCert: opts.AuditCluster.PostgresUserCert,
			CleanupTimeout:   opts.CleanupTimeout,
		}); err != nil {
			return restore, trace.Wrap(err, "failed to configure audit cluster restoration")
		}
		stage.WithAction("Teleport audit CNPG restore", auditRestore)
	}

	auditSessionLogsRestore := t.newS3Sync()
	if opts.AuditSessionLogs.Enabled {
		if err := auditSessionLogsRestore.Configure(t.kubeClusterClient, namespace, restoreName, teleportAuditSessionLogsDirectoryName, opts.AuditSessionLogs.S3Path, &opts.AuditSessionLogs.Credentials, s3sync.DirectionUpload, s3sync.S3SyncOptions{}); err != nil {
			return restore, trace.Wrap(err, "failed to configure audit session logs restoration")
		}
		stage.WithAction("Teleport audit session logs S3 sync", auditSessionLogsRestore)
	}

	// 2. Run
	ctx.Log.Step().Info("Running restoration actions")
	err = stage.Run(ctx.Child())
	return restore, trace.Wrap(err, "failed to run restoration actions")
}
