package vaultwarden

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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/pkg/utils"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

func TestBackup(t *testing.T) {
	releaseName := "test-successfull-backup"
	namespace := "default"
	cronjobName := releaseName + "-dr-job"

	// Build Helm install/uninstall options
	helmOpts := []helm.Option{
		helm.WithName(releaseName),
		helm.WithNamespace(namespace),
		helm.WithWait(),
	}

	tagSeparatorIndex := strings.LastIndex(imageName, ":")
	repository, tag := imageName[:tagSeparatorIndex], imageName[tagSeparatorIndex+1:]

	installValues := []string{
		fmt.Sprintf("resources.controllers.backup-tool.containers.backup-tool.image.repository=%s", repository),
		fmt.Sprintf("resources.controllers.backup-tool.containers.backup-tool.image.tag=%s", tag),
		fmt.Sprintf("jobConfig.configFile.namespace=%s", namespace),
		"jobConfig.drType=vaultwarden",
		"jobConfig.drAction=backup",
		"jobConfig.schedule=@yearly", // Make the job as unlikely as possible to trigger automatically during the test
	}

	installArgs := []string{"--values", "config/tests/backup.values.yaml"}
	for _, value := range installValues {
		installArgs = append(installArgs, "--set", value)
	}

	f1 := features.New("Successfull vaultwarden backup").
		WithLabel("service", "vaultwarden").
		WithLabel("type", "backup").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			helmInstallOpts := append(helmOpts, helm.WithChart(chartPath), helm.WithArgs(installArgs...))
			err := helm.New(c.KubeconfigFile()).RunInstall(helmInstallOpts...)
			assert.NoError(t, err)
			return ctx
		}).
		Assess("resources are deployed", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			client := c.Client()

			// Check if the backup job is created
			cronjob := &batchv1.CronJob{}
			err := client.Resources().Get(ctx, cronjobName, namespace, cronjob)
			assert.NoError(t, err)
			assert.NotNil(t, cronjob)

			return ctx
		}).
		Assess("job succeeds", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			client := c.Client()
			jobName := cronjobName + "-manual"

			// Get the cronjob
			cronjob := &batchv1.CronJob{}
			err := client.Resources().Get(ctx, cronjobName, namespace, cronjob)
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
		}).
		Assess("backup files are created", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			client := c.Client()

			// Get the name of the persistent volume
			pvc := &corev1.PersistentVolumeClaim{}
			err := client.Resources().Get(ctx, "e2e", namespace, pvc)
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
		Teardown(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			err := helm.New(c.KubeconfigFile()).RunUninstall(helmOpts...)
			assert.NoError(t, err)
			return ctx
		}).
		Feature()

	testenv.Test(t, f1)
}
