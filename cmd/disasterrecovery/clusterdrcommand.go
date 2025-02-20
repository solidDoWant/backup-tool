package disasterrecovery

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/spf13/cobra"
)

type ClusterDREventCommandRun[TConfig interface{}] func(ctx *contexts.Context, config TConfig, kubeCluster kubecluster.ClientInterface) error

// Used for cluster-targeted disaster recovery operations. Others (like managing ZFS backups on tape) will follow
// a different process.
type ClusterDREventCommand[TConfig interface{}] struct {
	name        string
	run         ClusterDREventCommandRun[TConfig]
	kubeCluster features.KubeClusterCommandInterface
	context     features.ContextCommandInterface
	configFile  features.ConfigFileCommandInterface[TConfig]
}

func NewClusterDREventCommand[TConfig interface{}](name string, run ClusterDREventCommandRun[TConfig]) *ClusterDREventCommand[TConfig] {
	return &ClusterDREventCommand[TConfig]{
		name:        name,
		run:         run,
		context:     features.NewContextCommand(true),
		configFile:  features.NewConfigFileCommand[TConfig](),
		kubeCluster: features.NewKubeClusterCommand(),
	}
}

func (cdrec *ClusterDREventCommand[TConfig]) setup() (*contexts.Context, context.CancelFunc, TConfig, kubecluster.ClientInterface, error) {
	var defaultConfigValue TConfig

	ctx, cancel := cdrec.context.GetCommandContext()

	config, err := cdrec.configFile.ReadConfigFile(ctx)
	if err != nil {
		return nil, nil, defaultConfigValue, nil, trace.Wrap(err, "failed to read backup configuration from file")
	}

	clusterClient, err := cdrec.kubeCluster.NewKubeClusterClient()
	if err != nil {
		return nil, nil, defaultConfigValue, nil, trace.Wrap(err, "failed to create new kubernetes cluster client")
	}

	return ctx, cancel, config, clusterClient, nil
}

func (cdrec *ClusterDREventCommand[TConfig]) ConfigureFlags(cmd *cobra.Command) {
	cdrec.context.ConfigureFlags(cmd)
	cdrec.configFile.ConfigureFlags(cmd)
	cdrec.kubeCluster.ConfigureFlags(cmd)
}

func (cdrec *ClusterDREventCommand[TConfig]) GenerateConfigSchema() ([]byte, error) {
	return cdrec.configFile.GenerateConfigSchema()
}

func (cdrec *ClusterDREventCommand[TConfig]) Run() error {
	ctx, cancel, config, kubeCluster, err := cdrec.setup()
	if err != nil {
		return trace.Wrap(err, "failed to setup for %s backup", cdrec.name)
	}
	defer cancel()

	err = cdrec.run(ctx, config, kubeCluster)
	return trace.Wrap(err, "failed to backup %s", cdrec.name)
}

type ClusterDRCommand[TBackupConfig, TRestoreConfig interface{}] struct {
	name           string
	backupCommand  ClusterDREventCommandRun[TBackupConfig]
	restoreCommand ClusterDREventCommandRun[TRestoreConfig]
}

func NewClusterDRCommand[TBackupConfig, TRestoreConfig interface{}](name string, backupCommand ClusterDREventCommandRun[TBackupConfig], restoreCommand ClusterDREventCommandRun[TRestoreConfig]) *ClusterDRCommand[TBackupConfig, TRestoreConfig] {
	return &ClusterDRCommand[TBackupConfig, TRestoreConfig]{
		name:           name,
		backupCommand:  backupCommand,
		restoreCommand: restoreCommand,
	}
}

func (cdrc *ClusterDRCommand[TBackupConfig, TRestoreConfig]) Name() string {
	return cdrc.name
}

func (cdrc *ClusterDRCommand[TBackupConfig, TRestoreConfig]) GetBackupCommand() DREventCommand {
	return NewClusterDREventCommand(cdrc.Name(), cdrc.backupCommand)
}

func (cdrc *ClusterDRCommand[TBackupConfig, TRestoreConfig]) GetRestoreCommand() DREventCommand {
	return NewClusterDREventCommand(cdrc.Name(), cdrc.restoreCommand)
}
