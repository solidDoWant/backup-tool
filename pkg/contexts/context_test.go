package contexts

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContext(t *testing.T) {
	baseCtx := context.Background()
	ctx := NewContext(baseCtx)
	require.NotNil(t, ctx)
	assert.Equal(t, baseCtx, ctx.Context)
}

func TestShallowCopy(t *testing.T) {
	baseCtx := context.Background()
	originalCtx := NewContext(baseCtx)

	copiedCtx := originalCtx.ShallowCopy()
	require.NotNil(t, copiedCtx)
	assert.Equal(t, originalCtx.Context, copiedCtx.Context)

	copiedCtx.Context = context.TODO()
	assert.NotEqual(t, originalCtx.Context, copiedCtx.Context)
	assert.NotEqual(t, originalCtx, copiedCtx)
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

			timeoutCtx, cancel := WithTimeout(parentCtx, tt.timeout)
			defer cancel()

			require.NotNil(t, timeoutCtx)

			_, isDeadlineSet := timeoutCtx.Deadline()
			assert.Equal(t, tt.timeout != 0, isDeadlineSet)
		})
	}
}
