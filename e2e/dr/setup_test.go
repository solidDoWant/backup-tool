package dr

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gravitational/trace"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/types"
	"sigs.k8s.io/e2e-framework/pkg/utils"
	"sigs.k8s.io/e2e-framework/support/kind"
)

const (
	repoRoot  = "./../.."       // TODO look upwards for .git or main.go or something
	zpoolName = "openebs-zpool" // This must match the name used in the storage class
)

var (
	// Making these global isn't great, but it's how the docs show that
	// setup values should be passed to the test functions
	testenv env.Environment

	// These values are set during various setup stages, and will cause panic if used too early
	registryName      string
	imageName         string
	chartPath         string
	s3AccessKeyId     string
	s3SecretAccessKey string
)

func TestMain(m *testing.M) {
	testenv = env.New()

	clusterSetup, clusterFinish, clusterName := Cluster()
	registrySetup, registryFinish := Registry(clusterName)
	pushImageSetup, pushImageFinish := PushImage()
	buildChartSetup, buildChartFinish := BuildChart()
	zfsPoolSetup, zfsPoolFinish := ZfsPool(clusterName)
	deploySnapshotVGSSetup, deploySnapshotVGSFinish := DeploySnapshotVGS()
	deployDependentServicesSetup, deployDependentServicesFinish := DeployDependentServices(clusterName)
	addTestHelmReposSetup, addTestHelmReposFinish := AddTestHelmRepos()

	// The cluster must exist before anything else (the registry attaches to its network,
	// dependent services deploy into it, and the image is pushed to a registry on its
	// network). Once it's up, the remaining setup steps are independent of one another
	// and run concurrently so the slow image build overlaps the slow dependent-services
	// deploy (seaweedfs in particular) instead of running after it. The registry must be
	// up before the image is pushed, so those two stay sequential within their branch.
	testenv.Setup(
		timed("cluster", clusterSetup),
		Parallel(
			Sequential(timed("registry", registrySetup), timed("push-image", pushImageSetup)),
			// The ZFS pool must exist before the dependent services (openebs consumes it), and the snapshot
			// stack + host-path driver must too (its CRDs must exist before openebs' csi-snapshotter sidecar
			// starts). Those two are independent, so they run concurrently before dependent-services.
			Sequential(
				Parallel(timed("zfs-pool", zfsPoolSetup), timed("snapshot-vgs", deploySnapshotVGSSetup)),
				timed("dependent-services", deployDependentServicesSetup),
			),
			timed("build-chart", buildChartSetup),
			timed("add-test-helm-repos", addTestHelmReposSetup),
		),
	)

	// Teardown mirrors setup: tear down the in-cluster/host resources concurrently, then
	// destroy the cluster last. These are registered as two separate Finish actions on
	// purpose: a Finish action stops at its first erroring func, so if the parallel group
	// fails (e.g. a flaky resource cleanup) the cluster destroy must be its own action to
	// still run and avoid leaking the kind cluster.
	testenv.Finish(
		Parallel(
			Sequential(timed("push-image-finish", pushImageFinish), timed("registry-finish", registryFinish)),
			timed("dependent-services-finish", deployDependentServicesFinish),
			timed("snapshot-vgs-finish", deploySnapshotVGSFinish),
			timed("build-chart-finish", buildChartFinish),
			timed("add-test-helm-repos-finish", addTestHelmReposFinish),
		),
	)
	testenv.Finish(
		timed("cluster-finish", clusterFinish),
	)
	// The host-side ZFS pool is torn down last, after the cluster is gone (see ZfsPool for why).
	testenv.Finish(
		timed("zfs-pool-finish", zfsPoolFinish),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

// Sequential runs the given env funcs in order, threading the context through each. It
// stops and returns on the first error.
func Sequential(funcs ...types.EnvFunc) types.EnvFunc {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		var err error
		for _, fn := range funcs {
			if ctx, err = fn(ctx, cfg); err != nil {
				return ctx, err
			}
		}
		return ctx, nil
	}
}

// Parallel runs the given env funcs concurrently against the same incoming context,
// returning that context once all have completed and aggregating any errors. The branches
// must be independent: they may read cfg and mutate package-level globals, but must not
// depend on context values produced by one another (the returned contexts are discarded).
func Parallel(funcs ...types.EnvFunc) types.EnvFunc {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		var wg sync.WaitGroup
		errs := make([]error, len(funcs))
		for i, fn := range funcs {
			wg.Go(func() {
				_, errs[i] = fn(ctx, cfg)
			})
		}
		wg.Wait()
		return ctx, trace.NewAggregate(errs...)
	}
}

