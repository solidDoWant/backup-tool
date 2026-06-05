package dr

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// jobPodLogs returns the concatenated logs of every pod belonging to the named Job, for surfacing why an
// in-cluster verification job failed (the cluster is torn down before the test process can be inspected).
func jobPodLogs(ctx context.Context, c *envconf.Config, namespace, jobName string) string {
	clientset, err := kubernetes.NewForConfig(c.Client().RESTConfig())
	if err != nil {
		return "failed to build clientset for logs: " + err.Error()
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil {
		return "failed to list job pods: " + err.Error()
	}

	var sb strings.Builder
	for _, pod := range pods.Items {
		stream, err := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
		if err != nil {
			sb.WriteString(pod.Name + ": failed to get logs: " + err.Error() + "\n")
			continue
		}
		data, _ := io.ReadAll(stream)
		_ = stream.Close()
		sb.WriteString("--- " + pod.Name + " ---\n" + string(data) + "\n")
	}
	return sb.String()
}

// DeployDummy stands up the synthetic "dummy" app's source side. There is no real workload — the backend
// manifests (seeded CNPG clusters, S3 buckets, and data PVC) are the app.
func DeployDummy() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/dummy/instance/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy dummy instance")

		require.NoError(t, waitForCNPGClusterToBeReady(ctx, cfg, "dummy-db1"), "dummy-db1 not ready")
		require.NoError(t, waitForCNPGClusterToBeReady(ctx, cfg, "dummy-db2"), "dummy-db2 not ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err, "failed to remove dummy instance")
		return ctx
	}

	return setup, finish
}

// DeployDummyRestore stands up the restore targets: two empty CNPG clusters and an empty data PVC the
// generic restore repopulates in place.
func DeployDummyRestore() (features.Func, features.Func) {
	helmSetup, helmFinish := Helmfile("./config/dummy/restore-backend/helmfile.yaml")

	setup := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmSetup(ctx, cfg)
		require.NoError(t, err, "failed to deploy dummy restore backend")

		require.NoError(t, waitForCNPGClusterToBeReady(ctx, cfg, "dummy-restore-db1"), "dummy-restore-db1 not ready")
		require.NoError(t, waitForCNPGClusterToBeReady(ctx, cfg, "dummy-restore-db2"), "dummy-restore-db2 not ready")

		return ctx
	}

	finish := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ctx, err := helmFinish(ctx, cfg)
		assert.NoError(t, err)
		return ctx
	}

	return setup, finish
}

func assertSeedFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if !assert.NoError(t, err, "failed to read backed-up file %q", path) {
		return
	}
	assert.Equal(t, want, strings.TrimSpace(string(content)), "unexpected content in %q", path)
}

// verifyDummyRestoredData runs an in-cluster Job that reads the restored data back: it connects to both
// restore clusters over TLS (client-cert auth as the `app` user) and checks the seeded rows survived the
// dump/restore round-trip, and checks the restored files PVC holds the seeded file. The S3 sources are
// re-uploaded in place, so their success is covered by the restore job completing.
func verifyDummyRestoredData(namespace string) features.Func {
	return func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
		// chmod the client key to 0600 (libpq refuses a group/world-readable key), then assert the
		// seeded row in each restored cluster and the restored file. Each step echoes its result (and any
		// psql error, via 2>&1) so a failure is diagnosable from the pod logs; the final test decides exit.
		query := "select val from dr_test where id=1"
		conn := func(cluster, caDir string) string {
			return "host=" + cluster + "-rw user=app dbname=app sslmode=verify-full " +
				"sslrootcert=/certs/" + caDir + "/ca.crt sslcert=/certs/app/tls.crt sslkey=/tmp/k"
		}
		script := strings.Join([]string{
			"set -x",
			"cp /certs/app/tls.key /tmp/k && chmod 600 /tmp/k",
			`db1=$(psql "` + conn("dummy-restore-db1", "db1") + `" -tAc '` + query + `' 2>&1) || true`,
			`echo "db1=[$db1]"`,
			`db2=$(psql "` + conn("dummy-restore-db2", "db2") + `" -tAc '` + query + `' 2>&1) || true`,
			`echo "db2=[$db2]"`,
			`files=$(cat /data/hello.txt 2>&1) || true`,
			`echo "files=[$files]"`,
			`[ "$db1" = db1-seed ] && [ "$db2" = db2-seed ] && [ "$files" = files-seed ]`,
			"echo verify-ok",
		}, "\n")

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "dummy-restore-verify", Namespace: namespace},
			Spec: batchv1.JobSpec{
				// Retry a few times: the verify pod starts right after the restore job finishes, so a
				// fresh pod can briefly race the restored cluster's endpoints becoming reachable.
				BackoffLimit: new(int32(3)),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{{
							Name:    "verify",
							Image:   "postgres:16-alpine",
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{script},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "app", MountPath: "/certs/app", ReadOnly: true},
								{Name: "db1", MountPath: "/certs/db1", ReadOnly: true},
								{Name: "db2", MountPath: "/certs/db2", ReadOnly: true},
								{Name: "data", MountPath: "/data", ReadOnly: true},
							},
						}},
						Volumes: []corev1.Volume{
							{Name: "app", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "dummy-restore-app"}}},
							{Name: "db1", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "dummy-restore-db1-serving"}}},
							{Name: "db2", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "dummy-restore-db2-serving"}}},
							{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "dummy-restore-backend-data-vol"}}},
						},
					},
				},
			},
		}

		client := c.Client()
		require.NoError(t, client.Resources().Create(ctx, job), "failed to create verification job")

		var finalCondition batchv1.JobCondition
		err := wait.For(func(ctx context.Context) (bool, error) {
			if err := client.Resources().Get(ctx, job.Name, namespace, job); err != nil {
				return false, trace.Wrap(err, "failed to get verification job")
			}
			for _, condition := range job.Status.Conditions {
				if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) && condition.Status == corev1.ConditionTrue {
					finalCondition = condition
					return true, nil
				}
			}
			return false, nil
		}, wait.WithContext(ctx), wait.WithInterval(10*time.Second), wait.WithTimeout(10*time.Minute))
		assert.NoError(t, err, "verification job did not finish")
		if !assert.Equal(t, batchv1.JobComplete, finalCondition.Type, "restored data verification failed") {
			t.Logf("verification job pod logs:\n%s", jobPodLogs(ctx, c, namespace, job.Name))
		}

		return ctx
	}
}

