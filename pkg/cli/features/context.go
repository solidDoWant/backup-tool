package features

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/spf13/cobra"
)

// Implement pflag.Value interface for log.Level
type logLevelVar struct {
	LogLevel log.Level
}

func (llv *logLevelVar) Set(value string) error {
	level, err := log.ParseLevel(value)
	if err != nil {
		return err
	}

	llv.LogLevel = level
	return nil
}

func (llv *logLevelVar) Type() string {
	return "logLevel"
}

func (llv *logLevelVar) String() string {
	return llv.LogLevel.String()
}

// Implement pflag.Value interface for log.Formatter
type logFormatVar struct {
	log.Formatter
}

func (lfv *logFormatVar) Set(value string) error {
	switch value {
	case "text":
		lfv.Formatter = log.TextFormatter
	case "json":
		lfv.Formatter = log.JSONFormatter
	case "logfmt":
		lfv.Formatter = log.LogfmtFormatter
	default:
		return fmt.Errorf("unknown log format: %s", value)
	}
	return nil
}

func (lfv *logFormatVar) Type() string {
	return "logFormat"
}

func (lfv *logFormatVar) String() string {
	switch lfv.Formatter {
	case log.TextFormatter:
		return "text"
	case log.JSONFormatter:
		return "json"
	case log.LogfmtFormatter:
		return "logfmt"
	default:
		// If this is hit then the program has a bug.
		return "unknown"
	}
}

type ContextCommandInterface interface {
	ConfigureFlags(cmd *cobra.Command)
	GetCommandContext() (*contexts.Context, context.CancelFunc)
}

// Gives the command the ability to create a context for its execution.
type ContextCommand struct {
	logLevelVar
	logFormatVar
	Timeout time.Duration
	// If false, the command will not have a timeout, and will not have a timeout flag.
	EnableTimeout bool
}

func NewContextCommand(enableTimeout bool) *ContextCommand {
	return &ContextCommand{
		Timeout: 0,
		logLevelVar: logLevelVar{
			LogLevel: log.InfoLevel,
		},
		logFormatVar: logFormatVar{
			Formatter: log.TextFormatter,
		},
		EnableTimeout: enableTimeout,
	}
}

func (ctc *ContextCommand) ConfigureFlags(cmd *cobra.Command) {
	cmd.Flags().Var(&ctc.logLevelVar, "log-level", "Log level (debug, info, warn, error)")
	cmd.Flags().Var(&ctc.logFormatVar, "log-format", "Log format (text, json, logfmt)")

	if ctc.EnableTimeout {
		cmd.Flags().DurationVar(&ctc.Timeout, "timeout", 0, "Maximum time to wait for the command to complete before beginning termination and cancellation (0 = no timeout)")
	}
}
func (ctc *ContextCommand) getLogOptions() log.Options {
	options := log.Options{
		Level:     ctc.LogLevel,
		Formatter: ctc.Formatter,
	}

	if ctc.LogLevel == log.DebugLevel {
		options.ReportCaller = true
	}

	if ctc.LogLevel == log.DebugLevel || ctc.LogLevel == log.InfoLevel {
		options.ReportTimestamp = true
	}

	return options
}

// Get a context suited for a command's primary function. This will include a timeout and a signal handler.
func (ctc *ContextCommand) GetCommandContext() (*contexts.Context, context.CancelFunc) {
	sigintCtx, sigintCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	ctx := contexts.NewContext(sigintCtx)

	ctx, timeoutCancel := ctx.WithTimeout(ctc.Timeout)
	combinedCancel := func() {
		timeoutCancel()
		sigintCancel()
	}

	ctx.WithLogger(contexts.NewLoggerContext(log.NewWithOptions(os.Stdout, ctc.getLogOptions())))

	return ctx, combinedCancel
}