// timed wraps an env func to log when it starts and how long it took. This makes the
// per-phase setup/teardown cost visible in `go test -v` output so future slow steps are
// easy to spot.
func timed(name string, fn types.EnvFunc) types.EnvFunc {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		start := time.Now()
		log.Printf("[e2e] %s: starting", name)
		ctx, err := fn(ctx, cfg)
		log.Printf("[e2e] %s: done in %s (failed=%t)", name, time.Since(start).Round(time.Second), err != nil)
		return ctx, err
	}
}

func Cluster() (types.EnvFunc, types.EnvFunc, string) {
	clusterName := envconf.RandomName("my-cluster", 16)

	setup := envfuncs.CreateClusterWithConfig(kind.NewProvider(), clusterName, "config/setup/kind-config.yaml")

	finish := envfuncs.DestroyCluster(clusterName)

	return setup, finish, clusterName
}

func Registry(clusterName string) (types.EnvFunc, types.EnvFunc) {
	registryContainerName := envconf.RandomName("registry", 16)
	kubePublicNamespace := "kube-public" // This namespace should be created by KinD
	configMapName := "local-registry-hosting"

	// Needs to be a func to capture the value of registryName when called
	// See https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry#localregistryhosting
	getConfigMap := func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: kubePublicNamespace,
			},
			Data: map[string]string{
				"localRegistryHosting.v1": fmt.Sprintf("host: %q\n", registryName),
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
		port := address[strings.LastIndex(address, ":")+1:]
		registryName = fmt.Sprintf("localhost:%s", port)

		// Configure the cluster nodes container registry resolver to map "localhost:<port>" to "<registry-container-name>:<port>"
		// Update the /etc/containerd/certs.d/localhost:<port>/hosts.toml file on each node
		// Docs: https://github.com/containerd/containerd/blob/main/docs/hosts.md
		certsDir := "/etc/containerd/certs.d/localhost:" + port
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

func PushImage() (types.EnvFunc, types.EnvFunc) {
	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Call the makefile target used for releases, but only build the image for the
		// architecture of the host the kind cluster runs on. The release flow builds a
		// multi-arch manifest (linux/amd64 + linux/arm64), but the e2e cluster only ever
		// pulls the local arch. Building the non-native arch costs minutes because the
		// Dockerfile's `apt install` step runs under QEMU emulation, and that image is
		// never used here. Restricting to the local platform removes that wasted work.
		localPlatform := "linux/" + runtime.GOARCH
		if p := utils.RunCommand(fmt.Sprintf("make -C %q container-manifest CONTAINER_REGISTRY=%s CONTAINER_MANIFEST_PUSH=true CONTAINER_PLATFORMS=%s", repoRoot, registryName, localPlatform)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to build image: %s", p.Result())
		}

		// Set the image name
		p := utils.RunCommand(fmt.Sprintf("make -C %q --no-print-directory print-container-image-tag CONTAINER_REGISTRY=%s", repoRoot, registryName))
		if p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to get image name: %s", p.Result())
		}
		imageName = strings.TrimSpace(p.Result())

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Clean up built binaries. These will have the image repository baked in. TODO make this configurable
		if p := utils.RunCommand(fmt.Sprintf("make -C %q clean CONTAINER_REGISTRY=%s", repoRoot, registryName)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to clean up built binaries: %s", p.Result())
		}

		return ctx, nil
	}

	return setup, finish
}

