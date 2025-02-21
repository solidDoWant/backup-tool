package dr

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

func DeployTeleport() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/teleport/instance/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy teleport instance")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "teleport-core")
		require.NoError(t, err, "failed to wait for teleport-core CNPG cluster to be ready")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "teleport-audit")
		require.NoError(t, err, "failed to wait for teleport-audit CNPG cluster to be ready")

		// Wait for the Vaultwarden service to become ready, by checking for at least one endpoint
		err = wait.For(func(ctx context.Context) (done bool, err error) {
			endpoints := &corev1.Endpoints{}
			if err := cfg.Client().Resources().Get(ctx, "teleport", "default", endpoints); err != nil {
				return false, trace.Wrap(err, "failed to get teleport service endpoints")
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
		}, wait.WithContext(ctx), wait.WithImmediate(), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute))
		require.NoError(t, err, "failed to wait for teleport service to be ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		// Remove the services
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err, "failed to remove teleport instance")

		return ctx
	}

	return setup, finish
}

func DeployTeleportRestore() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/teleport/restore-backend/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy teleport instance")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "tpr-core")
		require.NoError(t, err, "failed to wait for tpr-restore-core CNPG cluster to be ready")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "tpr-audit")
		require.NoError(t, err, "failed to wait for tpr-audit CNPG cluster to be ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err)
		return ctx
	}

	return setup, finish
}

func TestTeleport(t *testing.T) {
	backupReleaseName := "tp-test-successfull-backup"
	backupCronJobName := backupReleaseName + "-dr-job"

	restoreReleasename := "tp-test-successfull-restore"
	restoreCronJobName := restoreReleasename + "-dr-job"

	teleportRestoreReleaseName := "teleport-restore"

	namespace := "default"

	teleportSetup, teleportFinish := DeployTeleport()
	teleportRestoreSetup, teleportRestoreFinish := DeployTeleportRestore()

	f1 := features.New("Successfull teleport backup and recovery").
		WithLabel("service", "teleport").
		WithLabel("type", "dr").
		Setup(teleportSetup).
		Setup(teleportRestoreSetup).
		Setup(installBTHelmChart(backupReleaseName, namespace, "teleport", "backup", "config/teleport/tests/backup.values.yaml")).
		Assess("backup resources are deployed", verifyCronJobIsDeployed(backupCronJobName, namespace)).
		Assess("backup job succeeds", verifyJobSucceeds(backupCronJobName, namespace)).
		Assess("backup files are created", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			mountpoint, mountpointCleanup := getPVCLocalPath(ctx, t, c, namespace, "tp-e2e")
			defer mountpointCleanup()

			// Verify that the backup files are present
			assert.FileExists(t, filepath.Join(mountpoint, "backup-core.sql"))
			assert.FileExists(t, filepath.Join(mountpoint, "backup-audit.sql"))

			return ctx
		}).
		Teardown(uninstallBTHelmChart(backupReleaseName, namespace)).
		Setup(installBTHelmChart(restoreReleasename, namespace, "teleport", "restore", "config/teleport/tests/restore.values.yaml")).
		Assess("restore resources are deployed", verifyCronJobIsDeployed(restoreCronJobName, namespace)).
		Assess("restore job succeeds", verifyJobSucceeds(restoreCronJobName, namespace)).
		Assess("new teleport instance successfully deploys", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Install the helm chart
			hm := helm.New(cfg.KubeconfigFile())
			err := hm.RunRepo(helm.WithArgs("add", "teleport-charts", "https://charts.releases.teleport.dev"))
			assert.NoError(t, err)

			valuesFilePath := "config/teleport/tests/restore-instance.values.yaml"
			helpOpts := append(
				getCommonHelmOpts(teleportRestoreReleaseName, namespace),
				helm.WithChart("teleport-charts/teleport-cluster"),
				helm.WithArgs("--values", valuesFilePath),
			)

			err = hm.RunInstall(helpOpts...)
			assert.NoError(t, err)
			defer uninstallBTHelmChart(teleportRestoreReleaseName, namespace)(ctx, t, cfg)

			// Verify that the vaultwarden instance is running
			err = wait.For(func(ctx context.Context) (done bool, err error) {
				endpoints := &corev1.Endpoints{}
				if err := cfg.Client().Resources().Get(ctx, "teleport-restore", "default", endpoints); err != nil {
					return false, trace.Wrap(err, "failed to get teleport-restore endpoints")
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
			}, wait.WithContext(ctx), wait.WithImmediate(), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute))
			assert.NoError(t, err, "teleport-restore endpoints are not ready")

			// TODO run job to get backup instance CA endpoint, and verify that it's the same on the restored instance

			return ctx
		}).
		Teardown(uninstallBTHelmChart(restoreReleasename, namespace)).
		Teardown(teleportRestoreFinish).
		Teardown(teleportFinish).
		Feature()

	testenv.Test(t, f1)
}
