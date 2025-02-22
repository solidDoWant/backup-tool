package dr

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
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

func getBTInstallArgs(releaseName, namespace, service, action, valuesPath string, extraInstallValues []string) []helm.Option {
	tagSeparatorIndex := strings.LastIndex(imageName, ":")
	repository, tag := imageName[:tagSeparatorIndex], imageName[tagSeparatorIndex+1:]

	installValues := []string{
		fmt.Sprintf("resources.controllers.backup-tool.containers.backup-tool.image.repository=%s", repository),
		fmt.Sprintf("resources.controllers.backup-tool.containers.backup-tool.image.tag=%s", tag),
		fmt.Sprintf("jobConfig.configFile.namespace=%s", namespace),
		fmt.Sprintf("jobConfig.drType=%s", service),
		fmt.Sprintf("jobConfig.drAction=%s", action),
		"jobConfig.cronjob.schedule=@yearly", // Make the job as unlikely as possible to trigger automatically during the test
	}
	installValues = append(installValues, extraInstallValues...)

	installArgs := []string{"--values", valuesPath}
	for _, value := range installValues {
		installArgs = append(installArgs, "--set", value)
	}

	return append(getCommonHelmOpts(releaseName, namespace), helm.WithChart(chartPath), helm.WithArgs(installArgs...))
}

func installBTHelmChart(releaseName, namespace, service, action, valuesPath string, extraInstallValues ...string) func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		helmOpts := getBTInstallArgs(releaseName, namespace, service, action, valuesPath, extraInstallValues)
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

func getPVCLocalPath(ctx context.Context, t *testing.T, c *envconf.Config, namespace, pvcName string) (string, func()) {
	client := c.Client()

	// Get the name of the persistent volume
	pvc := &corev1.PersistentVolumeClaim{}
	err := client.Resources().Get(ctx, pvcName, namespace, pvc)
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
	cleanup := func() {
		p = utils.RunCommand(fmt.Sprintf("zfs destroy -f %q", cloneDatasetName))
		assert.NoError(t, p.Err())
	}

	return mountpoint, cleanup
}

func waitForCNPGClusterToBeReady(ctx context.Context, cfg *envconf.Config, clusterName string) error {
	// Wait for the CNPG cluster to become ready
	cnpgClient, err := versioned.NewForConfig(cfg.Client().RESTConfig())
	if err != nil {
		return trace.Wrap(err, "failed to create CNPG client")
	}

	err = wait.For(func(ctx context.Context) (done bool, err error) {
		cnpgCluster, err := cnpgClient.PostgresqlV1().Clusters("default").Get(ctx, clusterName, metav1.GetOptions{})
		if err != nil {
			return false, trace.Wrap(err, "failed to get CNPG cluster")
		}

		for _, condition := range cnpgCluster.Status.Conditions {
			if condition.Type != string(apiv1.ConditionClusterReady) {
				continue
			}

			return condition.Status == metav1.ConditionTrue, nil
		}

		return false, nil
	}, wait.WithContext(ctx), wait.WithInterval(10*time.Second), wait.WithTimeout(2*time.Minute))
	return trace.Wrap(err, "failed to wait for CNPG cluster to be ready")
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