func BuildChart() (types.EnvFunc, types.EnvFunc) {
	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Call the makefile target used for releases
		if p := utils.RunCommand(fmt.Sprintf("make -C %q helm", repoRoot)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to build chart: %s", p.Result())
		}

		// Set the chart path
		p := utils.RunCommand(fmt.Sprintf("make -C %q --no-print-directory print-chart-path", repoRoot))
		if p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to get chart path: %s", p.Result())
		}
		chartPath = strings.TrimSpace(p.Result())

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Nothing currently to do here
		return ctx, nil
	}

	return setup, finish
}

func Helmfile(helmfilePath string) (types.EnvFunc, types.EnvFunc) {
	// Each helmfile invocation gets its own helm repository config + cache. The DR suites run
	// in parallel, and `helmfile apply` runs `helm repo add` for its declared repositories;
	// without isolation those concurrent writes corrupt the shared
	// ~/.config/helm/repositories.yaml ("Adding repo ..." failures). A private HELM home per
	// invocation removes the shared-state race. Config paths contain no spaces, so the command
	// is wrapped in a single-quoted `sh -c` to carry the env without quote nesting.
	helmHome, mkdirErr := os.MkdirTemp("", "e2e-helm-")
	helmEnv := fmt.Sprintf("HELM_REPOSITORY_CONFIG=%s/repositories.yaml HELM_REPOSITORY_CACHE=%s/cache", helmHome, helmHome)

	run := func(ctx context.Context, helmfileCommand string) (context.Context, error) {
		if mkdirErr != nil {
			return ctx, trace.Wrap(mkdirErr, "failed to create temporary helm home")
		}

		command := fmt.Sprintf("sh -c '%s %s'", helmEnv, helmfileCommand)
		if p := utils.RunCommand(command); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to run %q for helmfile %q: %s", helmfileCommand, helmfilePath, p.Result())
		}

		return ctx, nil
	}

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return run(ctx, fmt.Sprintf("helmfile apply --file %s --skip-diff-on-install --suppress-diff", helmfilePath))
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		ctx, err := run(ctx, fmt.Sprintf("helmfile destroy --file %s", helmfilePath))
		if helmHome != "" {
			_ = os.RemoveAll(helmHome)
		}
		return ctx, err
	}

	return setup, finish
}

// AddTestHelmRepos adds the helm repositories used directly by the in-test `helm install`
// calls (the "new <app> instance successfully deploys" assessments). These installs go
// through the e2e-framework helm.Manager, which uses the shared/global helm repository
// config, so the repos must exist there. Adding them once up front (serially) means the
// parallel suites only ever *read* the global config during their installs and never race
// on `helm repo add`. Helmfile-managed repos are isolated separately (see Helmfile).
func AddTestHelmRepos() (types.EnvFunc, types.EnvFunc) {
	repos := map[string]string{
		"goauthentik-charts": "https://charts.goauthentik.io",
		"teleport-charts":    "https://charts.releases.teleport.dev",
		"bjw-s-charts":       "https://bjw-s-labs.github.io/helm-charts",
	}

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		for name, url := range repos {
			if p := utils.RunCommand(fmt.Sprintf("helm repo add %s %s --force-update", name, url)); p.Err() != nil {
				return ctx, trace.Wrap(p.Err(), "failed to add helm repo %q: %s", name, p.Result())
			}
		}
		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return ctx, nil
	}

	return setup, finish
}

