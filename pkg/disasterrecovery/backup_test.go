package disasterrecovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewBackupNow(t *testing.T) {
	name := "test-backup"
	backup := NewBackupNow(name)

	require.NotNil(t, backup)
	require.Equal(t, name, backup.Name)
	require.WithinDuration(t, time.Now(), backup.StartTime, 3*time.Second)
}

func TestBackupGetFullName(t *testing.T) {
	backup := &Backup{
		Name:      "test-backup",
		StartTime: time.Date(2023, 1, 2, 15, 4, 5, 0, time.UTC),
	}
	expected := "test-backup-2023-01-02T15.04.05Z"
	require.Equal(t, expected, backup.GetFullName())
}

func TestBackupStop(t *testing.T) {
	backup := NewBackupNow("test-backup")
	require.False(t, backup.HasCompleted())

	backup.Stop()
	require.True(t, backup.HasCompleted())
}

func TestBackupCalculateRuntime(t *testing.T) {
	backup := &Backup{
		Name:      "test-backup",
		StartTime: time.Now().Add(-5 * time.Second),
	}

	// Test incomplete backup
	runtime := backup.CalculateRuntime()
	require.InDelta(t, 5.0, runtime.Seconds(), 1.0)

	// Test completed backup
	backup.Stop()
	runtime = backup.CalculateRuntime()
	require.InDelta(t, 5.0, runtime.Seconds(), 1.0)
}
