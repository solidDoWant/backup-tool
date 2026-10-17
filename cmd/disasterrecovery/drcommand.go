package disasterrecovery

import (
	"github.com/spf13/cobra"
)

type DRCommand interface {
	Name() string
}

type DRBackupCommand interface {
	DRCommand
	GetBackupCommand() DREventCommand
}

type DRRestoreCommand interface {
	DRCommand
	GetRestoreCommand() DREventCommand
}

type DRCleanupCommand interface {
	DRCommand
	GetCleanupCommand() DREventCommand
}

func buildDRCommand(drCmd DRCommand) *cobra.Command {
	cmd := &cobra.Command{
		Use:   drCmd.Name(),
		Short: "Disaster recovery for " + drCmd.Name(),
	}

	// Add subcommands
	if backupDRCmd, ok := drCmd.(DRBackupCommand); ok {
		cmd.AddCommand(buildDREventCommand(backupDRCmd.GetBackupCommand(), drCmd.Name(), "backup"))
	}

	if restoreDRCmd, ok := drCmd.(DRRestoreCommand); ok {
		cmd.AddCommand(buildDREventCommand(restoreDRCmd.GetRestoreCommand(), drCmd.Name(), "restore"))
	}

	if cleanupDRCmd, ok := drCmd.(DRCleanupCommand); ok {
		cmd.AddCommand(buildDREventCommand(cleanupDRCmd.GetCleanupCommand(), drCmd.Name(), "cleanup"))
	}

	if len(cmd.Commands()) == 0 {
		return nil
	}

	return cmd
}

var drCommands = []DRCommand{
	NewVaultWardenDRCommand(),
	NewTeleportDRCommand(),
	NewAuthentikDRCommand(),
	NewGenericDRCommand(),
	NewBackupsDRCommand(),
}

func getDRSubcommands() []*cobra.Command {
	var commands []*cobra.Command
	for _, drCommand := range drCommands {
		cmd := buildDRCommand(drCommand)
		if cmd != nil {
			commands = append(commands, cmd)
		}
	}
	return commands
}

func GetDRCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dr",
		Short: "Disaster recovery",
	}
	cmd.AddCommand(getDRSubcommands()...)
	return cmd
}