// ZfsPool manages the host-side ZFS pool that backs the openebs storage class: a sparse image
// file attached to a loopback device, plus the zfsutils packages installed on the kind nodes
// (openebs-zfs needs them). It is created before the dependent services (openebs consumes the
// pool) and, crucially, torn down AFTER the cluster is destroyed (see TestMain). Once the kind
// node containers are gone nothing holds the datasets, so `zpool destroy` is immediate and
// reliable; destroying it while openebs is still running is what made teardown slow and flaky.
func ZfsPool(clusterName string) (types.EnvFunc, types.EnvFunc) {
	imageFilePath := fmt.Sprintf("/tmp/%s-vdev.img", zpoolName)
	var loopDevice string // Set during setup, used during finish

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// Deploy ZFS zpool for openebs
		if p := utils.RunCommand(fmt.Sprintf("truncate -s 100G %q", imageFilePath)); p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to create ZFS pool file: %s", p.Result())
		}

		p := utils.RunCommand(fmt.Sprintf("losetup --show --sector-size 4096 --direct-io --find %q", imageFilePath))
		if p.Err() != nil {
			return ctx, trace.Wrap(p.Err(), "failed to attach file to loop device: %s", p.Result())
		}

		loopDevice = strings.TrimSpace(p.Result())
		if p := utils.RunCommand(fmt.Sprintf("zpool create -f -o ashift=12 %q %q", zpoolName, loopDevice)); p.Err() != nil {
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

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		errors := []error{} // Collect errors to return as an aggregate so that all cleanup steps are attempted

		// The cluster has already been destroyed by the time this runs, so the openebs CSI
		// driver and every dataset mount are gone and the pool can be destroyed right away. A
		// short retry just covers the occasional lingering handle.
		if err := wait.For(func(ctx context.Context) (bool, error) {
			return utils.RunCommand(fmt.Sprintf("zpool destroy -f %q", zpoolName)).Err() == nil, nil
		}, wait.WithContext(ctx), wait.WithInterval(2*time.Second), wait.WithTimeout(30*time.Second), wait.WithImmediate()); err != nil {
			errors = append(errors, trace.Wrap(err, "failed to remove ZFS pool"))
		}

		if loopDevice != "" {
			if p := utils.RunCommand(fmt.Sprintf("losetup -d %q", loopDevice)); p.Err() != nil {
				errors = append(errors, trace.Wrap(p.Err(), "failed to detach loop device: %s", p.Result()))
			}
		}

		if err := os.Remove(imageFilePath); err != nil && !os.IsNotExist(err) {
			errors = append(errors, trace.Wrap(err, "failed to remove ZFS pool file"))
		}

		return ctx, trace.NewAggregate(errors...)
	}

	return setup, finish
}

