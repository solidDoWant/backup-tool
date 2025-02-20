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

func TestTeleport(t *testing.T) {
	backupReleaseName := "tp-test-successfull-backup"
	backupCronJobName := backupReleaseName + "-dr-job"

	namespace := "default"

	teleportSetup, teleportFinish := DeployTeleport()

	f1 := features.New("Successfull teleport backup and recovery").
		WithLabel("service", "teleport").
		WithLabel("type", "dr").
		Setup(teleportSetup).
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
		// TODO run job fore restore to get CA endpoint, and verify that it's the same on the restored cluster
		Teardown(uninstallBTHelmChart(backupReleaseName, namespace)).
		Teardown(teleportFinish).
		Feature()

	testenv.Test(t, f1)
}
