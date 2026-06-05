package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
)

// The generic app's config types already carry every field (and jsonschema tag) the CLI needs, so unlike
// the per-app commands there is no separate cmd-level config wrapper to embed — GenericBackupConfig and
// GenericRestoreConfig are used directly as the command's config types.
type GenericDRCommand struct {
	*ClusterDRCommand[disasterrecovery.GenericBackupConfig, disasterrecovery.GenericRestoreConfig]
}

func NewGenericDRCommand() *GenericDRCommand {
	backup := func(ctx *contexts.Context, config disasterrecovery.GenericBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		_, err := disasterrecovery.NewGenericApp(kubeCluster).Backup(ctx, config)
		return err
	}

	restore := func(ctx *contexts.Context, config disasterrecovery.GenericRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		_, err := disasterrecovery.NewGenericApp(kubeCluster).Restore(ctx, config)
		return err
	}

	return &GenericDRCommand{
		ClusterDRCommand: NewClusterDRCommand("generic", backup, restore),
	}
}
