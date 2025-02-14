package disasterrecovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewDREventNow(t *testing.T) {
	name := "test-backup"
	drEvent := NewDREventNow(name)

	require.NotNil(t, drEvent)
	require.Equal(t, name, drEvent.Name)
	require.WithinDuration(t, time.Now(), drEvent.StartTime, 3*time.Second)
}

func TestDREventGetFullName(t *testing.T) {
	drEvent := &DREvent{
		Name:      "test-backup",
		StartTime: time.Date(2023, 1, 2, 15, 4, 5, 0, time.UTC),
	}
	expected := "test-backup-2023-01-02T15.04.05Z"
	require.Equal(t, expected, drEvent.GetFullName())
}

func TestDREventStop(t *testing.T) {
	drEvent := NewDREventNow("test-backup")
	require.False(t, drEvent.HasCompleted())

	drEvent.Stop()
	require.True(t, drEvent.HasCompleted())
}

func TestDREventCalculateRuntime(t *testing.T) {
	drEvent := &DREvent{
		Name:      "test-backup",
		StartTime: time.Now().Add(-5 * time.Second),
	}

	// Test incomplete DR event
	runtime := drEvent.CalculateRuntime()
	require.InDelta(t, 5.0, runtime.Seconds(), 1.0)

	// Test completed DR event
	drEvent.Stop()
	runtime = drEvent.CalculateRuntime()
	require.InDelta(t, 5.0, runtime.Seconds(), 1.0)
}
