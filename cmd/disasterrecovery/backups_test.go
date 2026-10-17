package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupsDRCommand(t *testing.T) {
	assert.Implements(t, (*DRCommand)(nil), (*BackupsDRCommand)(nil))
	assert.Implements(t, (*DRCleanupCommand)(nil), (*BackupsDRCommand)(nil))
	// Backups aren't an application, so there is nothing to back up or restore
	assert.NotImplements(t, (*DRBackupCommand)(nil), (*BackupsDRCommand)(nil))
	assert.NotImplements(t, (*DRRestoreCommand)(nil), (*BackupsDRCommand)(nil))
}

func TestNewBackupsDRCommand(t *testing.T) {
	cmd := NewBackupsDRCommand()
	require.NotNil(t, cmd)
	assert.NotNil(t, cmd.cleanupCommand)
}

func TestBackupsDRCommandName(t *testing.T) {
	assert.Equal(t, "backups", NewBackupsDRCommand().Name())
}

func TestBackupsDRCommandGetCleanupCommand(t *testing.T) {
	cmd := NewBackupsDRCommand().GetCleanupCommand()
	require.NotNil(t, cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}

func TestBuildBackupsDRCommand(t *testing.T) {
	cmd := buildDRCommand(NewBackupsDRCommand())
	require.NotNil(t, cmd)
	assert.Equal(t, "backups", cmd.Name())

	require.Len(t, cmd.Commands(), 1)
	cleanupCmd := cmd.Commands()[0]
	assert.Equal(t, "cleanup", cleanupCmd.Name())

	subcommandNames := make([]string, 0, len(cleanupCmd.Commands()))
	for _, subcommand := range cleanupCmd.Commands() {
		subcommandNames = append(subcommandNames, subcommand.Name())
	}
	assert.ElementsMatch(t, []string{"run", "gen-config-schema"}, subcommandNames)
}
