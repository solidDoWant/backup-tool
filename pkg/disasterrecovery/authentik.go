package disasterrecovery

import (
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
	authentikSQLFileName        = "dump.sql"
	authentikMediaDirectoryName = "media"
)

type AuthentikBackupOptions struct {
	VolumeSize                  resource.Quantity                                  `yaml:"volumeSize,omitempty"`
	VolumeStorageClass          string                                             `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions         clonedcluster.CloneClusterOptions                  `yaml:"clusterCloning,omitempty"`
	RemoteBackupToolOptions     backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
	BackupSnapshot              OptionsBackupSnapshot                              `yaml:"backupSnapshot,omitempty"`
	CleanupTimeout              helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
}

type Authentik struct {
	kubeClusterClient kubecluster.ClientInterface
	// Testing injection
	newCNPGBackup  func() cnpgbackup.CNPGBackupInterface
	newCNPGRestore func() cnpgrestore.CNPGRestoreInterface
	newS3Sync      func() s3sync.S3SyncInterface
	newRemoteStage func(kubeClusterClient kubecluster.ClientInterface, namespace, eventName string, opts remote.RemoteStageOptions) remote.RemoteStageInterface
}

func NewAuthentik(kubeClusterClient kubecluster.ClientInterface) *Authentik {
	return &Authentik{
		kubeClusterClient: kubeClusterClient,
		newCNPGBackup:     cnpgbackup.NewCNPGBackup,
		newCNPGRestore:    cnpgrestore.NewCNPGRestore,
		newS3Sync:         s3sync.NewS3Sync,
		newRemoteStage:    remote.NewRemoteStage,
	}
}

func (a *Authentik) Backup(ctx *contexts.Context, namespace, backupName, clusterName, servingCertIssuerName, clientCertIssuerName, mediaS3Path string, mediaS3Credentials s3.CredentialsInterface, opts AuthentikBackupOptions) (backup *DREvent, err error) {
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
	drv, err := a.kubeClusterClient.NewDRVolume(ctx.Child(), namespace, backup.Name, opts.VolumeSize, drvolume.DRVolumeCreateOptions{
		VolumeStorageClass: opts.VolumeStorageClass,
		CNPGClusterNames:   []string{clusterName},
	})
	if err != nil {
		return backup, trace.Wrap(err, "failed to create the DR volume")
	}

	// Configuration
	ctx.Log.Step().Info("Configuring backup actions")
	stage := a.newRemoteStage(a.kubeClusterClient, namespace, backup.GetFullName(), remote.RemoteStageOptions{
		ClusterServiceSearchDomains: opts.ClusterServiceSearchDomains,
		CleanupTimeout:              opts.CleanupTimeout,
	})

	backupOpts := cnpgbackup.CNPGBackupOptions{
		CloningOpts:    opts.CloneClusterOptions,
		CleanupTimeout: opts.CleanupTimeout,
	}

	cnpgBackup := a.newCNPGBackup()
	if err := cnpgBackup.Configure(a.kubeClusterClient, namespace, clusterName, servingCertIssuerName, clientCertIssuerName, backupName, authentikSQLFileName, backupOpts); err != nil {
		return backup, trace.Wrap(err, "failed to configure CNPG cluster backup")
	}
	stage.WithAction("Authentik CNPG backup", cnpgBackup)

	mediaBackup := a.newS3Sync()
	if err := mediaBackup.Configure(a.kubeClusterClient, namespace, backupName, authentikMediaDirectoryName, mediaS3Path, mediaS3Credentials, s3sync.DirectionDownload, s3sync.S3SyncOptions{}); err != nil {
		return backup, trace.Wrap(err, "failed to configure media S3 backup")
	}
	stage.WithAction("Authentik media S3 sync", mediaBackup)

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

type AuthentikRestoreOptions struct {
	PostgresUserCert            cnpgrestore.CNPGRestoreOptionsCert                 `yaml:"postgresUserCert,omitempty"`
	IssuerKind                  string                                             `yaml:"issuerKind,omitempty"`
	RemoteBackupToolOptions     backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
	CleanupTimeout              helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
}

func (a *Authentik) Restore(ctx *contexts.Context, namespace, restoreName, clusterName, servingCertName, clientCertIssuerName string, mediaS3Path string, mediaS3Credentials s3.CredentialsInterface, opts AuthentikRestoreOptions) (restore *DREvent, err error) {
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
	stage := a.newRemoteStage(a.kubeClusterClient, namespace, restore.GetFullName(), remote.RemoteStageOptions{
		ClusterServiceSearchDomains: opts.ClusterServiceSearchDomains,
		CleanupTimeout:              opts.CleanupTimeout,
	})

	cnpgRestore := a.newCNPGRestore()
	if err := cnpgRestore.Configure(a.kubeClusterClient, namespace, clusterName, servingCertName, clientCertIssuerName, restoreName, authentikSQLFileName, cnpgrestore.CNPGRestoreOptions{
		PostgresUserCert: opts.PostgresUserCert,
		IssuerKind:       opts.IssuerKind,
		CleanupTimeout:   opts.CleanupTimeout,
	}); err != nil {
		return restore, trace.Wrap(err, "failed to configure CNPG cluster restoration")
	}
	stage.WithAction("Autnentik CNPG cluster restore", cnpgRestore)

	mediaRestore := a.newS3Sync()
	if err := mediaRestore.Configure(a.kubeClusterClient, namespace, restoreName, authentikMediaDirectoryName, mediaS3Path, mediaS3Credentials, s3sync.DirectionUpload, s3sync.S3SyncOptions{}); err != nil {
		return restore, trace.Wrap(err, "failed to configure media S3 restoration")
	}
	stage.WithAction("Authentik media S3 sync", mediaRestore)

	// 2. Run
	ctx.Log.Step().Info("Running restoration actions")
	err = stage.Run(ctx.Child())
	return restore, trace.Wrap(err, "failed to run restoration actions")
}
