package cnpg

import (
	"context"
	"sync"
	"testing"
	"time"

	"dario.cat/mergo"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/machinery/pkg/api"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestCreateBackup(t *testing.T) {
	namespace := "test-ns"
	backupName := "test-backup"
	clusterName := "test-cluster"

	standardBackupOptions := &apiv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: apiv1.BackupSpec{
			Cluster: api.LocalObjectReference{
				Name: clusterName,
			},
			Online: ptr.To(true),
			OnlineConfiguration: &apiv1.OnlineConfiguration{
				WaitForArchive: ptr.To(true),
			},
		},
	}

	tests := []struct {
		name                  string
		opts                  CreateBackupOptions
		expected              *apiv1.Backup
		simulateClientFailure bool
	}{
		{
			name: "basic backup creation",
			expected: &apiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name: backupName,
				},
				Spec: apiv1.BackupSpec{
					Method: apiv1.BackupMethodVolumeSnapshot,
					Target: apiv1.BackupTargetStandby,
				},
			},
		},
		{
			name: "backup with all options",
			opts: CreateBackupOptions{
				GenerateName: true,
				Method:       ptr.To(apiv1.BackupMethodBarmanObjectStore),
				Target:       ptr.To(apiv1.BackupTargetPrimary),
			},
			expected: &apiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: backupName,
				},
				Spec: apiv1.BackupSpec{
					Method: apiv1.BackupMethodBarmanObjectStore,
					Target: apiv1.BackupTargetPrimary,
				},
			},
		},
		{
			name:                  "backup creation errors",
			simulateClientFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fakeClientset, _ := createTestClient()

			if tt.simulateClientFailure {
				fakeClientset.PrependReactor("create", "backups", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			ctx := th.NewTestContext()
			createdBackup, err := client.CreateBackup(ctx, namespace, backupName, clusterName, tt.opts)
			if tt.simulateClientFailure {
				require.Error(t, err)
				require.Nil(t, createdBackup)
				return
			}

			expectedBackup := standardBackupOptions.DeepCopy()
			require.NoError(t, mergo.MergeWithOverwrite(expectedBackup, tt.expected))

			require.NoError(t, err)
			require.Equal(t, expectedBackup, createdBackup)
		})
	}
}

