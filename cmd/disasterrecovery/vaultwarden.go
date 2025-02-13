package disasterrecovery

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/spf13/cobra"
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

type VaultWardenCommand struct {
	kubeConfig     features.KubernetesCommand
	timeoutContext features.ContextCommand
	configFile     features.ConfigFileCommand[VaultWardenBackupConfig]
}

func NewVaultWardenCommand() *VaultWardenCommand {
	return &VaultWardenCommand{}
}

func (vwc *VaultWardenCommand) DRCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vaultwarden",
		Short: "Disaster recovery for Vaultwarden",
	}

	return cmd
}

func (vwc *VaultWardenCommand) Backup() error {
	ctx, cancel := vwc.timeoutContext.GetCommandContext()
	defer cancel()

	config, err := vwc.configFile.ReadConfigFile(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to read backup configuration from file")
	}

	clusterClient, err := vwc.kubeConfig.NewKubeClusterClient()
	if err != nil {
		return trace.Wrap(err, "failed to create new kubernetes cluster client")
	}

	vw := disasterrecovery.NewVaultWarden(clusterClient)
	_, err = vw.Backup(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName, config.ServingCertIssuerName, config.ClientCACertIssuerName, config.VaultWardenBackupOptions)
	return trace.Wrap(err, "failed to backup Vaultwarden")
}

func (vwc *VaultWardenCommand) ConfigureBackupFlags(cmd *cobra.Command) {
	vwc.kubeConfig.ConfigureFlags(cmd)
	vwc.timeoutContext.ConfigureFlags(cmd)
	vwc.configFile.ConfigureFlags(cmd)
}

func (vwc *VaultWardenCommand) GenerateConfigSchema() ([]byte, error) {
	return vwc.configFile.GenerateConfigSchema()
}
