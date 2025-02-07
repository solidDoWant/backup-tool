package cmd

import (
	"fmt"

	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/spf13/cobra"
)

// TODO print full build info
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: fmt.Sprintf("Print the %s version number", constants.ToolName),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("v" + constants.Version)
	},
}
