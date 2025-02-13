package cleanup

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCleanupTimeout(t *testing.T) {
	require.NotZero(t, DefaultCleanupTimeout)
}

func TestTo(t *testing.T) {
	testFunc := func(*contexts.Context) error { return nil }

	cleanup := To(testFunc)
	require.NotNil(t, cleanup)
	require.Equal(t, reflect.ValueOf(testFunc), reflect.ValueOf(cleanup.cleanupLogic))
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

func TestWithParentCtx(t *testing.T) {
	parentCtx := contexts.NewContext(context.Background())

	cleanup := To(nil)
	cleanup.WithParentCtx(parentCtx)
	require.Equal(t, parentCtx, cleanup.parentCtx)
}

func TestWithTimeout(t *testing.T) {
	timeout := 10 * time.Second

	cleanup := To(nil)
	cleanup.WithTimeout(timeout)
	require.Equal(t, timeout, cleanup.timeout)
}

func TestBuildContext(t *testing.T) {
	tests := []struct {
		desc string
		c    *Cleanup
	}{
		{
			desc: "nil parent context",
			c:    To(nil),
		},
		{
			desc: "parent context",
			c:    To(nil).WithParentCtx(contexts.NewContext(context.Background())),
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx, cancel := tt.c.buildContext()
			defer cancel()

			require.NotNil(t, ctx)
			require.NotNil(t, cancel)
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name         string
		cleanupLogic func(*contexts.Context) error
		errMessage   string
		originalErr  error
		shouldErr    bool
	}{
		{
			name: "nil cleanup logic returns nil",
		},
		{
			name:         "successful cleanup",
			cleanupLogic: func(*contexts.Context) error { return nil },
		},
		{
			name:         "cleanup error",
			cleanupLogic: func(*contexts.Context) error { return assert.AnError },
			shouldErr:    true,
		},
		{
			name:         "cleanup error with message",
			cleanupLogic: func(*contexts.Context) error { return assert.AnError },
			errMessage:   "failed cleanup",
			shouldErr:    true,
		},
		{
			name:         "original error and cleanup error",
			cleanupLogic: func(*contexts.Context) error { return assert.AnError },
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
