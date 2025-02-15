package disasterrecovery

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildGenerateConfigSchemaCommand(t *testing.T) {
	// Test the command with schema generation succeeding
	schemaBytes := []byte("schema")
	schemaGeneratorCmd := NewMockDREventGenerateSchemaCommand(t)
	schemaGeneratorCmd.EXPECT().GenerateConfigSchema().Return(schemaBytes, nil)

	var outWriter bytes.Buffer
	cmd := buildGenerateConfigSchemaCommand(schemaGeneratorCmd, &outWriter)
	require.NotNil(t, cmd)

	err := cmd.RunE(nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, schemaBytes, outWriter.Bytes())

	// Test the command with schema generation failing
	outWriter.Reset()
	schemaGeneratorCmd.EXPECT().GenerateConfigSchema().Unset()
	schemaGeneratorCmd.EXPECT().GenerateConfigSchema().Return(nil, assert.AnError)
	err = cmd.RunE(nil, nil)
	assert.Error(t, err)
	assert.Empty(t, outWriter.Bytes())
}

func TestBuildDREventCommand(t *testing.T) {
	tests := []struct {
		desc                 string
		returnedError        error
		shouldIncludeSchema  bool
		expectedCommandCount int
	}{
		{
			desc:                 "no schema generation, no error",
			expectedCommandCount: 1,
		},
		{
			desc:                 "schema generation, no error",
			shouldIncludeSchema:  true,
			expectedCommandCount: 2,
		},
		{
			desc:                 "no schema generation, error",
			returnedError:        assert.AnError,
			expectedCommandCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Unfortunately the two mocks d ont share a common mock interface,
			// so this basically identical logic is needed to create the correct mock
			var eventCmd DREventCommand
			if tt.shouldIncludeSchema {
				mockEventCmd := NewMockDREventGenerateSchemaCommand(t)
				mockEventCmd.EXPECT().Run().Return(tt.returnedError)
				mockEventCmd.EXPECT().ConfigureFlags(mock.Anything)
				eventCmd = mockEventCmd
			} else {
				mockEventCmd := NewMockDREventCommand(t)
				mockEventCmd.EXPECT().Run().Return(tt.returnedError)
				mockEventCmd.EXPECT().ConfigureFlags(mock.Anything)
				eventCmd = mockEventCmd
			}

			cmd := buildDREventCommand(eventCmd, "test", "event")
			require.NotNil(t, cmd)
			assert.Len(t, cmd.Commands(), tt.expectedCommandCount)

			// Test the run command
			var runCommand *cobra.Command
			for _, c := range cmd.Commands() {
				if c.Use == "run" {
					runCommand = c
					break
				}
			}
			require.NotNil(t, runCommand)

			err := runCommand.RunE(nil, nil)
			assert.Equal(t, tt.returnedError, err)
		})
	}
}