func TestWaitForReadyBackup(t *testing.T) {
	backupName := "test-backup"
	backupNamespace := "test-ns"

	noStatusBackup := &apiv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: backupNamespace,
			Name:      backupName,
		},
	}

	completedBackup := noStatusBackup.DeepCopy()
	completedBackup.Status.Phase = apiv1.BackupPhaseCompleted

	failedBackup := noStatusBackup.DeepCopy()
	failedBackup.Status.Phase = apiv1.BackupPhaseFailed

	walFailingBackup := noStatusBackup.DeepCopy()
	walFailingBackup.Status.Phase = apiv1.BackupPhaseWalArchivingFailing

	pendingBackup := noStatusBackup.DeepCopy()
	pendingBackup.Status.Phase = apiv1.BackupPhasePending

	tests := []struct {
		desc                string
		initialBackup       *apiv1.Backup
		shouldError         bool
		afterStartedWaiting func(*testing.T, context.Context, versioned.Interface)
	}{
		{
			desc:          "backup starts completed",
			initialBackup: completedBackup,
		},
		{
			desc:          "backup failed",
			initialBackup: failedBackup,
			shouldError:   true,
		},
		{
			desc:          "backup WAL archiving failing",
			initialBackup: walFailingBackup,
			shouldError:   true,
		},
		{
			desc:          "backup has no status",
			initialBackup: noStatusBackup,
			shouldError:   true,
		},
		{
			desc:        "backup does not exist",
			shouldError: true,
		},
		{
			desc:          "backup becomes completed",
			initialBackup: pendingBackup,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.PostgresqlV1().Backups(backupNamespace).Update(ctx, completedBackup, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:          "backup becomes failed",
			initialBackup: pendingBackup,
			shouldError:   true,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.PostgresqlV1().Backups(backupNamespace).Update(ctx, failedBackup, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, cnpgFakeClient, _ := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialBackup != nil {
				_, err := cnpgFakeClient.PostgresqlV1().Backups(backupNamespace).Create(ctx, tt.initialBackup, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var backup *apiv1.Backup
			wg.Add(1)
			go func() {
				backup, waitErr = client.WaitForReadyBackup(ctx, backupNamespace, backupName, WaitForReadyBackupOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, cnpgFakeClient)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, backup)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, backup)
		})
	}
}

func TestDeleteBackup(t *testing.T) {
	namespace := "test-ns"
	backupName := "test-backup"

	tests := []struct {
		desc              string
		shouldSetupBackup bool
		wantErr           bool
	}{
		{
			desc:              "delete existing backup",
			shouldSetupBackup: true,
		},
		{
			desc:    "delete non-existent backup",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, _, _ := createTestClient()
			ctx := th.NewTestContext()

			var existingbackup *apiv1.Backup
			if tt.shouldSetupBackup {
				existingbackup = &apiv1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupName,
						Namespace: namespace,
					},
				}
				_, err := client.cnpgClient.PostgresqlV1().Backups(namespace).Create(ctx, existingbackup, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteBackup(ctx, namespace, backupName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the backup was deleted
			backupList, err := client.cnpgClient.PostgresqlV1().Backups(namespace).List(ctx, metav1.SingleObject(existingbackup.ObjectMeta))
			assert.NoError(t, err)
			assert.Empty(t, backupList.Items)
		})
	}
}

func TestCreateCluster(t *testing.T) {
	namespace := "test-ns"
	clusterName := "test-cluster"
	volumeSize := resource.MustParse("1Gi")
	servingCertName := "serving-cert"
	clientCAName := "client-ca"
	replicationUserCertName := "replication-user-cert"

	standardClusterOptions := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: apiv1.ClusterSpec{
			Instances: 1,
			Bootstrap: &apiv1.BootstrapConfiguration{},
			StorageConfiguration: apiv1.StorageConfiguration{
				Size: volumeSize.String(),
			},
			Certificates: &apiv1.CertificatesConfiguration{
				ServerTLSSecret:      servingCertName,
				ServerCASecret:       servingCertName,
				ClientCASecret:       clientCAName,
				ReplicationTLSSecret: replicationUserCertName,
			},
			PostgresConfiguration: apiv1.PostgresConfiguration{
				PgHBA: []string{"hostssl all all all cert"},
			},
		},
	}

	tests := []struct {
		desc                        string
		enablePrometheusMetrics     bool
		shouldFailToQueryForMetrics bool
		opts                        CreateClusterOptions
		expected                    *apiv1.Cluster
		simulateClientFailure       bool
	}{
		{
			desc: "basic cluster creation",
			opts: CreateClusterOptions{},
			expected: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
			},
		},
		{
			desc: "basic cluster with generated name",
			opts: CreateClusterOptions{
				GenerateName: true,
			},
			expected: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: clusterName,
				},
			},
		},
		{
			desc: "basic cluster with resource requirements",
			opts: CreateClusterOptions{
				ResourceRequirements: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    *resource.NewScaledQuantity(100, resource.Milli),
						corev1.ResourceMemory: *resource.NewScaledQuantity(128, resource.Mega),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewScaledQuantity(128, resource.Mega),
					},
				},
			},
			expected: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
				Spec: apiv1.ClusterSpec{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    *resource.NewScaledQuantity(100, resource.Milli),
							corev1.ResourceMemory: *resource.NewScaledQuantity(128, resource.Mega),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: *resource.NewScaledQuantity(128, resource.Mega),
						},
					},
				},
			},
		},
		{
			desc:                    "basic cluster with monitoring supported",
			enablePrometheusMetrics: true,
			opts:                    CreateClusterOptions{},
			expected: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
				Spec: apiv1.ClusterSpec{
					Monitoring: &apiv1.MonitoringConfiguration{
						EnablePodMonitor: true,
					},
				},
			},
		},
		{
			desc:                        "basic cluster with monitoring supported but querying for support fails",
			enablePrometheusMetrics:     true,
			shouldFailToQueryForMetrics: true,
			opts:                        CreateClusterOptions{},
		},
		{
			desc: "cluster with backup recovery",
			opts: CreateClusterOptions{
				BackupName:   "test-backup",
				DatabaseName: "testdb",
				OwnerName:    "testowner",
			},
			expected: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
				Spec: apiv1.ClusterSpec{
					Bootstrap: &apiv1.BootstrapConfiguration{
						Recovery: &apiv1.BootstrapRecovery{
							Backup: &apiv1.BackupSource{
								LocalObjectReference: apiv1.LocalObjectReference{
									Name: "test-backup",
								},
							},
							Database: "testdb",
							Owner:    "testowner",
						},
					},
				},
			},
		},
		{
			desc: "cluster with initdb",
			opts: CreateClusterOptions{
				DatabaseName: "testdb",
				OwnerName:    "testowner",
				StorageClass: "test-class",
			},
			expected: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
				Spec: apiv1.ClusterSpec{
					Bootstrap: &apiv1.BootstrapConfiguration{
						InitDB: &apiv1.BootstrapInitDB{
							Database: "testdb",
							Owner:    "testowner",
						},
					},
					StorageConfiguration: apiv1.StorageConfiguration{
						StorageClass: ptr.To("test-class"),
					},
				},
			},
		},
		{
			desc:                  "cluster creation errors",
			simulateClientFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			client, cnpgFakeClient, apiExtensionsFakeClient := createTestClient()

			if tt.simulateClientFailure {
				cnpgFakeClient.PrependReactor("create", "clusters", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			if tt.enablePrometheusMetrics {
				apiExtensionsFakeClient.PrependReactor("get", "customresourcedefinitions", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
					// Return just enough to satisfy the API call tests until the bugged API extensions package is fixed
					if tt.shouldFailToQueryForMetrics {
						return true, nil, assert.AnError
					}
					return true, nil, nil
				})
				// THe API extensions client is all kinds of screwed up, see https://github.com/kubernetes/kubernetes/issues/126850
				// _, err := apiExtensionsFakeClient.ApiextensionsV1().CustomResourceDefinitions().Apply(ctx, v1.CustomResourceDefinition("podmonitors.monitoring.coreos.com"), metav1.ApplyOptions{})
				// require.NoError(t, err)
			}

			createdCluster, err := client.CreateCluster(ctx, namespace, clusterName, volumeSize, servingCertName, clientCAName, replicationUserCertName, tt.opts)
			if tt.simulateClientFailure || tt.shouldFailToQueryForMetrics {
				require.Error(t, err)
				require.Nil(t, createdCluster)
				return
			}

			expectedCluster := standardClusterOptions.DeepCopy()
			require.NoError(t, mergo.MergeWithOverwrite(expectedCluster, tt.expected))

			require.NoError(t, err)
			require.Equal(t, expectedCluster, createdCluster)
		})
	}
}

