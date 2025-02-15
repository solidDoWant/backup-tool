package features

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKubeClusterCommand(t *testing.T) {
	assert.Implements(t, (*KubeClusterCommandInterface)(nil), &KubeClusterCommand{})
}

func TestNewKubeClusterCommand(t *testing.T) {
	assert.NotNil(t, NewKubeClusterCommand())
}
