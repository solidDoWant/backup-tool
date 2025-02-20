package disasterrecovery

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/spf13/cobra"
)

type TeleportDRCommand struct{}

func NewTeleportDRCommand() *TeleportDRCommand {
	return &TeleportDRCommand{}
}

func (vwdrc *TeleportDRCommand) Name() string {
	return "Teleport"
}

func (vwdrc *TeleportDRCommand) GetBackupCommand() DREventCommand {
	return NewTeleportBackupCommand()
}

// func (vwdrc *TeleportDRCommand) GetRestoreCommand() DREventCommand {
// 	return NewTeleportRestoreCommand()
// }

type TeleportDREventCommand[T interface{}] struct {
	kubeCluster features.KubeClusterCommandInterface
	context     features.ContextCommandInterface
	configFile  features.ConfigFileCommandInterface[T]
}

func NewTeleportDREventCommand[T interface{}]() *TeleportDREventCommand[T] {
	return &TeleportDREventCommand[T]{
		context:     features.NewContextCommand(true),
		configFile:  features.NewConfigFileCommand[T](),
		kubeCluster: features.NewKubeClusterCommand(),
	}
}

func (vwdrec *TeleportDREventCommand[T]) setup() (*contexts.Context, context.CancelFunc, T, *disasterrecovery.Teleport, error) {
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

	vw := disasterrecovery.NewTeleport(clusterClient)

	return ctx, cancel, config, vw, nil
}

func (vwdrec *TeleportDREventCommand[T]) ConfigureFlags(cmd *cobra.Command) {
	vwdrec.context.ConfigureFlags(cmd)
	vwdrec.configFile.ConfigureFlags(cmd)
	vwdrec.kubeCluster.ConfigureFlags(cmd)
}

func (vwdrec *TeleportDREventCommand[T]) GenerateConfigSchema() ([]byte, error) {
	return vwdrec.configFile.GenerateConfigSchema()
}

type TeleportBackupConfig struct {
	disasterrecovery.TeleportBackupOptions `yaml:",inline"`
	// TODO test if these can be moved to an embedded "required" struct
	Namespace              string `yaml:"namespace" jsonschema:"required"`
	BackupName             string `yaml:"backupName" jsonschema:"required"`
	DataPVCName            string `yaml:"dataPVCName" jsonschema:"required"`
	CNPGClusterName        string `yaml:"cnpgClusterName" jsonschema:"required"`
	ServingCertIssuerName  string `yaml:"servingCertIssuerName" jsonschema:"required"`
	ClientCACertIssuerName string `yaml:"clientCACertIssuerName" jsonschema:"required"`
}

type TeleportBackupCommand struct {
	*TeleportDREventCommand[TeleportBackupConfig]
}

func NewTeleportBackupCommand() *TeleportBackupCommand {
	return &TeleportBackupCommand{
		TeleportDREventCommand: NewTeleportDREventCommand[TeleportBackupConfig](),
	}
}

func (vwbc *TeleportBackupCommand) Run() error {
	ctx, cancel, config, vw, err := vwbc.setup()
	if err != nil {
		return trace.Wrap(err, "failed to setup for Teleport backup")
	}
	defer cancel()

	_, err = vw.Backup(ctx, config.Namespace, config.BackupName, config.DataPVCName, config.CNPGClusterName, config.ServingCertIssuerName, config.ClientCACertIssuerName, config.TeleportBackupOptions)
	return trace.Wrap(err, "failed to backup Teleport")
}
