package disasterrecovery

import (
	"bytes"
	"io"
	"os"

	"github.com/gravitational/trace"
	"github.com/spf13/cobra"
)

type SupportsBackup interface {
	Backup() error
	ConfigureBackupFlags(cmd *cobra.Command)
}

type SupportsRestore interface {
	Restore() error
}

type SupportsConfigSchemaGeneration interface {
	GenerateConfigSchema() ([]byte, error)
}

type DRCommand interface {
	DRCommand() *cobra.Command
}

var drCommands = []DRCommand{
	NewVaultWardenCommand(),
}

func getDRSubcommands() []*cobra.Command {
	var commands []*cobra.Command
	for _, cmd := range drCommands {
		drCommand := cmd.DRCommand()

		// Add subcommands
		if backupDRCmd, ok := cmd.(SupportsBackup); ok {
			backupCommand := &cobra.Command{
				Use: "backup",
				RunE: func(cmd *cobra.Command, args []string) error {
					return backupDRCmd.Backup()
				},
				SilenceUsage: true,
			}
			backupDRCmd.ConfigureBackupFlags(backupCommand)
			drCommand.AddCommand(backupCommand)
		}

		if restoreDRCmd, ok := cmd.(SupportsRestore); ok {
			restoreCommand := &cobra.Command{
				Use: "restore",
				RunE: func(cmd *cobra.Command, args []string) error {
					return restoreDRCmd.Restore()
				},
				SilenceUsage: true,
			}
			drCommand.AddCommand(restoreCommand)
		}

		if genSchemaCmd, ok := cmd.(SupportsConfigSchemaGeneration); ok {
			restoreCommand := &cobra.Command{
				Use: "gen-config-schema",
				RunE: func(cmd *cobra.Command, args []string) error {
					schema, err := genSchemaCmd.GenerateConfigSchema()
					if err != nil {
						return trace.Wrap(err, "failed to generate config schema")
					}

					_, err = io.Copy(os.Stdout, bytes.NewReader(schema))
					return trace.Wrap(err, "failed to write config schema to stdout")
				},
				SilenceUsage: true,
			}
			drCommand.AddCommand(restoreCommand)
		}

		if len(drCommand.Commands()) != 0 {
			commands = append(commands, drCommand)
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
