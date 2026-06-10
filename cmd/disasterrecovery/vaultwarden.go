package disasterrecovery

import (
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
)

type VaultWardenBackupConfig struct {
	disasterrecovery.VaultWardenBackupOptions `yaml:",inline"`
	// TODO test if these can be moved to an embedded "required" struct
	Namespace       string `yaml:"namespace" jsonschema:"required"`
	BackupName      string `yaml:"backupName" jsonschema:"required"`
	DataPVCName     string `yaml:"dataPVCName" jsonschema:"required"`
	CNPGClusterName string `yaml:"cnpgClusterName" jsonschema:"required"`
}

type VaultWardenRestoreConfig struct {
	disasterrecovery.VaultWardenRestoreOptions `yaml:",inline"`
	// TODO test if these can be moved to an embedded "required" struct
	Namespace       string                 `yaml:"namespace" jsonschema:"required"`
	BackupName      string                 `yaml:"backupName" jsonschema:"required"`
	DataPVCName     string                 `yaml:"dataPVCName" jsonschema:"required"`
	CNPGClusterName string                 `yaml:"cnpgClusterName" jsonschema:"required"`
	ServingCertName string                 `yaml:"servingCertName" jsonschema:"required"`
	ClientCAIssuer  cmmeta.IssuerReference `yaml:"clientCAIssuer" jsonschema:"required"`
}

type VaultWardenDRCommand struct {
	*ClusterDRCommand[VaultWardenBackupConfig, VaultWardenRestoreConfig]
}

func NewVaultWardenDRCommand() *VaultWardenDRCommand {
	vwBackup := func(ctx *contexts.Context, config VaultWardenBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		vw := disasterrecovery.NewVaultWarden(kubeCluster)
		_, err := vw.Backup(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName,
			config.VaultWardenBackupOptions)
		return err
	}

	vwRestore := func(ctx *contexts.Context, config VaultWardenRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		vw := disasterrecovery.NewVaultWarden(kubeCluster)
		_, err := vw.Restore(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName,
			config.ServingCertName, config.ClientCAIssuer, config.VaultWardenRestoreOptions)
		return err
	}

	return &VaultWardenDRCommand{
		ClusterDRCommand: NewClusterDRCommand("vaultwarden", vwBackup, vwRestore),
	}
}
