package vaultwarden

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
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

	clusterSetup, clusterFinish, clusterName := Cluster()
	registrySetup, registryFinish := Registry(clusterName)
	pushImageSetup, pushImageFinish := PushImage(getRegistryName)
	deployDependentServicesSetup, deployDependentServicesFinish := DeployDependentServices(clusterName)
	vaultWardenSetup, vaultWardenFinish := DeployVaultWarden()

	// Use pre-defined environment funcs to create a kind cluster prior to test run
	testenv.Setup(
		clusterSetup,
		registrySetup,
		pushImageSetup,
		deployDependentServicesSetup,
		vaultWardenSetup,
	)

	// Use pre-defined environment funcs to teardown kind cluster after tests
	testenv.Finish(
		vaultWardenFinish,
		deployDependentServicesFinish,
		pushImageFinish,
		registryFinish,
		clusterFinish,
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

func Cluster() (types.EnvFunc, types.EnvFunc, string) {
	clusterName := envconf.RandomName("my-cluster", 16)

	setup := envfuncs.CreateClusterWithConfig(kind.NewProvider(), clusterName, "config/kind-config.yaml")

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
		// Update the /etc/containerd/certs.d/localhost:<port>/hosts.toml file on each node
		// Docs: https://github.com/containerd/containerd/blob/main/docs/hosts.md
		certsDir := "/etc/containerd/certs.d/localhost:" + *futurePort
		hostsConfigFilePath := filepath.Join(certsDir, "hosts.toml")
		hostsConfigFileContents := fmt.Sprintf("[host.%q]", fmt.Sprintf("http://%s:%d", registryContainerName, registryInternalPort))
		commands := []string{
			fmt.Sprintf("mkdir -p %s", certsDir),
			fmt.Sprintf("echo %q >> %q", hostsConfigFileContents, hostsConfigFilePath),
		}

		err := RunNodesScript(clusterName, commands)
		if err != nil {
			return ctx, trace.Wrap(err, "failed to configure nodes to use the registry")
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
		errors := []error{} // Collect errors to return as an aggregate so that all cleanup steps are attempted

		registryConfigMap := getConfigMap()
		if err := cfg.Client().Resources().Delete(ctx, registryConfigMap); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to delete %s config map", registryConfigMap.Name))
		}

		if p := utils.RunCommand(fmt.Sprintf("docker stop %s", registryContainerName)); p.Err() != nil {
			errors = append(errors, trace.Wrap(p.Err(), "failed to stop registry container"))
		}

		return ctx, trace.NewAggregate(errors...)
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

func Helmfile(helmfilePath string) (types.EnvFunc, types.EnvFunc) {
	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		if p := utils.RunCommand(fmt.Sprintf("helmfile apply --file %q --skip-diff-on-install --suppress-diff --args --skip-schema-validation", helmfilePath)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to deploy helmfile at %q: %s", helmfilePath, p.Result())
		}

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		if p := utils.RunCommand(fmt.Sprintf("helmfile destroy --file %q", helmfilePath)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to remove helmfile at %q: %s", helmfilePath, p.Result())
		}

		return ctx, nil
	}

	return setup, finish
}

func DeployDependentServices(clusterName string) (types.EnvFunc, types.EnvFunc) {
	zpoolName := "openebs-zpool" // This must match the name used in the storage class
	imageFilePath := fmt.Sprintf("/tmp/%s-vdev.img", zpoolName)
	defaultStorageClass := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard"}}
	storageClassPatch := func(isDefault bool) k8s.Patch {
		return k8s.Patch{
			PatchType: k8stypes.StrategicMergePatchType,
			Data:      []byte(fmt.Sprintf(`{"metadata": {"annotations": {"storageclass.kubernetes.io/is-default-class": "%t"}}}`, isDefault)),
		}
	}
	var loopDevice *string // This will be set during setup and used during finish
	helmSetup, helmFinish := Helmfile("./config/dependent-services/helmfile.yaml")

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Deploy ZFS zpool for openebs
		if p := utils.RunCommand(fmt.Sprintf("truncate -s 100G %q", imageFilePath)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to create ZFS pool file: %s", p.Result())
		}

		p := utils.RunCommand(fmt.Sprintf("losetup --show --sector-size 4096 --direct-io --find %q", imageFilePath))
		if p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to attach file to loop device: %s", p.Result())
		}

		loopDevice = ptr.To(strings.TrimSpace(p.Result()))
		if p := utils.RunCommand(fmt.Sprintf("zpool create -f -o ashift=12 %q %q", zpoolName, *loopDevice)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to create ZFS pool: %s", p.Result())
		}

		// Install ZFS userspace utilities in the cluster node containers (needed for openebs-zfs)
		commands := []string{
			fmt.Sprintf("sed -i %q %q", "s/main/main contrib/", "/etc/apt/sources.list.d/debian.sources"),
			"apt update",
			"apt install --no-install-recommends -y zfsutils-linux",
		}
		if err := RunNodesScript(clusterName, commands); err != nil {
			return ctx, trace.Wrap(err, "failed to install ZFS userspace utilities")
		}

		// Remove the "default" annotation from local-path CSI storage class
		if err := cfg.Client().Resources().Patch(ctx, defaultStorageClass, storageClassPatch(false)); err != nil {
			return ctx, trace.Wrap(err, "failed to remove default annotation from storage class %q", defaultStorageClass.Name)
		}

		// Deploy services
		if ctx, err := helmSetup(ctx, cfg); err != nil {
			return ctx, trace.Wrap(err, "failed to deploy dependent services")
		}

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		errors := []error{} // Collect errors to return as an aggregate so that all cleanup steps are attempted

		// Remove the services
		var err error
		if ctx, err = helmFinish(ctx, cfg); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to remove dependent services"))
		}

		// Set the "default" annotation from local-path CSI storage class
		if err := cfg.Client().Resources().Patch(ctx, defaultStorageClass, storageClassPatch(true)); err != nil {
			return ctx, trace.Wrap(err, "failed to set default annotation from storage class %q", defaultStorageClass.Name)
		}

		// Wait for Helm-created pods to be deleted
		// Helmfile will wait for the deployments and daemonsets to be deleted, but not the pods
		pods := &corev1.PodList{}
		if err := cfg.Client().Resources().List(ctx, pods, resources.WithLabelSelector(labels.FormatLabels(map[string]string{"heritage": "Helm"}))); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to list Helm-created pods"))
		} else if err := wait.For(conditions.New(cfg.Client().Resources()).ResourcesDeleted(pods), wait.WithContext(ctx), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute)); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to wait for pods to be deleted"))
		}

		// Uninstall ZFS userspace utilities in the cluster node containers
		if err := RunNodesCommand(clusterName, "apt autoremove --purge -y zfsutils-linux"); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to uninstall ZFS userspace utilities"))
		}

		// Destroy the ZFS pool. Sometimes this may fail and need to be retried.
		if err := wait.For(func(ctx context.Context) (bool, error) {
			return utils.RunCommand(fmt.Sprintf("zpool destroy -f %q", zpoolName)).Err() == nil, nil
		}, wait.WithContext(ctx), wait.WithInterval(2*time.Second), wait.WithTimeout(30*time.Second), wait.WithImmediate()); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to remove ZFS pool"))
		} else {
			// Do the rest of the ZFS cleanup, which will fail if the pool isn't destroyed
			if p := utils.RunCommand(fmt.Sprintf("losetup -d %q", *loopDevice)); p.Err() != nil {
				errors = append(errors, trace.Wrap(p.Err(), "failed to detach loop device: %s", p.Result()))
			}

			if err := os.Remove(imageFilePath); err != nil {
				errors = append(errors, trace.Wrap(err, "failed to remove ZFS pool file"))
			}
		}

		return ctx, trace.NewAggregate(errors...)
	}

	return setup, finish
}

