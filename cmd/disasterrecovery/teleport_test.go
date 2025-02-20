package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeleportDRCommand(t *testing.T) {
	assert.Implements(t, (*DRCommand)(nil), (*TeleportDRCommand)(nil))
	assert.Implements(t, (*DRBackupCommand)(nil), (*TeleportDRCommand)(nil))
	assert.Implements(t, (*DRRestoreCommand)(nil), (*TeleportDRCommand)(nil))
}

func TestNewTeleportDRCommand(t *testing.T) {
	cmd := NewTeleportDRCommand()
	require.NotNil(t, cmd)
	require.NotNil(t, cmd.ClusterDRCommand)
	assert.NotNil(t, cmd.backupCommand)
	assert.NotNil(t, cmd.restoreCommand)
}

func TestTeleportDRCommandName(t *testing.T) {
	assert.Equal(t, "teleport", NewTeleportDRCommand().Name())
}

func TestTeleportDRCommandGetBackupCommand(t *testing.T) {
	cmd := NewTeleportDRCommand().GetBackupCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}

func TestTeleportDRCommandGetRestoreCommand(t *testing.T) {
	cmd := NewTeleportDRCommand().GetRestoreCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}
