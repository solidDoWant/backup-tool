package vaultwarden

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravitational/trace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/types"
	"sigs.k8s.io/e2e-framework/pkg/utils"
	"sigs.k8s.io/e2e-framework/support/kind"
)

// TODO look upwards for .git or main.go or something
const repoRoot = "./../../.."

var (
	// Making these global isn't great, but it's how the docs show that
	// setup values should be passed to the test functions
	testenv         env.Environment
	getRegistryName func() string
)

func TestMain(m *testing.M) {
	testenv = env.New()
	// kindClusterName := envconf.RandomName("my-cluster", 16)
	namespace := envconf.RandomName("myns", 16)

	clusterSetup, clusterFinish, clusterName := Cluster()
	registrySetup, registryFinish := Registry(clusterName)
	pushImageSetup, pushImageFinish := PushImage(getRegistryName)

	// Use pre-defined environment funcs to create a kind cluster prior to test run
	testenv.Setup(
		clusterSetup,
		registrySetup,
		pushImageSetup,
		envfuncs.CreateNamespace(namespace),
	)

	// Use pre-defined environment funcs to teardown kind cluster after tests
	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
		pushImageFinish,
		registryFinish,
		clusterFinish,
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

func Cluster() (types.EnvFunc, types.EnvFunc, string) {
	clusterName := envconf.RandomName("my-cluster", 16)

	setup := envfuncs.CreateClusterWithConfig(kind.NewProvider(), clusterName, "kind-config.yaml")

	finish := envfuncs.DestroyCluster(clusterName)

	return setup, finish, clusterName
}

func Registry(clusterName string) (types.EnvFunc, types.EnvFunc) {
	registryContainerName := envconf.RandomName("registry", 16)
	var futurePort *string
	kubePublicNamespace := "kube-public" // This namespace should be created by KinD
	configMapName := "local-registry-hosting"

	getRegistryName = func() string {
		return fmt.Sprintf("localhost:%s", *futurePort)
	}

	// Needs to be a func to capture the value of futurePort when called
	// See https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry#localregistryhosting
	getConfigMap := func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: kubePublicNamespace,
			},
			Data: map[string]string{
				// "localRegistryHosting.v1": fmt.Sprintf(`{"host":%q, "help":%q}`, getRegistryName(), "https://kind.sigs.k8s.io/docs/user/local-registry/"),
				"localRegistryHosting.v1": fmt.Sprintf("host: %q\n", getRegistryName()),
			},
		}
	}

	// This roughly follows https://kind.sigs.k8s.io/docs/user/local-registry/
	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Start the registry container
		registryInternalPort := 5000
		if p := utils.RunCommand(
			fmt.Sprintf("docker run -d --rm --expose %d -p 127.0.0.1:0:%d --name %s registry:2", registryInternalPort, registryInternalPort, registryContainerName),
		); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to deploy registry")
		}

		// Connect the registry container to the cluster network
		if p := utils.RunCommand(
			fmt.Sprintf("docker network connect kind %q", registryContainerName),
		); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to connect registry to cluster network")
		}

		// Get the registry container exposed (localhost) port
		p := utils.RunCommand(fmt.Sprintf("docker port %q %d", registryContainerName, registryInternalPort))
		if p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to get registry port")
		}

		address := strings.TrimSpace(p.Result())
		futurePort = ptr.To(address[strings.LastIndex(address, ":")+1:])

		// Configure the cluster nodes container registry resolver to map "localhost:<port>" to "<registry-container-name>:<port>"
		p = utils.RunCommand(fmt.Sprintf("kind get nodes --name %q", clusterName))
		if p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to get cluster nodes")
		}
		nodes := strings.Split(strings.TrimSpace(p.Result()), "\n")

		// Update the /etc/containerd/certs.d/localhost:<port>/hosts.toml file on each node
		// Docs: https://github.com/containerd/containerd/blob/main/docs/hosts.md
		certsDir := "/etc/containerd/certs.d/localhost:" + *futurePort
		hostsConfigFilePath := filepath.Join(certsDir, "hosts.toml")
		hostsConfigFileContents := fmt.Sprintf("[host.%q]", fmt.Sprintf("http://%s:%d", registryContainerName, registryInternalPort))
		commands := []string{
			fmt.Sprintf("mkdir -p %s", certsDir),
			fmt.Sprintf("echo %q >> %q", hostsConfigFileContents, hostsConfigFilePath),
		}

		script := strings.Join(commands, " && ")
		commandToExec := fmt.Sprintf("sh -c '%s'", script)

		for _, node := range nodes {
			fullExecCommand := fmt.Sprintf("docker exec %q %s", node, commandToExec)
			p := utils.RunCommand(fullExecCommand)
			if p.Err() != nil {
				return ctx, trace.Wrap(p.Err(), "failed to configure %q to use the registry: %s", node, p.Result())
			}
		}

		// Configure the cluster to use the registry
		// Docs: https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry#localregistryhosting
		registryConfigMap := getConfigMap()
		if err := cfg.Client().Resources().Create(ctx, registryConfigMap); err != nil {
			return ctx, trace.Wrap(err, "failed to create %q config map", registryConfigMap.Name)
		}

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		registryConfigMap := getConfigMap()
		if err := cfg.Client().Resources().Delete(ctx, registryConfigMap); err != nil {
			return ctx, trace.Wrap(err, "failed to create %s config map", registryConfigMap.Name)
		}

		if p := utils.RunCommand(fmt.Sprintf("docker stop %s", registryContainerName)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to stop registry")
		}

		return ctx, nil
	}

	return setup, finish
}

func PushImage(getRegistryName func() string) (types.EnvFunc, types.EnvFunc) {
	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Call the makefile target used for releases
		if p := utils.RunCommand(fmt.Sprintf("make -C %q container-manifest CONTAINER_REGISTRY=%s CONTAINER_MANIFEST_PUSH=true", repoRoot, getRegistryName())); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to build image: %s", p.Result())
		}

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Nothing currently to do here. Buildkit does not cache the image.
		return ctx, nil
	}

	return setup, finish
}
