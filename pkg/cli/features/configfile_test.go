package features

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCOnfigFileCommandConfigureFlags(t *testing.T) {
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
