package dr

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		// Wait for the Teleport service to become ready, by checking for at least one endpoint
		err = waitForServiceEndpoints(ctx, cfg, "teleport")
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
	// Run concurrently with the other DR suites. See TestAuthentik for why this is safe.
	t.Parallel()

	backupReleaseName := "tp-test-successfull-backup"
	backupCronJobName := backupReleaseName + "-dr-job"

	restoreReleasename := "tp-test-successfull-restore"
	restoreCronJobName := restoreReleasename + "-dr-job"

	teleportRestoreReleaseName := "teleport-restore"

	namespace := "default"

	teleportSetup, teleportFinish := DeployTeleport()
	teleportRestoreSetup, teleportRestoreFinish := DeployTeleportRestore()

	extraInstallValues := []string{
		"jobConfig.configFile.auditSessionLogs.credentials.accessKeyId=" + s3AccessKeyId,
		"jobConfig.configFile.auditSessionLogs.credentials.secretAccessKey=" + s3SecretAccessKey,
	}

	f1 := features.New("Successfull teleport backup and recovery").
		WithLabel("service", "teleport").
		WithLabel("type", "dr").
		Setup(teleportSetup).
		Setup(teleportRestoreSetup).
		Setup(installBTHelmChart(backupReleaseName, namespace, "teleport", "backup", "config/teleport/tests/backup.values.yaml", extraInstallValues...)).
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
		Setup(installBTHelmChart(restoreReleasename, namespace, "teleport", "restore", "config/teleport/tests/restore.values.yaml", extraInstallValues...)).
		Assess("restore resources are deployed", verifyCronJobIsDeployed(restoreCronJobName, namespace)).
		Assess("restore job succeeds", verifyJobSucceeds(restoreCronJobName, namespace)).
		Assess("new teleport instance successfully deploys", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Install the helm chart. The teleport-charts repo is added once during suite
			// setup (see AddTestHelmRepos) so parallel suites don't race on `helm repo add`.
			hm := helm.New(cfg.KubeconfigFile())

			valuesFilePath := "config/teleport/tests/restore-instance.values.yaml"
			helpOpts := append(
				getCommonHelmOpts(teleportRestoreReleaseName, namespace),
				helm.WithChart("teleport-charts/teleport-cluster"),
				helm.WithVersion("18.1.4"),
				helm.WithArgs("--values", valuesFilePath),
			)

			err := hm.RunInstall(helpOpts...)
			assert.NoError(t, err)
			defer uninstallBTHelmChart(teleportRestoreReleaseName, namespace)(ctx, t, cfg)

			// Verify that the teleport instance is running
			err = waitForServiceEndpoints(ctx, cfg, "teleport-restore")
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
