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

func DeployAuthentik() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/authentik/instance/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy authentik instance")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "authentik-postgres")
		require.NoError(t, err, "failed to wait for authentik CNPG cluster to be ready")

		// Wait for the Authentik service to become ready, by checking for at least one endpoint
		err = wait.For(func(ctx context.Context) (done bool, err error) {
			endpoints := &corev1.Endpoints{}
			if err := cfg.Client().Resources().Get(ctx, "authentik-server", "default", endpoints); err != nil {
				return false, trace.Wrap(err, "failed to get authentik service endpoints")
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
		require.NoError(t, err, "failed to wait for authentik service to be ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		// Remove the services
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err, "failed to remove authentik instance")

		return ctx
	}

	return setup, finish
}

func DeployAuthentikRestore() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/authentik/restore-backend/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy authentik instance")

		err = waitForCNPGClusterToBeReady(ctx, cfg, "ar-postgres")
		require.NoError(t, err, "failed to wait for ar-postgres CNPG cluster to be ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err)
		return ctx
	}

	return setup, finish
}

func TestAuthentik(t *testing.T) {
	backupReleaseName := "at-test-successfull-backup"
	backupCronJobName := backupReleaseName + "-dr-job"

	restoreReleasename := "at-test-successfull-restore"
	restoreCronJobName := restoreReleasename + "-dr-job"

	authentikRestoreReleaseName := "authentik-restore"

	namespace := "default"

	authentikSetup, authentikFinish := DeployAuthentik()
	authentikRestoreSetup, authentikRestoreFinish := DeployAuthentikRestore()

	extraInstallValues := []string{
		"jobConfig.configFile.s3.credentials.accessKeyId=" + s3AccessKeyId,
		"jobConfig.configFile.s3.credentials.secretAccessKey=" + s3SecretAccessKey,
	}

	f1 := features.New("Successfull authentik backup and recovery").
		WithLabel("service", "authentik").
		WithLabel("type", "dr").
		Setup(authentikSetup).
		Setup(authentikRestoreSetup).
		Setup(installBTHelmChart(backupReleaseName, namespace, "authentik", "backup", "config/authentik/tests/backup.values.yaml", extraInstallValues...)).
		Assess("backup resources are deployed", verifyCronJobIsDeployed(backupCronJobName, namespace)).
		Assess("backup job succeeds", verifyJobSucceeds(backupCronJobName, namespace)).
		Assess("backup files are created", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			mountpoint, mountpointCleanup := getPVCLocalPath(ctx, t, c, namespace, "at-e2e")
			defer mountpointCleanup()

			// Verify that the backup files are present
			assert.FileExists(t, filepath.Join(mountpoint, "dump.sql"))

			return ctx
		}).
		Teardown(uninstallBTHelmChart(backupReleaseName, namespace)).
		Setup(installBTHelmChart(restoreReleasename, namespace, "authentik", "restore", "config/authentik/tests/restore.values.yaml", extraInstallValues...)).
		Assess("restore resources are deployed", verifyCronJobIsDeployed(restoreCronJobName, namespace)).
		Assess("restore job succeeds", verifyJobSucceeds(restoreCronJobName, namespace)).
		Assess("new authentik instance successfully deploys", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Install the helm chart
			hm := helm.New(cfg.KubeconfigFile())
			err := hm.RunRepo(helm.WithArgs("add", "goauthentik-charts", "https://charts.goauthentik.io"))
			assert.NoError(t, err)

			valuesFilePath := "config/authentik/tests/restore-instance.values.yaml"
			helpOpts := append(
				getCommonHelmOpts(authentikRestoreReleaseName, namespace),
				helm.WithChart("goauthentik-charts/authentik"),
				helm.WithArgs("--values", valuesFilePath),
			)

			err = hm.RunInstall(helpOpts...)
			assert.NoError(t, err)
			defer uninstallBTHelmChart(authentikRestoreReleaseName, namespace)(ctx, t, cfg)

			// Verify that the authentik instance is running
			err = wait.For(func(ctx context.Context) (done bool, err error) {
				endpoints := &corev1.Endpoints{}
				if err := cfg.Client().Resources().Get(ctx, "ar-server", "default", endpoints); err != nil {
					return false, trace.Wrap(err, "failed to get ar-server endpoints")
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
			assert.NoError(t, err, "ar-server endpoints are not ready")
			return ctx
		}).
		Teardown(uninstallBTHelmChart(restoreReleasename, namespace)).
		Teardown(authentikRestoreFinish).
		Teardown(authentikFinish).
		Feature()

	testenv.Test(t, f1)
}
