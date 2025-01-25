package cleanup

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCleanupTimeout(t *testing.T) {
	require.NotZero(t, DefaultCleanupTimeout)
}

func TestTo(t *testing.T) {
	testFunc := func() error { return nil }

	cleanup := To(testFunc)
	require.NotNil(t, cleanup)
	require.Equal(t, reflect.ValueOf(testFunc), reflect.ValueOf(cleanup.cleanupLogic))
}

func TestToWithTimeout(t *testing.T) {
	tests := []struct {
		desc    string
		timeout time.Duration
	}{
		{
			desc: "zero timeout",
		},
		{
			desc:    "non-zero timeout",
			timeout: time.Millisecond,
		},
	}

	originalDefaultCleanupTimeout := DefaultCleanupTimeout
	defer func() { DefaultCleanupTimeout = originalDefaultCleanupTimeout }()
	DefaultCleanupTimeout = 10 * time.Millisecond

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			expectedTimeout := tt.timeout
			if expectedTimeout == 0 {
				expectedTimeout = DefaultCleanupTimeout
			}

			called := false
			testFunc := func(ctx context.Context) error {
				called = true
				<-ctx.Done() // Wait for the context to be canceled to simulate a timeout
				return nil
			}

			cleanup := WithTimeoutTo(tt.timeout, testFunc)
			require.NotNil(t, cleanup)

			startTime := time.Now()
			err := cleanup.cleanupLogic()
			endTime := time.Now()

			require.NoError(t, err)
			require.True(t, called)
			// Verify that the correct timeout was set
			require.WithinDuration(t, startTime.Add(expectedTimeout), endTime, 5*time.Millisecond)
		})
	}
}

func TestWithErrMessage(t *testing.T) {
	errMessage := "some message"
	args := []interface{}{"arg1", "arg2"}

	cleanup := To(nil)
	cleanup.WithErrMessage(errMessage, args...)
	require.Equal(t, errMessage, cleanup.onErrArgs[0])
	require.Equal(t, args, cleanup.onErrArgs[1:])
}

func TestWithOriginalErr(t *testing.T) {
	err := assert.AnError

	cleanup := To(nil)
	cleanup.WithOriginalErr(&err)
	require.Equal(t, &err, cleanup.originalErr)
}

func TestRun(t *testing.T) {
	tests := []struct {
		name         string
		cleanupLogic func() error
		errMessage   string
		originalErr  error
		shouldErr    bool
	}{
		{
			name: "nil cleanup logic returns nil",
		},
		{
			name:         "successful cleanup",
			cleanupLogic: func() error { return nil },
		},
		{
			name:         "cleanup error",
			cleanupLogic: func() error { return assert.AnError },
			shouldErr:    true,
		},
		{
			name:         "cleanup error with message",
			cleanupLogic: func() error { return assert.AnError },
			errMessage:   "failed cleanup",
			shouldErr:    true,
		},
		{
			name:         "original error and cleanup error",
			cleanupLogic: func() error { return assert.AnError },
			originalErr:  assert.AnError,
			shouldErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := To(tt.cleanupLogic)

			if tt.errMessage != "" {
				cleanup.WithErrMessage(tt.errMessage)
			}

			var err error
			if tt.originalErr != nil {
				err = tt.originalErr
				cleanup.WithOriginalErr(&err)
			}

			result := cleanup.Run()

			if !tt.shouldErr {
				require.NoError(t, result)
				return
			}
			require.Error(t, result)

			if tt.originalErr == nil {
				return
			}

			require.True(t, trace.IsAggregate(err))
			require.Len(t, trace.Unwrap(err).(trace.Aggregate).Errors(), 2)
		})
	}
}
