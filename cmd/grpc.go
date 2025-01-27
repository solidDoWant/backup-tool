package cmd

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/grpc/servers"
	"github.com/spf13/cobra"
)

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "Run in GRPC server mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := servers.StartServer()
		return trace.Wrap(err, "grpc server failed")
	},
}
