package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
)

type VaultWardenBackupConfig struct {
	disasterrecovery.VaultWardenBackupOptions `yaml:",inline"`
	// TODO test if these can be moved to an embedded "required" struct
	Namespace              string `yaml:"namespace" jsonschema:"required"`
	BackupName             string `yaml:"backupName" jsonschema:"required"`
	DataPVCName            string `yaml:"dataPVCName" jsonschema:"required"`
	CNPGClusterName        string `yaml:"cnpgClusterName" jsonschema:"required"`
	ServingCertIssuerName  string `yaml:"servingCertIssuerName" jsonschema:"required"`
	ClientCACertIssuerName string `yaml:"clientCACertIssuerName" jsonschema:"required"`
}

type VaultWardenRestoreConfig struct {
	disasterrecovery.VaultWardenRestoreOptions `yaml:",inline"`
	// TODO test if these can be moved to an embedded "required" struct
	Namespace            string `yaml:"namespace" jsonschema:"required"`
	BackupName           string `yaml:"backupName" jsonschema:"required"`
	DataPVCName          string `yaml:"dataPVCName" jsonschema:"required"`
	CNPGClusterName      string `yaml:"cnpgClusterName" jsonschema:"required"`
	ServingCertName      string `yaml:"servingCertName" jsonschema:"required"`
	ClientCertIssuerName string `yaml:"clientCertIssuerName" jsonschema:"required"`
}

type VaultWardenDRCommand struct {
	*ClusterDRCommand[VaultWardenBackupConfig, VaultWardenRestoreConfig]
}

func NewVaultWardenDRCommand() *VaultWardenDRCommand {
	vwBackup := func(ctx *contexts.Context, config VaultWardenBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		vw := disasterrecovery.NewVaultWarden(kubeCluster)
		_, err := vw.Backup(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName,
			config.ServingCertIssuerName, config.ClientCACertIssuerName, config.VaultWardenBackupOptions)
		return err
	}

	vwRestore := func(ctx *contexts.Context, config VaultWardenRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		vw := disasterrecovery.NewVaultWarden(kubeCluster)
		_, err := vw.Restore(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName,
			config.ServingCertName, config.ClientCertIssuerName, config.VaultWardenRestoreOptions)
		return err
	}

	return &VaultWardenDRCommand{
		ClusterDRCommand: NewClusterDRCommand("vaultwarden", vwBackup, vwRestore),
	}
}