func TestWaitForReadyCluster(t *testing.T) {
	clusterName := "test-cluster"
	clusterNamespace := "test-ns"

	noStatusCluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterNamespace,
			Name:      clusterName,
		},
	}

	notReadyCluster := noStatusCluster.DeepCopy()
	notReadyCondition := metav1.Condition{Type: string(apiv1.ConditionClusterReady), Status: metav1.ConditionStatus(apiv1.ConditionFalse)}
	notReadyCluster.Status.Conditions = append(notReadyCluster.Status.Conditions, notReadyCondition)

	readyCluster := notReadyCluster.DeepCopy()
	readyCondition := notReadyCondition.DeepCopy()
	readyCondition.Status = metav1.ConditionTrue
	readyCluster.Status.Conditions[0] = *readyCondition

	multipleConditionsCluster := readyCluster.DeepCopy()
	issuingCondition := metav1.Condition{Type: string(apiv1.ConditionContinuousArchiving), Status: metav1.ConditionStatus(apiv1.ConditionTrue)}
	multipleConditionsCluster.Status.Conditions = []metav1.Condition{issuingCondition, *readyCondition}

	tests := []struct {
		desc                string
		initialCluster      *apiv1.Cluster
		shouldError         bool
		afterStartedWaiting func(*testing.T, context.Context, versioned.Interface)
	}{
		{
			desc:           "cluster starts ready",
			initialCluster: readyCluster,
		},
		{
			desc:           "cluster not ready",
			initialCluster: notReadyCluster,
			shouldError:    true,
		},
		{
			desc:           "cluster has no status",
			initialCluster: noStatusCluster,
			shouldError:    true,
		},
		{
			desc:        "cluster does not exist",
			shouldError: true,
		},
		{
			desc:           "cluster becomes ready",
			initialCluster: notReadyCluster,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.PostgresqlV1().Clusters(clusterNamespace).Update(ctx, readyCluster, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:           "multiple conditions",
			initialCluster: notReadyCluster,
			afterStartedWaiting: func(t *testing.T, ctx context.Context, client versioned.Interface) {
				_, err := client.PostgresqlV1().Clusters(clusterNamespace).Update(ctx, multipleConditionsCluster, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, cnpgFakeClient, _ := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialCluster != nil {
				_, err := cnpgFakeClient.PostgresqlV1().Clusters(clusterNamespace).Create(ctx, tt.initialCluster, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var cluster *apiv1.Cluster
			wg.Add(1)
			go func() {
				cluster, waitErr = client.WaitForReadyCluster(ctx, clusterNamespace, clusterName, WaitForReadyClusterOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, cnpgFakeClient)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, cluster)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, cluster)
		})
	}
}

func TestGetCluster(t *testing.T) {
	namespace := "test-ns"
	clusterName := "test-cluster"

	existingCluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}

	tests := []struct {
		desc               string
		shouldSetupCluster bool
		simulateError      bool
		wantErr            bool
	}{
		{
			desc:               "get existing cluster",
			shouldSetupCluster: true,
		},
		{
			desc:    "get non-existent cluster",
			wantErr: true,
		},
		{
			desc:               "client error",
			shouldSetupCluster: true,
			simulateError:      true,
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, cnpgFakeClient, _ := createTestClient()
			ctx := th.NewTestContext()

			if tt.shouldSetupCluster {
				_, err := cnpgFakeClient.PostgresqlV1().Clusters(namespace).Create(ctx, existingCluster, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tt.simulateError {
				cnpgFakeClient.PrependReactor("get", "clusters", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			cluster, err := client.GetCluster(ctx, namespace, clusterName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, cluster)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, existingCluster, cluster)
		})
	}
}

func TestDeleteCluster(t *testing.T) {
	namespace := "test-ns"
	clusterName := "test-cluster"

	tests := []struct {
		desc               string
		shouldSetupCluster bool
		wantErr            bool
	}{
		{
			desc:               "delete existing cluster",
			shouldSetupCluster: true,
		},
		{
			desc:    "delete non-existent cluster",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, cnpgFakeClient, _ := createTestClient()
			ctx := th.NewTestContext()

			var existingCluster *apiv1.Cluster
			if tt.shouldSetupCluster {
				existingCluster = &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
				}
				_, err := cnpgFakeClient.PostgresqlV1().Clusters(namespace).Create(ctx, existingCluster, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteCluster(ctx, namespace, clusterName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the cluster was deleted
			clusterList, err := cnpgFakeClient.PostgresqlV1().Clusters(namespace).List(ctx, metav1.SingleObject(existingCluster.ObjectMeta))
			assert.NoError(t, err)
			assert.Empty(t, clusterList.Items)
		})
	}
}

func TestGetUsername(t *testing.T) {
	tests := []struct {
		desc     string
		user     string
		expected string
	}{
		{
			desc:     "empty username returns postgres",
			user:     "",
			expected: "postgres",
		},
		{
			desc:     "custom username is returned",
			user:     "testuser",
			expected: "testuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			credentials := &KubernetesSecretCredentials{User: tt.user}
			assert.Equal(t, tt.expected, credentials.GetUsername())
		})
	}
}

func TestGetHost(t *testing.T) {
	tests := []struct {
		desc     string
		host     string
		expected string
	}{
		{
			desc:     "empty host returns empty string",
			host:     "",
			expected: "",
		},
		{
			desc:     "host is returned",
			host:     "test-host",
			expected: "test-host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			credentials := &KubernetesSecretCredentials{Host: tt.host}
			assert.Equal(t, tt.expected, credentials.GetHost())
		})
	}
}

func TestGetPort(t *testing.T) {
	tests := []struct {
		desc     string
		port     string
		expected string
	}{
		{
			desc:     "empty port returns default postgres port",
			port:     "",
			expected: postgres.PostgresDefaultPort,
		},
		{
			desc:     "custom port is returned",
			port:     "5433",
			expected: "5433",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			credentials := &KubernetesSecretCredentials{Port: tt.port}
			assert.Equal(t, tt.expected, credentials.GetPort())
		})
	}
}

func TestGetVariables(t *testing.T) {
	tests := []struct {
		desc     string
		creds    *KubernetesSecretCredentials
		expected postgres.CredentialVariables
	}{
		{
			desc: "all fields populated",
			creds: &KubernetesSecretCredentials{
				Host:                         "test-host",
				Port:                         "5433",
				User:                         "test-user",
				ServingCertificateCAFilePath: "/certs/tls.crt",
				ClientCertificateFilePath:    "/certs/client.crt",
				ClientPrivateKeyFilePath:     "/certs/client.key",
			},
			expected: postgres.CredentialVariables{
				postgres.HostVarName:        "test-host",
				postgres.PortVarName:        "5433",
				postgres.UserVarName:        "test-user",
				postgres.RequireAuthVarName: "none",
				postgres.SSLModeVarName:     "verify-full",
				postgres.SSLCertVarName:     "/certs/client.crt",
				postgres.SSLKeyVarName:      "/certs/client.key",
				postgres.SSLRootCertVarName: "/certs/tls.crt",
			},
		},
		{
			desc:  "minimal values",
			creds: &KubernetesSecretCredentials{},
			expected: postgres.CredentialVariables{
				postgres.HostVarName:        "",
				postgres.PortVarName:        postgres.PostgresDefaultPort,
				postgres.UserVarName:        "postgres",
				postgres.RequireAuthVarName: "none",
				postgres.SSLModeVarName:     "verify-full",
				postgres.SSLCertVarName:     "",
				postgres.SSLKeyVarName:      "",
				postgres.SSLRootCertVarName: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			variables := tt.creds.GetVariables()
			assert.Equal(t, tt.expected, variables)
		})
	}
}
