package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericDRCommand(t *testing.T) {
	assert.Implements(t, (*DRCommand)(nil), (*GenericDRCommand)(nil))
	assert.Implements(t, (*DRBackupCommand)(nil), (*GenericDRCommand)(nil))
	assert.Implements(t, (*DRRestoreCommand)(nil), (*GenericDRCommand)(nil))
}

func TestNewGenericDRCommand(t *testing.T) {
	cmd := NewGenericDRCommand()
	require.NotNil(t, cmd)
	require.NotNil(t, cmd.ClusterDRCommand)
	assert.NotNil(t, cmd.backupCommand)
	assert.NotNil(t, cmd.restoreCommand)
}

func TestGenericDRCommandName(t *testing.T) {
	assert.Equal(t, "generic", NewGenericDRCommand().Name())
}

func TestGenericDRCommandGetBackupCommand(t *testing.T) {
	cmd := NewGenericDRCommand().GetBackupCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}

func TestGenericDRCommandGetRestoreCommand(t *testing.T) {
	cmd := NewGenericDRCommand().GetRestoreCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}
