package features

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestKubernetesCommand(t *testing.T) {
	assert.Implements(t, (*KubernetesCommandInterface)(nil), &KubernetesCommand{})
}

func TestNewKubernetesCommand(t *testing.T) {
	assert.NotNil(t, NewKubernetesCommand())
}

func TestKubernetesCommandConfigureFlags(t *testing.T) {
	kc := &KubernetesCommand{}

	cmd := &cobra.Command{}
	kc.ConfigureFlags(cmd)
	assert.True(t, cmd.Flags().HasAvailableFlags())
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		assert.True(t, strings.HasPrefix(f.Name, "kube"))
	})
}
