package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVaultWardenCommand(t *testing.T) {
	cmd := NewVaultWardenCommand()
	assert.NotNil(t, cmd)
}

func TestVaultWardenCommandDRCommand(t *testing.T) {
	cmd := NewVaultWardenCommand()
	drCmd := cmd.DRCommand()

	assert.NotNil(t, drCmd)
	assert.Equal(t, "vaultwarden", drCmd.Use)
	assert.NotEmpty(t, drCmd.Short)
	assert.Implements(t, (*SupportsBackup)(nil), cmd)
	assert.Implements(t, (*SupportsConfigSchemaGeneration)(nil), cmd)
}