// DeploySnapshotVGS installs the external-snapshotter v8.6.0 snapshot stack (CRDs + controller) plus the
// host-path CSI driver, for VolumeGroupSnapshot coverage. It replaces the piraeus snapshot-controller chart:
// that chart only reaches external-snapshotter v8.5.0, whose VGS CRD serves v1beta1/v1beta2, while
// backup-tool uses the GA groupsnapshot.storage.k8s.io/v1 API first served by v8.6.0 (which has no helm chart
// yet). v8.6.0 still serves VolumeSnapshot v1, so the existing zfs-backed apps are unaffected. Must run
// before DeployDependentServices so the snapshot CRDs exist before openebs' csi-snapshotter sidecar starts.
//
// Everything is fetched from upstream at pinned versions (nothing vendored): the CRDs + controller via a
// kustomization referencing git refs, and the host-path manifests - which have no kustomization base - via
// raw-URL `kubectl apply -f` plus a patch (snapshotter-patch.yaml). There is no teardown; TestMain destroys
// the cluster wholesale.
func DeploySnapshotVGS() (types.EnvFunc, types.EnvFunc) {
	// Real path is kubernetes-1.30: deploy/kubernetes-1.31 (and -latest) are symlinks to it in this tag, and
	// raw.githubusercontent.com does not follow directory symlinks (a 1.31 URL 404s).
	const hostpathDir = "https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/v1.17.1/deploy/kubernetes-1.30/hostpath"

	// Sidecar RBAC, pinned to the sidecar image versions in the plugin manifest; defines the external-*-runner
	// ClusterRoles / *-cfg Roles the plugin's ServiceAccount bindings reference.
	sidecarRBAC := []string{
		"https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/v5.2.0/deploy/kubernetes/rbac.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-attacher/v4.8.0/deploy/kubernetes/rbac.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-resizer/v1.13.1/deploy/kubernetes/rbac.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-health-monitor/v0.14.0/deploy/kubernetes/external-health-monitor-controller/rbac.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.6.0/deploy/kubernetes/csi-snapshotter/rbac-csi-snapshotter.yaml",
	}

	commands := []string{
		// Snapshot CRDs + controller (external-snapshotter v8.6.0) + the controller's VGS feature gate.
		"kubectl apply -k ./config/setup/csi-snapshot-vgs",
		// Host-path sidecar RBAC, then the CSIDriver + node plugin. RBAC first so the plugin's sidecars have
		// it as soon as they start.
		"kubectl apply -f " + strings.Join(sidecarRBAC, " -f "),
		"kubectl apply -f " + hostpathDir + "/csi-hostpath-driverinfo.yaml -f " + hostpathDir + "/csi-hostpath-plugin.yaml",
		// Match the csi-snapshotter sidecar to the v8.6.0 controller and enable the VGS feature gate on it.
		"kubectl -n default patch statefulset csi-hostpathplugin --type=strategic --patch-file ./config/setup/csi-snapshot-vgs/snapshotter-patch.yaml",
	}

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		for _, command := range commands {
			if p := utils.RunCommand(command); p.Err() != nil {
				return ctx, trace.Wrap(p.Err(), "failed to deploy the snapshot stack / host-path driver via %q: %s", command, p.Result())
			}
		}
		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		return ctx, nil
	}

	return setup, finish
}

// DeployDependentServices deploys the cluster-side services shared by the DR tests (openebs,
// seaweedfs, CNPG, cert-manager, ...) via helmfile, and makes the openebs-backed storage class
// the default. It depends on ZfsPool having created the backing pool first. There is no
// teardown: these services live only inside the kind cluster, which TestMain destroys wholesale,
// so a graceful helm uninstall here would only add minutes for no benefit.
func DeployDependentServices(clusterName string) (types.EnvFunc, types.EnvFunc) {
	defaultStorageClass := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard"}}
	storageClassPatch := func(isDefault bool) k8s.Patch {
		return k8s.Patch{
			PatchType: k8stypes.StrategicMergePatchType,
			Data:      []byte(fmt.Sprintf(`{"metadata": {"annotations": {"storageclass.kubernetes.io/is-default-class": "%t"}}}`, isDefault)),
		}
	}
	helmSetup, _ := Helmfile("./config/setup/dependent-services/helmfile.yaml")

	setup := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {

		// Remove the "default" annotation from local-path CSI storage class
		if err := cfg.Client().Resources().Patch(ctx, defaultStorageClass, storageClassPatch(false)); err != nil {
			return ctx, trace.Wrap(err, "failed to remove default annotation from storage class %q", defaultStorageClass.Name)
		}

		// Deploy services
		if ctx, err := helmSetup(ctx, cfg); err != nil {
			return ctx, trace.Wrap(err, "failed to deploy dependent services")
		}

		// Set the S3 credentials
		s3Secret := &corev1.Secret{}
		if err := cfg.Client().Resources().Get(ctx, "seaweedfs-s3-secret", "default", s3Secret); err != nil {
			return ctx, trace.Wrap(err, "failed to get S3 secret")
		}
		s3AccessKeyId = string(s3Secret.Data["admin_access_key_id"])
		s3SecretAccessKey = string(s3Secret.Data["admin_secret_access_key"])

		return ctx, nil
	}

	finish := func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// No teardown — the cluster is destroyed wholesale by TestMain (see the doc comment).
		return ctx, nil
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