func TestDummy(t *testing.T) {
	// Run concurrently with the other DR suites. See TestAuthentik for why this is safe (distinct
	// namespaced names, S3 buckets, and DR PVCs).
	t.Parallel()

	backupReleaseName := "dm-test-successfull-backup"
	backupCronJobName := backupReleaseName + "-dr-job"

	restoreReleaseName := "dm-test-successfull-restore"
	restoreCronJobName := restoreReleaseName + "-dr-job"

	namespace := "default"

	dummySetup, dummyFinish := DeployDummy()
	dummyRestoreSetup, dummyRestoreFinish := DeployDummyRestore()

	// The generic config's two S3 sources take inline credentials; inject the seaweedfs keys (resolved
	// during dependent-services setup) at install time. Helm merges these into each list entry's
	// credentials map (which already carries the endpoint/region/path-style from the values file).
	s3Creds := []string{
		"jobConfig.configFile.s3[0].credentials.accessKeyId=" + s3AccessKeyId,
		"jobConfig.configFile.s3[0].credentials.secretAccessKey=" + s3SecretAccessKey,
		"jobConfig.configFile.s3[1].credentials.accessKeyId=" + s3AccessKeyId,
		"jobConfig.configFile.s3[1].credentials.secretAccessKey=" + s3SecretAccessKey,
	}

	f1 := features.New("Successful dummy (generic) backup and recovery").
		WithLabel("service", "dummy").
		WithLabel("type", "dr").
		Setup(dummySetup).
		Setup(dummyRestoreSetup).
		Setup(installBTHelmChart(backupReleaseName, namespace, "generic", "backup", "config/dummy/tests/backup.values.yaml", s3Creds...)).
		Assess("backup resources are deployed", verifyCronJobIsDeployed(backupCronJobName, namespace)).
		Assess("backup job succeeds", verifyJobSucceeds(backupCronJobName, namespace)).
		Assess("backup captures every source", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			mountpoint, mountpointCleanup := getPVCLocalPath(ctx, t, c, namespace, "dm-e2e")
			defer mountpointCleanup()

			// Each path is the per-source slot-name derivation (postgres => <name>.sql, files/s3 => subdir);
			// content confirms the capture, not just the layout.
			assert.FileExists(t, filepath.Join(mountpoint, "db1.sql"))
			assert.FileExists(t, filepath.Join(mountpoint, "db2.sql"))
			assertSeedFile(t, filepath.Join(mountpoint, "files", "hello.txt"), "files-seed")
			assertSeedFile(t, filepath.Join(mountpoint, "bucket1", "seed.txt"), "bucket1-seed")
			assertSeedFile(t, filepath.Join(mountpoint, "bucket2", "seed.txt"), "bucket2-seed")

			return ctx
		}).
		Teardown(uninstallBTHelmChart(backupReleaseName, namespace)).
		Setup(installBTHelmChart(restoreReleaseName, namespace, "generic", "restore", "config/dummy/tests/restore.values.yaml", s3Creds...)).
		Assess("restore resources are deployed", verifyCronJobIsDeployed(restoreCronJobName, namespace)).
		Assess("restore job succeeds", verifyJobSucceeds(restoreCronJobName, namespace)).
		Assess("restored data round-trips", verifyDummyRestoredData(namespace)).
		Teardown(uninstallBTHelmChart(restoreReleaseName, namespace)).
		Teardown(dummyRestoreFinish).
		Teardown(dummyFinish).
		Feature()

	testenv.Test(t, f1)
}
