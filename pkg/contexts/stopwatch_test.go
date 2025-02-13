package contexts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStopwatchContext(t *testing.T) {
	stopwatch := NewStopwatchContext()
	require.NotNil(t, stopwatch)
	assert.WithinDuration(t, stopwatch.StartTime, time.Now(), time.Millisecond)
}

func TestStopwatchContextElapsed(t *testing.T) {
	stopwatch := NewStopwatchContext()
	time.Sleep(10 * time.Millisecond)

	elapsedTime := stopwatch.Elapsed()
	assert.GreaterOrEqual(t, elapsedTime, 10*time.Millisecond)
	assert.LessOrEqual(t, elapsedTime, 11*time.Millisecond)
}

func TestStopwatchContextKeyval(t *testing.T) {
	stopwatch := NewStopwatchContext()
	time.Sleep(10 * time.Millisecond)

	keyval := stopwatch.Keyval()
	require.NotNil(t, keyval)
	require.NotNil(t, keyval.Value)

	assert.Equal(t, "runtime", keyval.Key)

	value := keyval.Value()
	require.IsType(t, time.Duration(0), value)

	assert.GreaterOrEqual(t, value.(time.Duration), 10*time.Millisecond)
	assert.LessOrEqual(t, value.(time.Duration), 11*time.Millisecond)
}
