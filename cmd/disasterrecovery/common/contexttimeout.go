package common

import (
	"context"
	"time"

	"github.com/spf13/cobra"
)

type ContextTimeoutCommand struct {
	Timeout time.Duration
}

func (ctc *ContextTimeoutCommand) ConfigureFlags(cmd *cobra.Command) {
	cmd.Flags().DurationVar(&ctc.Timeout, "timeout", 0, "Maximum time to wait for the command to complete before beginning termination and cancellation")
}

func (ctc *ContextTimeoutCommand) GetContextTimeout() (context.Context, context.CancelFunc) {
	rootContext := context.Background()
	if ctc.Timeout > 0 {
		ctx, cancel := context.WithTimeout(rootContext, ctc.Timeout)
		return ctx, cancel
	}

	return context.WithCancel(rootContext)
}
