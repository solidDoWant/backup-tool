package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateConfigSchema(t *testing.T) {
	type testConfig struct {
		Name     string `jsonschema:"required"`
		Optional string
	}
	expectedSchema := []byte(`{` +
		`"$schema":"https://json-schema.org/draft/2020-12/schema",` +
		`"$id":"https://github.com/solidDoWant/backup-tool/cmd/disasterrecovery/common/test-config",` +
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
