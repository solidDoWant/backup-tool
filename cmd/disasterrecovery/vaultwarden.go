package disasterrecovery

import (
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

type VaultWardenBackupConfigCNPG struct {
	Name                string                            `yaml:"name" jsonschema:"required"`
	CloneClusterOptions clonedcluster.CloneClusterOptions `yaml:"clusterCloning,omitempty"`
}

type VaultWardenBackupConfig struct {
	Namespace          string                                 `yaml:"namespace" jsonschema:"required"`
	BackupName         string                                 `yaml:"backupName" jsonschema:"required"`
	DataPVCName        string                                 `yaml:"dataPVCName" jsonschema:"required"`
	Cluster            VaultWardenBackupConfigCNPG            `yaml:"cluster" jsonschema:"required"`
	BackupVolume       ConfigBackupVolume                     `yaml:"backupVolume" jsonschema:"omitempty"`
	BackupSnapshot     disasterrecovery.OptionsBackupSnapshot `yaml:"backupSnapshot" jsonschema:"omitempty"`
	BackupToolInstance ConfigBTI                              `yaml:"backupToolInstance,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime                    `yaml:"cleanupTimeout,omitempty"`
}

type VaultWardenRestoreConfigCNPG struct {
	Name                    string                             `yaml:"name" jsonschema:"required"`
	ServingCertName         string                             `yaml:"servingCertName" jsonschema:"required"`
	ClientCAIssuer          cmmeta.IssuerReference             `yaml:"clientCAIssuer" jsonschema:"required"`
	PostgresUserCertOptions cnpgrestore.CNPGRestoreOptionsCert `yaml:"postgresUserCert,omitempty"`
}

type VaultWardenRestoreConfig struct {
	Namespace          string                       `yaml:"namespace" jsonschema:"required"`
	BackupName         string                       `yaml:"backupName" jsonschema:"required"`
	DataPVCName        string                       `yaml:"dataPVCName" jsonschema:"required"`
	Cluster            VaultWardenRestoreConfigCNPG `yaml:"cluster" jsonschema:"required"`
	BackupToolInstance ConfigBTI                    `yaml:"backupToolInstance,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime          `yaml:"cleanupTimeout,omitempty"`
}

type VaultWardenDRCommand struct {
	*ClusterDRCommand[VaultWardenBackupConfig, VaultWardenRestoreConfig]
}

func NewVaultWardenDRCommand() *VaultWardenDRCommand {
	vwBackup := func(ctx *contexts.Context, config VaultWardenBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		vw := disasterrecovery.NewVaultWarden(kubeCluster)

		opts := disasterrecovery.VaultWardenBackupOptions{
			VolumeSize:              config.BackupVolume.Size,
			VolumeStorageClass:      config.BackupVolume.StorageClass,
			CloneClusterOptions:     config.Cluster.CloneClusterOptions,
			RemoteBackupToolOptions: config.BackupToolInstance.CreationOptions,
			BackupSnapshot:          config.BackupSnapshot,
			CleanupTimeout:          config.CleanupTimeout,
		}

		_, err := vw.Backup(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.Cluster.Name, opts)
		return err
	}

	vwRestore := func(ctx *contexts.Context, config VaultWardenRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		vw := disasterrecovery.NewVaultWarden(kubeCluster)

		opts := disasterrecovery.VaultWardenRestoreOptions{
			PostgresUserCert:        config.Cluster.PostgresUserCertOptions,
			RemoteBackupToolOptions: config.BackupToolInstance.CreationOptions,
			CleanupTimeout:          config.CleanupTimeout,
		}

		_, err := vw.Restore(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.Cluster.Name,
			config.Cluster.ServingCertName, config.Cluster.ClientCAIssuer, opts)
		return err
	}

	return &VaultWardenDRCommand{
		ClusterDRCommand: NewClusterDRCommand("vaultwarden", vwBackup, vwRestore),
	}
}
