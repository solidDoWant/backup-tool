package dr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

func DeployVaultWarden() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/vaultwarden/instance/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy vaultwarden instance")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "vaultwarden")
		require.NoError(t, err, "failed to wait for vaultwarden CNPG cluster to be ready")

		// Wait for the Vaultwarden service to become ready, by checking for at least one endpoint
		err = wait.For(func(ctx context.Context) (done bool, err error) {
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
		}, wait.WithContext(ctx), wait.WithImmediate(), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute))
		require.NoError(t, err, "failed to wait for vaultwarden service to be ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		// Remove the services
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err, "failed to remove Vaultwarden instance")

		return ctx
	}

	return setup, finish
}

func DeployVaultWardenRestore() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/vaultwarden/restore-backend/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy dependent services")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "vaultwarden-restore")
		require.NoError(t, err, "failed to wait for vaultwarden CNPG cluster to be ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err)
		return ctx
	}

	return setup, finish
}

func TestVaultWarden(t *testing.T) {
	backupReleaseName := "vw-test-successfull-backup"
	backupCronJobName := backupReleaseName + "-dr-job"

	restoreReleaseName := "vw-test-successfull-restore"
	restoreCronJobName := restoreReleaseName + "-dr-job"

	vaultwardenRestoreReleaseName := "vaultwarden-restore"

	namespace := "default"

	vaultWardenSetup, vaultWardenFinish := DeployVaultWarden()
	vaultWardenRestoreSetup, vaultWardenRestoreFinish := DeployVaultWardenRestore()

	f1 := features.New("Successfull vaultwarden backup and recovery").
		WithLabel("service", "vaultwarden").
		WithLabel("type", "dr").
		Setup(vaultWardenSetup).
		Setup(vaultWardenRestoreSetup).
		Setup(installBTHelmChart(backupReleaseName, namespace, "vaultwarden", "backup", "config/vaultwarden/tests/backup.values.yaml")).
		Assess("backup resources are deployed", verifyCronJobIsDeployed(backupCronJobName, namespace)).
		Assess("backup job succeeds", verifyJobSucceeds(backupCronJobName, namespace)).
		Assess("backup files are created", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			mountpoint, mountpointCleanup := getPVCLocalPath(ctx, t, c, namespace, "vw-e2e")
			defer mountpointCleanup()

			// Verify that the backup files are present
			assert.FileExists(t, filepath.Join(mountpoint, "dump.sql"))
			assert.DirExists(t, filepath.Join(mountpoint, "data-vol"))

			fileInfo, err := os.Lstat(filepath.Join(mountpoint, "data-vol", "rsa_key.pem"))
			assert.NoError(t, err)
			assert.True(t, fileInfo.Mode().IsRegular())
			assert.Equal(t, os.FileMode(0644), fileInfo.Mode().Perm())

			return ctx
		}).
		Teardown(uninstallBTHelmChart(backupReleaseName, namespace)).
		Setup(installBTHelmChart(restoreReleaseName, namespace, "vaultwarden", "restore", "config/vaultwarden/tests/restore.values.yaml")).
		Assess("restore resources are deployed", verifyCronJobIsDeployed(restoreCronJobName, namespace)).
		Assess("restore job succeeds", verifyJobSucceeds(restoreCronJobName, namespace)).
		Assess("new vaultwarden instance successfully deploys", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Install the helm chart
			hm := helm.New(cfg.KubeconfigFile())
			err := hm.RunRepo(helm.WithArgs("add", "bjw-s-charts", "https://bjw-s.github.io/helm-charts"))
			assert.NoError(t, err)

			valuesFilePath := "config/vaultwarden/tests/restore-instance.values.yaml"
			helpOpts := append(
				getCommonHelmOpts(vaultwardenRestoreReleaseName, namespace),
				helm.WithChart("bjw-s-charts/app-template"),
				helm.WithArgs("--values", valuesFilePath),
			)

			err = hm.RunInstall(helpOpts...)
			assert.NoError(t, err)
			defer uninstallBTHelmChart(vaultwardenRestoreReleaseName, namespace)(ctx, t, cfg)

			// Verify that the vaultwarden instance is running
			err = wait.For(func(ctx context.Context) (done bool, err error) {
				endpoints := &corev1.Endpoints{}
				if err := cfg.Client().Resources().Get(ctx, "vaultwarden-restore", "default", endpoints); err != nil {
					return false, trace.Wrap(err, "failed to get vaultwarden-restore endpoints")
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
			assert.NoError(t, err, "vaultwarden-restore endpoints are not ready")

			return ctx
		}).
		Teardown(uninstallBTHelmChart(restoreReleaseName, namespace)).
		Teardown(vaultWardenRestoreFinish).
		Teardown(vaultWardenFinish).
		Feature()

	testenv.Test(t, f1)
}
