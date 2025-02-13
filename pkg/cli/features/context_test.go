package features

import (
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogLevelVar(t *testing.T) {
	llv := &logLevelVar{}
	assert.Implements(t, (*pflag.Value)(nil), llv)

	fieldType := strings.ToLower(llv.Type())
	assert.Contains(t, fieldType, "log")
	assert.Contains(t, fieldType, "level")

	assert.Equal(t, "info", llv.String())

	for _, logLevel := range []string{"debug", "info", "warn", "error", "fatal"} {
		assert.NoError(t, llv.Set(logLevel))
		assert.Equal(t, logLevel, llv.String())
	}
	assert.Error(t, llv.Set("invalid"))
}

func TestLogFormatVar(t *testing.T) {
	lfv := &logFormatVar{}
	assert.Implements(t, (*pflag.Value)(nil), lfv)

	fieldType := strings.ToLower(lfv.Type())
	assert.Contains(t, fieldType, "log")
	assert.Contains(t, fieldType, "format")

	assert.Equal(t, "text", lfv.String())

	for _, logFormat := range []string{"text", "json", "logfmt"} {
		assert.NoError(t, lfv.Set(logFormat))
		assert.Equal(t, logFormat, lfv.String())
	}
	assert.Error(t, lfv.Set("invalid"))
}

func TestNewContextCommand(t *testing.T) {
	tests := []struct {
		desc          string
		enableTimeout bool
	}{
		{
			desc:          "timeout enabled",
			enableTimeout: true,
		},
		{
			desc: "timeout disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := NewContentCommand(tt.enableTimeout)

			require.NotNil(t, cc)
			assert.Equal(t, tt.enableTimeout, cc.EnableTimeout)
			assert.Zero(t, cc.Timeout)
			assert.Equal(t, log.InfoLevel, cc.LogLevel)
			assert.Equal(t, log.TextFormatter, cc.Formatter)
		})
	}
}

func TestContextCommandConfigureFlags(t *testing.T) {
	tests := []struct {
		desc          string
		enableTimeout bool
	}{
		{
			desc:          "timeout enabled",
			enableTimeout: true,
		},
		{
			desc: "timeout disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := NewContentCommand(tt.enableTimeout)

			cmd := &cobra.Command{}
			cc.ConfigureFlags(cmd)
			assert.True(t, cmd.Flags().HasAvailableFlags())

			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				switch f.Name {
				case "log-level":
					assert.Equal(t, log.InfoLevel.String(), f.DefValue)
				case "log-format":
					assert.Equal(t, "text", f.DefValue)
				case "timeout":
					if tt.enableTimeout {
						assert.Equal(t, "0s", f.DefValue)
						return
					}
					assert.Fail(t, "timeout flag should not be available")
				default:
					assert.Fail(t, "unexpected flag: %s", f.Name)
				}
			})
		})
	}
}

func TestContextCommandGetLogOptions(t *testing.T) {
	tests := []struct {
		desc            string
		cc              *ContextCommand
		expectedOptions log.Options
	}{
		{
			desc: "info level, text format",
			cc: &ContextCommand{
				logLevelVar: logLevelVar{
					LogLevel: log.InfoLevel,
				},
				logFormatVar: logFormatVar{
					Formatter: log.TextFormatter,
				},
			},
			expectedOptions: log.Options{
				Level:           log.InfoLevel,
				Formatter:       log.TextFormatter,
				ReportTimestamp: true,
			},
		},

		{
			desc: "debug level, json format",
			cc: &ContextCommand{
				logLevelVar: logLevelVar{
					LogLevel: log.DebugLevel,
				},
				logFormatVar: logFormatVar{
					Formatter: log.JSONFormatter,
				},
			},
			expectedOptions: log.Options{
				Level:           log.DebugLevel,
				Formatter:       log.JSONFormatter,
				ReportCaller:    true,
				ReportTimestamp: true,
			},
		},

		{
			desc: "warn level, logfmt format",
			cc: &ContextCommand{
				logLevelVar: logLevelVar{
					LogLevel: log.WarnLevel,
				},
				logFormatVar: logFormatVar{
					Formatter: log.LogfmtFormatter,
				},
			},
			expectedOptions: log.Options{
				Level:     log.WarnLevel,
				Formatter: log.LogfmtFormatter,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			options := tt.cc.getLogOptions()
			assert.Equal(t, tt.expectedOptions, options)
		})
	}
}

func TestContextCommandGetCommandContext(t *testing.T) {
	ctc := NewContentCommand(false)
	ctx, cancel := ctc.GetCommandContext()
	require.NotNil(t, cancel)
	defer cancel()

	assert.NotNil(t, ctx)

	// Verify that cancelleation works
	cancel()
	select {
	case <-ctx.Done():
	default:
		assert.Fail(t, "context was not cancelled")
	}
}
