package features

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFileCommand(t *testing.T) {
	cfc := &ConfigFileCommand[interface{}]{} // Type does not matter here
	assert.Implements(t, (*ConfigFileCommandInterface[interface{}])(nil), cfc)
}

func TestNewConfigFileCommand(t *testing.T) {
	cfc := NewConfigFileCommand[interface{}]()
	require.NotNil(t, cfc)
}

func TestConfigFileCommandConfigureFlags(t *testing.T) {
	cfc := &ConfigFileCommand[string]{} // Type does not matter here

	cmd := &cobra.Command{}
	cfc.ConfigureFlags(cmd)
	assert.True(t, cmd.Flags().HasAvailableFlags())
}

func TestGenerateConfigSchema(t *testing.T) {
	type testConfig struct {
		Name     string `jsonschema:"required"`
		Optional string
	}
	expectedSchema := []byte(`{` +
		`"$schema":"https://json-schema.org/draft/2020-12/schema",` +
		`"$id":"https://github.com/solidDoWant/backup-tool/pkg/cli/features/test-config",` +
		`"$ref":"#/$defs/testConfig",` +
		`"$defs":{` +
		`"testConfig":{` +
		`"properties":{` +
		`"Name":{"type":"string"},"Optional":{"type":"string"}` +
		`},` +
		`"additionalProperties":false,` +
		`"type":"object",` +
		`"required":["Name"]` +
		`}}}`)

	cmd := &ConfigFileCommand[testConfig]{}
	generatedSchema, err := cmd.GenerateConfigSchema()

	require.NoError(t, err)
	require.Equal(t, expectedSchema, generatedSchema)
}

type validatableTestConfig struct {
	Name string `yaml:"name" jsonschema:"required"`
}

func (c validatableTestConfig) Validate() error {
	if c.Name == "invalid" {
		return assert.AnError
	}
	return nil
}

func TestReadConfigFile(t *testing.T) {
	tests := []struct {
		desc     string
		contents string
		wantErr  bool
	}{
		{
			desc:     "valid config",
			contents: "name: valid",
		},
		{
			desc:     "fails tag validation",
			contents: "{}",
			wantErr:  true,
		},
		{
			desc:     "fails Validate method",
			contents: "name: invalid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			configFilePath := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(configFilePath, []byte(tt.contents), 0600))

			cfc := &ConfigFileCommand[validatableTestConfig]{ConfigFilePath: configFilePath}
			config, err := cfc.ReadConfigFile(t.Context())
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "valid", config.Name)
		})
	}
}
