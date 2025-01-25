package files

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLocalRuntime(t *testing.T) {
	runtime := NewLocalRuntime()
	require.NotNil(t, runtime)
}
