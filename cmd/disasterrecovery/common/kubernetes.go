package common

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Load interactive authentication providers
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesCommand struct {
	ConfigOverrides    clientcmd.ConfigOverrides
	ExplicitConfigPath string
}

func (kc *KubernetesCommand) ConfigureFlags(cmd *cobra.Command) {
	kubeFlags := pflag.NewFlagSet("kube", pflag.ExitOnError)

	kubeFlags.StringVar(&kc.ExplicitConfigPath, clientcmd.RecommendedConfigPathFlag, "", "Path to the kubeconfig file to use for CLI requests.")

	clientcmd.BindOverrideFlags(&kc.ConfigOverrides, kubeFlags, clientcmd.RecommendedConfigOverrideFlags("kube-"))
	var flagNames []string
	kubeFlags.VisitAll(func(kubeFlag *pflag.Flag) {
		flagNames = append(flagNames, kubeFlag.Name)
	})

	cmd.Flags().AddFlagSet(kubeFlags)
}

// Get the cluster configurations from the following sources, in order of precedence:
// 1. A provided "kubeconfig" flag
// 2. The default kubeconfig file, at ~/.kube/config
// 3. The in-cluster kubeconfig, if running in a pod
func (kc *KubernetesCommand) GetClusterConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kc.ExplicitConfigPath

	var clientConfig clientcmd.ClientConfig
	if term.IsTerminal(int(os.Stdin.Fd())) {
		clientConfig = clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &kc.ConfigOverrides, os.Stdin)
	} else {
		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &kc.ConfigOverrides)
	}

	return clientConfig.ClientConfig()
}