func DeployVaultWarden() (types.EnvFunc, types.EnvFunc) {
	helmSetup, helmFinish := Helmfile("./config/vaultwarden-instance/helmfile.yaml")

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		if ctx, err := helmSetup(ctx, cfg); err != nil {
			return ctx, trace.Wrap(err, "failed to deploy dependent services")
		}

		// Wait for the CNPG cluster to become ready
		cnpgClient, err := versioned.NewForConfig(cfg.Client().RESTConfig())
		if err != nil {
			return ctx, trace.Wrap(err, "failed to create CNPG client")
		}

		if err := wait.For(func(ctx context.Context) (done bool, err error) {
			cnpgCluster, err := cnpgClient.PostgresqlV1().Clusters("default").Get(ctx, "vaultwarden", metav1.GetOptions{})
			if err != nil {
				return false, trace.Wrap(err, "failed to get vaultwarden CNPG cluster")
			}

			for _, condition := range cnpgCluster.Status.Conditions {
				if condition.Type != string(apiv1.ConditionClusterReady) {
					continue
				}

				return condition.Status == metav1.ConditionTrue, nil
			}

			return false, nil
		}, wait.WithContext(ctx), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute)); err != nil {
			return ctx, trace.Wrap(err, "failed to wait for vaultwarden CNPG cluster to be ready")
		}

		// Wait for the Vaultwarden service to become ready, by checking for at least one endpoint
		if err := wait.For(func(ctx context.Context) (done bool, err error) {
			endpoints := &corev1.Endpoints{}
			if err := cfg.Client().Resources().Get(ctx, "vaultwarden", "default", endpoints); err != nil {
				return false, trace.Wrap(err, "failed to get vaultwarden CNPG cluster")
			}

			if len(endpoints.Subsets) == 0 {
				return false, nil
			}

			for _, subset := range endpoints.Subsets {
				if len(subset.Addresses) > 0 {
					return true, nil
				}
			}

			return false, nil
		}, wait.WithContext(ctx), wait.WithImmediate(), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute)); err != nil {
			return ctx, trace.Wrap(err, "failed to wait for vaultwarden service to be ready")
		}

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		errors := []error{} // Collect errors to return as an aggregate so that all cleanup steps are attempted

		// Remove the services
		var err error
		if ctx, err = helmFinish(ctx, cfg); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to remove dependent services"))
		}

		return ctx, trace.NewAggregate(errors...)
	}

	return setup, finish
}

func RunNodesScript(clusterName string, commands []string) error {
	script := strings.Join(commands, " && ")
	containerCommand := fmt.Sprintf("sh -c '%s'", script)
	return RunNodesCommand(clusterName, containerCommand)
}

func RunNodesCommand(clusterName, command string) error {
	p := utils.RunCommand(fmt.Sprintf("kind get nodes --name %q", clusterName))
	if p.Err() != nil {
		return trace.Wrap(p.Err(), "failed to get cluster %q nodes: %s", clusterName, p.Result())
	}

	nodes := strings.Split(strings.TrimSpace(p.Result()), "\n")
	for _, node := range nodes {
		p := utils.RunCommand(fmt.Sprintf("docker exec %q %s", node, command))
		if p.Err() != nil {
			return trace.Wrap(p.Err(), "failed to run command %q against node %q: %s", command, node, p.Result())
		}
	}

	return nil
}
