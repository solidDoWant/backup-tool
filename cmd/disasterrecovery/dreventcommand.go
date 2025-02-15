package disasterrecovery

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/gravitational/trace"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type DREventCommand interface {
	Run() error
	ConfigureFlags(cmd *cobra.Command)
}

type DREventGenerateSchemaCommand interface {
	DREventCommand
	GenerateConfigSchema() ([]byte, error)
}

func buildGenerateConfigSchemaCommand(genSchemaCmd DREventGenerateSchemaCommand, outputWriter io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "gen-config-schema",
		Short: "Generate the configuration file schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			schema, err := genSchemaCmd.GenerateConfigSchema()
			if err != nil {
				return trace.Wrap(err, "failed to generate config schema")
			}

			_, err = io.Copy(outputWriter, bytes.NewReader(schema))
			return trace.Wrap(err, "failed to write config schema to stdout")
		},
		SilenceUsage: true,
	}
}

func buildDREventCommand(drCmd DREventCommand, drName, drEventName string) *cobra.Command {
	eventCmd := &cobra.Command{
		Use:   drEventName,
		Short: fmt.Sprintf("%s for %s", cases.Title(language.Und).String(drEventName), drName),
	}

	runCommand := &cobra.Command{
		Use:   "run",
		Short: fmt.Sprintf("Perform the %s %s", drName, drEventName),
		RunE: func(cmd *cobra.Command, args []string) error {
			return drCmd.Run()
		},
	}
	eventCmd.AddCommand(runCommand)
	drCmd.ConfigureFlags(runCommand)

	if genSchemaCmd, ok := drCmd.(DREventGenerateSchemaCommand); ok {
		eventCmd.AddCommand(buildGenerateConfigSchemaCommand(genSchemaCmd, os.Stdout))
	}

	return eventCmd
}
