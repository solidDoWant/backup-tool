package cmd

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/grpc/servers"
	"github.com/spf13/cobra"
)

type GRPCCommand struct {
	timeoutContext features.ContextCommand
}

func NewGRPCCommand() *GRPCCommand {
	return &GRPCCommand{}
}

func (grpcc *GRPCCommand) run() error {
	ctx, cancel := grpcc.timeoutContext.GetCommandContext()
	defer cancel()

	err := servers.StartServer(ctx)
	return trace.Wrap(err, "GRPC server failed")
}

func (grpcc *GRPCCommand) configureFlags(cmd *cobra.Command) {
	grpcc.timeoutContext.ConfigureFlags(cmd)
}

func (grpcc *GRPCCommand) GRPCCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grpc",
		Short: "Run in GRPC server mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return grpcc.run()
		},
	}

	grpcc.configureFlags(cmd)

	return cmd
}

func GetGRPCCommand() *cobra.Command {
	return NewGRPCCommand().GRPCCommand()
}
