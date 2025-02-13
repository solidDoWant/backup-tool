package contexts

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContext(t *testing.T) {
	tests := []struct {
		desc              string
		ctx               context.Context
		shouldParentBeSet bool
	}{
		{
			desc: "nil context",
			ctx:  nil,
		},
		{
			desc: "context.Context context",
			ctx:  context.Background(),
		},
		{
			desc:              "Context context",
			ctx:               NewContext(context.Background()),
			shouldParentBeSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := NewContext(tt.ctx)
			require.NotNil(t, ctx)
			assert.Equal(t, tt.ctx, ctx.Context)
			assert.NotNil(t, ctx.Stopwatch)
			assert.NotNil(t, ctx.Log)
			assert.Equal(t, nullLogger, ctx.Log.Logger)

			if tt.shouldParentBeSet {
				assert.Equal(t, tt.ctx, ctx.parentCtx)
			} else {
				assert.Nil(t, ctx.parentCtx)
			}
		})
	}
}

func TestWithLogger(t *testing.T) {
	ctx := NewContext(context.Background())
	logger := NewLoggerContext(log.Default())

	returnedCtx := ctx.WithLogger(logger)
	require.NotNil(t, returnedCtx)
	assert.Equal(t, logger, returnedCtx.Log)
	assert.Equal(t, ctx, returnedCtx)
}

func TestChild(t *testing.T) {
	ctx := NewContext(context.Background())
	childCtx := ctx.Child()

	require.NotNil(t, childCtx)
	assert.NotEqual(t, ctx, childCtx)
	assert.NotNil(t, childCtx.Stopwatch)
	assert.NotNil(t, childCtx.Log)
	assert.Equal(t, ctx, childCtx.parentCtx)
}

func TestWithTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{
			name:    "with zero timeout uses WithCancel",
			timeout: 0,
		},
		{
			name:    "with non-zero timeout uses WithTimeout",
			timeout: time.Millisecond * 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentCtx := NewContext(context.Background())

			timeoutCtx, cancel := parentCtx.WithTimeout(tt.timeout)
			defer cancel()

			require.NotNil(t, timeoutCtx)

			_, isDeadlineSet := timeoutCtx.Deadline()
			assert.Equal(t, tt.timeout != 0, isDeadlineSet)
		})
	}
}

func TestIsChildOf(t *testing.T) {
	parentCtx := NewContext(context.Background())
	childCtx := parentCtx.Child()
	grandchildCtx := childCtx.Child()

	assert.False(t, parentCtx.IsChildOf(nil))
	assert.True(t, childCtx.IsChildOf(parentCtx))
	assert.False(t, parentCtx.IsChildOf(childCtx))
	assert.True(t, grandchildCtx.IsChildOf(parentCtx))
}

func TestHandlerContexts(t *testing.T) {
	handlerCtx := context.Background()
	realCtx := NewContext(context.TODO())

	wrappedCtx := WrapHandlerContext(handlerCtx, realCtx)
	require.NotNil(t, wrappedCtx)
	assert.Equal(t, realCtx, wrappedCtx)

	unwrappedCtx := UnwrapHandlerContext(wrappedCtx)
	require.NotNil(t, unwrappedCtx)
	assert.Equal(t, realCtx, unwrappedCtx)

	badUnwrappedCtx := UnwrapHandlerContext(handlerCtx)
	// Don't crash, but do log the problem
	require.NotNil(t, badUnwrappedCtx)
}
