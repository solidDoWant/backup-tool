package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/s3"
)

type AuthentikConfigCNPG struct {
	Name                   string `yaml:"name" jsonschema:"required"`
	ClientCACertIssuerName string `yaml:"clientCACertIssuerName" jsonschema:"required"`
}

type AuthentikBackupConfigCNPG struct {
	AuthentikConfigCNPG   `yaml:",inline"`
	ServingCertIssuerName string                            `yaml:"servingCertIssuerName" jsonschema:"required"`
	CloneClusterOptions   clonedcluster.CloneClusterOptions `yaml:"clusterCloning,omitempty"`
}

type AuthentikBackupConfigS3 struct {
	S3Path      string         `yaml:"s3Path" jsonschema:"required"`
	Credentials s3.Credentials `yaml:"credentials" jsonschema:"required"`
}

type AuthentikBackupConfig struct {
	Namespace          string                                 `yaml:"namespace" jsonschema:"required"`
	BackupName         string                                 `yaml:"backupName" jsonschema:"required"`
	Cluster            AuthentikBackupConfigCNPG              `yaml:"cluster" jsonschema:"required"`
	S3                 AuthentikBackupConfigS3                `yaml:"s3" jsonschema:"required"`
	BackupVolume       ConfigBackupVolume                     `yaml:"backupVolume" jsonschema:"omitempty"`
	BackupSnapshot     disasterrecovery.OptionsBackupSnapshot `yaml:"backupSnapshot" jsonschema:"omitempty"`
	BackupToolInstance ConfigBTI                              `yaml:"backupToolInstance,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime                    `yaml:"cleanupTimeout,omitempty"`
}

type AuthentikRestoreConfigCNPG struct {
	AuthentikConfigCNPG     `yaml:",inline"`
	ServingCertName         string                             `yaml:"servingCertName" jsonschema:"required"`
	PostgresUserCertOptions cnpgrestore.CNPGRestoreOptionsCert `yaml:"postgresUserCert,omitempty"`
}

type AuthentikRestoreConfig struct {
	Namespace          string                     `yaml:"namespace" jsonschema:"required"`
	BackupName         string                     `yaml:"backupName" jsonschema:"required"`
	Cluster            AuthentikRestoreConfigCNPG `yaml:"cluster" jsonschema:"required"`
	S3                 AuthentikBackupConfigS3    `yaml:"s3" jsonschema:"required"`
	BackupToolInstance ConfigBTI                  `yaml:"backupToolInstance,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime        `yaml:"cleanupTimeout,omitempty"`
}

type AuthentikDRCommand struct {
	*ClusterDRCommand[AuthentikBackupConfig, AuthentikRestoreConfig]
}

func NewAuthentikDRCommand() *AuthentikDRCommand {
	aBackup := func(ctx *contexts.Context, config AuthentikBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		a := disasterrecovery.NewAuthentik(kubeCluster)

		opts := disasterrecovery.AuthentikBackupOptions{
			VolumeSize:                  config.BackupVolume.Size,
			VolumeStorageClass:          config.BackupVolume.StorageClass,
			CloneClusterOptions:         config.Cluster.CloneClusterOptions,
			RemoteBackupToolOptions:     config.BackupToolInstance.CreationOptions,
			ClusterServiceSearchDomains: config.BackupToolInstance.ServiceSearchDomains,
			BackupSnapshot:              config.BackupSnapshot,
			CleanupTimeout:              config.CleanupTimeout,
		}

		_, err := a.Backup(ctx, config.Namespace, config.BackupName, config.Cluster.Name, config.Cluster.ServingCertIssuerName,
			config.Cluster.ClientCACertIssuerName, config.S3.S3Path, &config.S3.Credentials, opts)

		return err
	}

	aRestore := func(ctx *contexts.Context, config AuthentikRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		a := disasterrecovery.NewAuthentik(kubeCluster)

		opts := disasterrecovery.AuthentikRestoreOptions{
			PostgresUserCert:            config.Cluster.PostgresUserCertOptions,
			RemoteBackupToolOptions:     config.BackupToolInstance.CreationOptions,
			ClusterServiceSearchDomains: config.BackupToolInstance.ServiceSearchDomains,
			CleanupTimeout:              config.CleanupTimeout,
		}

		_, err := a.Restore(ctx, config.Namespace, config.BackupName, config.Cluster.Name, config.Cluster.ServingCertName,
			config.Cluster.ClientCACertIssuerName, config.S3.S3Path, &config.S3.Credentials, opts)

		return err
	}

	return &AuthentikDRCommand{
		ClusterDRCommand: NewClusterDRCommand("authentik", aBackup, aRestore),
	}
}
