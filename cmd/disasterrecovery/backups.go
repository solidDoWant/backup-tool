package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
)

// BackupsCommandName is the name of the command that manages existing backups of every DR type, rather
// than backing up or restoring a specific application. Exposes `dr backups cleanup`.
const BackupsCommandName = "backups"

// BackupsDRCommand manages the backups taken by every DR type. Like the generic app, its config type
// already carries every field the CLI needs, so it is used directly.
type BackupsDRCommand struct {
	cleanupCommand ClusterDREventCommandRun[disasterrecovery.BackupCleanupConfig]
}

func NewBackupsDRCommand() *BackupsDRCommand {
	cleanup := func(ctx *contexts.Context, config disasterrecovery.BackupCleanupConfig, kubeCluster kubecluster.ClientInterface) error {
		return disasterrecovery.NewBackupCleanup(kubeCluster).Run(ctx, config)
	}

	return &BackupsDRCommand{
		cleanupCommand: cleanup,
	}
}

func (bdrc *BackupsDRCommand) Name() string {
	return BackupsCommandName
}

func (bdrc *BackupsDRCommand) GetCleanupCommand() DREventCommand {
	return NewClusterDREventCommand(bdrc.Name(), bdrc.cleanupCommand)
}
