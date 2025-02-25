package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthentikDRCommand(t *testing.T) {
	assert.Implements(t, (*DRCommand)(nil), (*AuthentikDRCommand)(nil))
	assert.Implements(t, (*DRBackupCommand)(nil), (*AuthentikDRCommand)(nil))
	assert.Implements(t, (*DRRestoreCommand)(nil), (*AuthentikDRCommand)(nil))
}

func TestNewAuthentikDRCommand(t *testing.T) {
	cmd := NewAuthentikDRCommand()
	require.NotNil(t, cmd)
	require.NotNil(t, cmd.ClusterDRCommand)
	assert.NotNil(t, cmd.backupCommand)
	assert.NotNil(t, cmd.restoreCommand)
}

func TestAuthentikDRCommandName(t *testing.T) {
	assert.Equal(t, "authentik", NewAuthentikDRCommand().Name())
}

func TestAuthentikDRCommandGetBackupCommand(t *testing.T) {
	cmd := NewAuthentikDRCommand().GetBackupCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}

func TestAuthentikDRCommandGetRestoreCommand(t *testing.T) {
	cmd := NewAuthentikDRCommand().GetRestoreCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}
