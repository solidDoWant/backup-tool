package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultWardenDRCommand(t *testing.T) {
	assert.Implements(t, (*DRCommand)(nil), (*VaultWardenDRCommand)(nil))
	assert.Implements(t, (*DRBackupCommand)(nil), (*VaultWardenDRCommand)(nil))
	assert.Implements(t, (*DRRestoreCommand)(nil), (*VaultWardenDRCommand)(nil))
}

func TestNewVaultWardenDRCommand(t *testing.T) {
	cmd := NewVaultWardenDRCommand()
	require.NotNil(t, cmd)
	require.NotNil(t, cmd.ClusterDRCommand)
	assert.NotNil(t, cmd.backupCommand)
	assert.NotNil(t, cmd.restoreCommand)
}

func TestVaultWardenDRCommandName(t *testing.T) {
	assert.Equal(t, "vaultwarden", NewVaultWardenDRCommand().Name())
}

func TestVaultWardenDRCommandGetBackupCommand(t *testing.T) {
	cmd := NewVaultWardenDRCommand().GetBackupCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}

func TestVaultWardenDRCommandGetRestoreCommand(t *testing.T) {
	cmd := NewVaultWardenDRCommand().GetRestoreCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}
