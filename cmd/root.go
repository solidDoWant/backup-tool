package cmd

import (
	"fmt"
	"os"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/cmd/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   constants.ToolName,
	Short: "A tool to backup and restore infra-mk3 resources",
	// Long: `backup-tool is a CLI tool to backup and restore infra-mk3 resources. // TODO
}

func Execute() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(grpcCmd)
	rootCmd.AddCommand(disasterrecovery.GetDRCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, trace.DebugReport(err))
		os.Exit(1)
	}
}
