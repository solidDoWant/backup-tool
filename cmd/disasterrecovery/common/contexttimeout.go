package common

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

type ContextTimeoutCommand struct {
	Timeout time.Duration
}

func (ctc *ContextTimeoutCommand) ConfigureFlags(cmd *cobra.Command) {
	cmd.Flags().DurationVar(&ctc.Timeout, "timeout", 0, "Maximum time to wait for the command to complete before beginning termination and cancellation")
}

func (ctc *ContextTimeoutCommand) GetContextTimeout(rootContext context.Context) (context.Context, context.CancelFunc) {
	if ctc.Timeout > 0 {
		ctx, cancel := context.WithTimeout(rootContext, ctc.Timeout)
		return ctx, cancel
	}

	return context.WithCancel(rootContext)
}

// Get a context suited for a command's primary function. This will include a timeout and a signal handler.
func (ctc *ContextTimeoutCommand) GetCommandContext() (context.Context, context.CancelFunc) {
	ctx := context.Background()
	ctx, sigintCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	ctx, timeoutCancel := ctc.GetContextTimeout(ctx)

	combinedCancel := func() {
		timeoutCancel()
		sigintCancel()
	}

	return ctx, combinedCancel
}
