package common

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/spf13/cobra"
)

type ContextTimeoutCommand struct {
	Timeout time.Duration
}

func (ctc *ContextTimeoutCommand) ConfigureFlags(cmd *cobra.Command) {
	cmd.Flags().DurationVar(&ctc.Timeout, "timeout", 0, "Maximum time to wait for the command to complete before beginning termination and cancellation")
}

func (ctc *ContextTimeoutCommand) GetContextTimeout(rootContext *contexts.Context) (*contexts.Context, context.CancelFunc) {
	return contexts.WithTimeout(rootContext, ctc.Timeout)
}

// Get a context suited for a command's primary function. This will include a timeout and a signal handler.
func (ctc *ContextTimeoutCommand) GetCommandContext() (*contexts.Context, context.CancelFunc) {
	sigintCtx, sigintCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	ctx, timeoutCancel := ctc.GetContextTimeout(contexts.NewContext(sigintCtx))

	combinedCancel := func() {
		timeoutCancel()
		sigintCancel()
	}

	return ctx, combinedCancel
}
