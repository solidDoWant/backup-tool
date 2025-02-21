package s3

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalRuntime(t *testing.T) {
	assert.Implements(t, (*Runtime)(nil), new(LocalRuntime))
}

func TestNewLocalRuntime(t *testing.T) {
	runtime := NewLocalRuntime()
	assert.NotNil(t, runtime)
	assert.NotNil(t, runtime.newSyncManager)
}
