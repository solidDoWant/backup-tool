package postgres

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLocalRuntime(t *testing.T) {
	runtime := NewLocalRuntime()
	require.NotNil(t, runtime)

	// Verify that the NewCmdWrapper will be used when not under test
	// For details, see https://github.com/stretchr/testify/issues/182#issuecomment-495359313
	require.Equal(t, reflect.ValueOf(NewCmdWrapper), reflect.ValueOf(runtime.wrapCommand))

	require.Equal(t, os.Stderr, runtime.errOutputWriter)
}
