package dr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/pkg/utils"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

func getCommonHelmOpts(releaseName, namespace string) []helm.Option {
	return []helm.Option{
		helm.WithName(releaseName),
		helm.WithNamespace(namespace),
		helm.WithWait(),
	}
}

func getBTInstallArgs(releaseName, namespace, action, valuesPath string) []helm.Option {
	tagSeparatorIndex := strings.LastIndex(imageName, ":")
	repository, tag := imageName[:tagSeparatorIndex], imageName[tagSeparatorIndex+1:]

	installValues := []string{
		fmt.Sprintf("resources.controllers.backup-tool.containers.backup-tool.image.repository=%s", repository),
		fmt.Sprintf("resources.controllers.backup-tool.containers.backup-tool.image.tag=%s", tag),
		fmt.Sprintf("jobConfig.configFile.namespace=%s", namespace),
		"jobConfig.drType=vaultwarden",
		fmt.Sprintf("jobConfig.drAction=%s", action),
		"jobConfig.cronjob.schedule=@yearly", // Make the job as unlikely as possible to trigger automatically during the test
	}

	installArgs := []string{"--values", valuesPath}
	for _, value := range installValues {
		installArgs = append(installArgs, "--set", value)
	}

	return append(getCommonHelmOpts(releaseName, namespace), helm.WithChart(chartPath), helm.WithArgs(installArgs...))
}

func installBTHelmChart(releaseName, namespace, action, valuesPath string) func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		helmOpts := getBTInstallArgs(releaseName, namespace, action, valuesPath)
		err := helm.New(c.KubeconfigFile()).RunInstall(helmOpts...)
		assert.NoError(t, err)
		return ctx
	}
}

func uninstallBTHelmChart(releaseName, namespace string) func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		helmOpts := getCommonHelmOpts(releaseName, namespace)
		err := helm.New(c.KubeconfigFile()).RunUninstall(helmOpts...)
		assert.NoError(t, err)
		return ctx
	}
}

func verifyCronJobIsDeployed(cronjobName, namespace string) func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		client := c.Client()

		cronjob := &batchv1.CronJob{}
		err := client.Resources().Get(ctx, cronjobName, namespace, cronjob)

		assert.NoError(t, err)
		assert.NotNil(t, cronjob)

		return ctx
	}
}

func verifyJobSucceeds(cronJobName, namespace string) func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		client := c.Client()
		jobName := cronJobName + "-manual"

		// Get the cronjob
		cronjob := &batchv1.CronJob{}
		err := client.Resources().Get(ctx, cronJobName, namespace, cronjob)
		assert.NoError(t, err)

		// Use it to build a normal job using what `kubectl create job --from=cronjob/...` would do
		annotations := make(map[string]string)
		annotations["cronjob.kubernetes.io/instantiate"] = "manual"
		for k, v := range cronjob.Spec.JobTemplate.Annotations {
			annotations[k] = v
		}

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:        jobName,
				Namespace:   namespace,
				Annotations: annotations,
				Labels:      cronjob.Spec.JobTemplate.Labels,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: batchv1.SchemeGroupVersion.String(),
						Kind:       "CronJob",
						Name:       cronjob.GetName(),
						UID:        cronjob.GetUID(),
						Controller: ptr.To(true),
					},
				},
			},
			Spec: cronjob.Spec.JobTemplate.Spec,
		}

		// Start the job
		err = client.Resources().Create(ctx, job)
		assert.NoError(t, err)

		// Wait for the job to finish, whether it succeeds or fails
		var finalCondition batchv1.JobCondition
		err = wait.For(func(ctx context.Context) (bool, error) {
			if err := client.Resources().Get(ctx, jobName, namespace, job); err != nil {
				return false, trace.Wrap(err, "failed to get job")
			}

			for _, condition := range job.Status.Conditions {
				finalCondition = condition
				if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) &&
					condition.Status == "True" {
					return true, nil
				}
			}

			return false, nil
		}, wait.WithContext(ctx), wait.WithInterval(10*time.Second), wait.WithTimeout(20*time.Minute))
		assert.NoError(t, err)

		// Verify that the job reports success
		assert.Equal(t, batchv1.JobComplete, finalCondition.Type)
		assert.Equal(t, corev1.ConditionTrue, finalCondition.Status)

		return ctx
	}
}

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

	restoreReleaseName := "vw-successfull-restore"
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
		Setup(installBTHelmChart(backupReleaseName, namespace, "backup", "config/vaultwarden/tests/backup.values.yaml")).
		Assess("backup resources are deployed", verifyCronJobIsDeployed(backupCronJobName, namespace)).
		Assess("backup job succeeds", verifyJobSucceeds(backupCronJobName, namespace)).
		Assess("backup files are created", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			client := c.Client()

			// Get the name of the persistent volume
			pvc := &corev1.PersistentVolumeClaim{}
			err := client.Resources().Get(ctx, "vw-e2e", namespace, pvc)
			assert.NoError(t, err)

			pvName := pvc.Spec.VolumeName
			datasetName := fmt.Sprintf("%s/%s", zpoolName, pvName)

			// Get the snapshot name
			p := utils.RunCommand(fmt.Sprintf("zfs list -t snapshot -H -o name -d 1 %q", datasetName))
			assert.NoError(t, p.Err())
			snapshotName := strings.TrimSpace(p.Result())

			// Create a new dataset from the snapshot
			mountpoint := t.TempDir()
			cloneDatasetName := fmt.Sprintf("%s/%s", zpoolName, "backup-test")
			p = utils.RunCommand(fmt.Sprintf("zfs clone -o %q %q %q", "mountpoint="+mountpoint, snapshotName, cloneDatasetName))
			assert.NoError(t, p.Err())
			defer func() {
				p = utils.RunCommand(fmt.Sprintf("zfs destroy -f %q", cloneDatasetName))
				assert.NoError(t, p.Err())
			}()

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
		Setup(installBTHelmChart(restoreReleaseName, namespace, "restore", "config/vaultwarden/tests/restore.values.yaml")).
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
