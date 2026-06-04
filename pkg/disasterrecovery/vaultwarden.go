package disasterrecovery

import (
	"github.com/gravitational/trace"
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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	vaultwardenDRVolPath   = "data-vol" // Important: changing this is will break restoration of old backups!
	vaultwardenSQLFileName = "dump.sql" // Important: changing this is will break restoration of old backups!
)

// TODO plumb a lot more options through to here
type VaultWardenBackupOptions struct {
	VolumeSize                   resource.Quantity                 `yaml:"volumeSize,omitempty"`
	VolumeStorageClass           string                            `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions          clonedcluster.CloneClusterOptions `yaml:"clusterCloning,omitempty"`
	BackupToolPodCreationTimeout helpers.MaxWaitTime               `yaml:"backupToolPodCreationTimeout,omitempty"`
	BackupSnapshot               OptionsBackupSnapshot             `yaml:"backupSnapshot,omitempty"`
	CleanupTimeout               helpers.MaxWaitTime               `yaml:"cleanupTimeout,omitempty"`
}

type VaultWarden struct {
	kubeClusterClient kubecluster.ClientInterface
	// Testing injection
	newCNPGBackup   func() cnpgbackup.CNPGBackupInterface
	newCNPGRestore  func() cnpgrestore.CNPGRestoreInterface
	newFilesBackup  func() filesbackup.FilesBackupInterface
	newFilesRestore func() filesrestore.FilesRestoreInterface
	newRemoteStage  func(kubeClusterClient kubecluster.ClientInterface, namespace, eventName string, opts remote.RemoteStageOptions) remote.RemoteStageInterface
}

func NewVaultWarden(client kubecluster.ClientInterface) *VaultWarden {
	return &VaultWarden{
		kubeClusterClient: client,
		newCNPGBackup:     cnpgbackup.NewCNPGBackup,
		newCNPGRestore:    cnpgrestore.NewCNPGRestore,
		newFilesBackup:    filesbackup.NewFilesBackup,
		newFilesRestore:   filesrestore.NewFilesRestore,
		newRemoteStage:    remote.NewRemoteStage,
	}
}

// Backup process:
//  1. Create the DR PVC if not exists
//  2. Configure the backup actions: a CNPG logical backup of the cluster, and a files capture of the
//     data-directory PVC
//  3. Run the stage's pre-consistency-point work in order — the CNPG action takes its base backup, then the
//     files action clones the live data PVC. The clone's creation time becomes the event's consistency
//     point, and the stage sets up and executes each action against a single tool pod:
//     - the CNPG action clones the cluster recovering forward to the consistency point (the data-directory
//     freeze) and dumps it to the DR volume
//     - the files action syncs the clone into the DR volume's data-vol subdirectory
//  4. Snapshot the DR volume
//
// The CNPG action is registered before the files action because the database base backup must be taken
// before the data-directory clone: the clone time is the consistency point, and the database can only
// recover forward to it from a base backup taken earlier. Recovering forward to exactly the filesystem
// freeze reproduces the original Vaultwarden behaviour, where the database is aligned to the moment the
// data directory was captured.
func (vw *VaultWarden) Backup(ctx *contexts.Context, namespace, backupName, dataPVC, cnpgClusterName, servingCertIssuerName, clientCertIssuerName string, opts VaultWardenBackupOptions) (backup *DREvent, err error) {
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

	// Create the DR PVC if not exists. Vaultwarden's DR volume holds the synced data directory in addition
	// to the SQL dump, so size it from the data PVC rather than the CNPG cluster. Default to roughly twice
	// the data PVC size to fit both captures.
	ctx.Log.Step().Info("Ensuring DR volume exists")
	drVolumeSize := opts.VolumeSize
	if drVolumeSize.IsZero() {
		dataPVCObj, err := vw.kubeClusterClient.Core().GetPVC(ctx.Child(), namespace, dataPVC)
		if err != nil {
			return backup, trace.Wrap(err, "failed to get data PVC %q to size the DR volume", dataPVC)
		}

		size, ok := dataPVCObj.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			return backup, trace.Errorf("data PVC %q has no storage request to size the DR volume from", dataPVC)
		}
		drVolumeSize = size
		drVolumeSize.Mul(2)
	}

	drv, err := vw.kubeClusterClient.NewDRVolume(ctx.Child(), namespace, backup.Name, drVolumeSize, drvolume.DRVolumeCreateOptions{
		VolumeStorageClass: opts.VolumeStorageClass,
		CNPGClusterNames:   []string{cnpgClusterName},
	})
	if err != nil {
		return backup, trace.Wrap(err, "failed to create the DR volume")
	}

	// Configuration
	ctx.Log.Step().Info("Configuring backup actions")
	stage := vw.newRemoteStage(vw.kubeClusterClient, namespace, backup.GetFullName(), remote.RemoteStageOptions{
		CleanupTimeout: opts.CleanupTimeout,
	})

	backupOpts := cnpgbackup.CNPGBackupOptions{
		CloningOpts:    opts.CloneClusterOptions,
		CleanupTimeout: opts.CleanupTimeout,
	}

	cnpgBackup := vw.newCNPGBackup()
	if err := cnpgBackup.Configure(vw.kubeClusterClient, namespace, cnpgClusterName, servingCertIssuerName, clientCertIssuerName, backup.Name, vaultwardenSQLFileName, backupOpts); err != nil {
		return backup, trace.Wrap(err, "failed to configure CNPG cluster backup")
	}
	stage.WithAction("Vaultwarden CNPG backup", cnpgBackup)

	dataBackup := vw.newFilesBackup()
	if err := dataBackup.Configure(vw.kubeClusterClient, namespace, dataPVC, backup.Name, vaultwardenDRVolPath, filesbackup.FilesBackupOptions{
		CleanupTimeout: opts.CleanupTimeout,
	}); err != nil {
		return backup, trace.Wrap(err, "failed to configure data directory backup")
	}
	stage.WithAction("Vaultwarden data directory backup", dataBackup)

	// Run
	ctx.Log.Step().Info("Running backup actions")
	if err := stage.Run(ctx.Child()); err != nil {
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

type vaultWardenRestoreOptionsCertificates struct {
	PostgresUserCert OptionsClusterUserCert `yaml:"postgresUserCert,omitempty"`
}

type VaultWardenRestoreOptions struct {
	Certificates            vaultWardenRestoreOptionsCertificates              `yaml:"certificates,omitempty"`
	IssuerKind              string                                             `yaml:"issuerKind,omitempty"`
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
//  1. Configure the restoration actions: a files restore of the data directory onto the data PVC, and a
//     CNPG logical recovery of the cluster
//  2. Run the stage. It sets up and executes each action against a single tool pod:
//     - the CNPG action issues a postgres user cert and restores the SQL dump into the cluster
//     - the files action syncs the DR volume's data-vol subdirectory back onto the data PVC
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

	// 1. Configuration
	ctx.Log.Step().Info("Configuring restoration actions")
	stage := vw.newRemoteStage(vw.kubeClusterClient, namespace, restore.GetFullName(), remote.RemoteStageOptions{
		CleanupTimeout: opts.CleanupTimeout,
	})

	// Restore actions are independent (no consistency point is established on restore), but they are
	// registered in the same order as the backup actions for consistency: CNPG first, then the data
	// directory.
	cnpgRestore := vw.newCNPGRestore()
	if err := cnpgRestore.Configure(vw.kubeClusterClient, namespace, cnpgClusterName, servingCertName, clientCertIssuerName, restoreName, vaultwardenSQLFileName, cnpgrestore.CNPGRestoreOptions{
		IssuerKind: opts.IssuerKind,
		PostgresUserCert: cnpgrestore.CNPGRestoreOptionsCert{
			Subject:            opts.Certificates.PostgresUserCert.Subject,
			CRPOpts:            opts.Certificates.PostgresUserCert.CRPOpts,
			WaitForCertTimeout: opts.Certificates.PostgresUserCert.WaitForReadyTimeout,
		},
		CleanupTimeout: opts.CleanupTimeout,
	}); err != nil {
		return restore, trace.Wrap(err, "failed to configure CNPG cluster restoration")
	}
	stage.WithAction("Vaultwarden CNPG cluster restore", cnpgRestore)

	dataRestore := vw.newFilesRestore()
	if err := dataRestore.Configure(vw.kubeClusterClient, namespace, dataPVCName, restoreName, vaultwardenDRVolPath, filesrestore.FilesRestoreOptions{}); err != nil {
		return restore, trace.Wrap(err, "failed to configure data directory restoration")
	}
	stage.WithAction("Vaultwarden data directory restore", dataRestore)

	// 2. Run
	ctx.Log.Step().Info("Running restoration actions")
	err = stage.Run(ctx.Child())
	return restore, trace.Wrap(err, "failed to run restoration actions")
}
