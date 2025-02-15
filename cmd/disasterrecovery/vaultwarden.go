package disasterrecovery

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/spf13/cobra"
)

type VaultWardenDRCommand struct{}

func NewVaultWardenDRCommand() *VaultWardenDRCommand {
	return &VaultWardenDRCommand{}
}

func (vwdrc *VaultWardenDRCommand) Name() string {
	return "vaultwarden"
}

func (vwdrc *VaultWardenDRCommand) GetBackupCommand() DREventCommand {
	return NewVaultWardenBackupCommand()
}

func (vwdrc *VaultWardenDRCommand) GetRestoreCommand() DREventCommand {
	return NewVaultWardenRestoreCommand()
}

type VaultWardenDREventCommand[T interface{}] struct {
	kubeCluster features.KubeClusterCommandInterface
	context     features.ContextCommandInterface
	configFile  features.ConfigFileCommandInterface[T]
}

func NewVaultWardenDREventCommand[T interface{}]() *VaultWardenDREventCommand[T] {
	return &VaultWardenDREventCommand[T]{
		context:     features.NewContextCommand(true),
		configFile:  features.NewConfigFileCommand[T](),
		kubeCluster: features.NewKubeClusterCommand(),
	}
}

func (vwdrec *VaultWardenDREventCommand[T]) setup() (*contexts.Context, context.CancelFunc, T, *disasterrecovery.VaultWarden, error) {
	var defaultConfigValue T

	ctx, cancel := vwdrec.context.GetCommandContext()

	config, err := vwdrec.configFile.ReadConfigFile(ctx)
	if err != nil {
		return nil, nil, defaultConfigValue, nil, trace.Wrap(err, "failed to read backup configuration from file")
	}

	clusterClient, err := vwdrec.kubeCluster.NewKubeClusterClient()
	if err != nil {
		return nil, nil, defaultConfigValue, nil, trace.Wrap(err, "failed to create new kubernetes cluster client")
	}

	vw := disasterrecovery.NewVaultWarden(clusterClient)

	return ctx, cancel, config, vw, nil
}

func (vwdrec *VaultWardenDREventCommand[T]) ConfigureFlags(cmd *cobra.Command) {
	vwdrec.context.ConfigureFlags(cmd)
	vwdrec.configFile.ConfigureFlags(cmd)
	vwdrec.kubeCluster.ConfigureFlags(cmd)
}

func (vwdrec *VaultWardenDREventCommand[T]) GenerateConfigSchema() ([]byte, error) {
	return vwdrec.configFile.GenerateConfigSchema()
}

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

type VaultWardenBackupCommand struct {
	*VaultWardenDREventCommand[VaultWardenBackupConfig]
}

func NewVaultWardenBackupCommand() *VaultWardenBackupCommand {
	return &VaultWardenBackupCommand{
		VaultWardenDREventCommand: NewVaultWardenDREventCommand[VaultWardenBackupConfig](),
	}
}

func (vwbc *VaultWardenBackupCommand) Run() error {
	ctx, cancel, config, vw, err := vwbc.setup()
	if err != nil {
		return trace.Wrap(err, "failed to setup for Vaultwarden backup")
	}
	defer cancel()

	_, err = vw.Backup(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName, config.ServingCertIssuerName, config.ClientCACertIssuerName, config.VaultWardenBackupOptions)
	return trace.Wrap(err, "failed to backup Vaultwarden")
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

type VaultWardenRestoreCommand struct {
	*VaultWardenDREventCommand[VaultWardenRestoreConfig]
}

func NewVaultWardenRestoreCommand() *VaultWardenRestoreCommand {
	return &VaultWardenRestoreCommand{
		VaultWardenDREventCommand: NewVaultWardenDREventCommand[VaultWardenRestoreConfig](),
	}
}

func (vwrc *VaultWardenRestoreCommand) Run() error {
	ctx, cancel, config, vw, err := vwrc.setup()
	if err != nil {
		return trace.Wrap(err, "failed to setup for Vaultwarden restore")
	}
	defer cancel()

	_, err = vw.Restore(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName, config.ServingCertName, config.ClientCertIssuerName, config.VaultWardenRestoreOptions)
	return trace.Wrap(err, "failed to restore Vaultwarden")
}
