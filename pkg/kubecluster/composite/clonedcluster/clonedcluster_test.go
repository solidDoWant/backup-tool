package clonedcluster

import (
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	barmanapi "github.com/cloudnative-pg/barman-cloud/pkg/api"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	barmancloudv1 "github.com/cloudnative-pg/plugin-barman-cloud/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetClusterDomainNames(t *testing.T) {
	tests := []struct {
		desc        string
		clusterName string
		namespace   string
		expected    []string
	}{
		{
			desc:        "basic cluster name and namespace",
			clusterName: "test-cluster",
			namespace:   "test-ns",
			expected: []string{
				"test-cluster-r",
				"test-cluster-r.test-ns",
				"test-cluster-r.test-ns.svc",
				"test-cluster-ro",
				"test-cluster-ro.test-ns",
				"test-cluster-ro.test-ns.svc",
				"test-cluster-rw",
				"test-cluster-rw.test-ns",
				"test-cluster-rw.test-ns.svc",
			},
		},
		{
			desc:        "cluster name with numbers characters",
			clusterName: "my-special-cluster-123",
			namespace:   "prod-ns",
			expected: []string{
				"my-special-cluster-123-r",
				"my-special-cluster-123-r.prod-ns",
				"my-special-cluster-123-r.prod-ns.svc",
				"my-special-cluster-123-ro",
				"my-special-cluster-123-ro.prod-ns",
				"my-special-cluster-123-ro.prod-ns.svc",
				"my-special-cluster-123-rw",
				"my-special-cluster-123-rw.prod-ns",
				"my-special-cluster-123-rw.prod-ns.svc",
			},
		},
		{
			desc:        "short names",
			clusterName: "c1",
			namespace:   "ns",
			expected: []string{
				"c1-r",
				"c1-r.ns",
				"c1-r.ns.svc",
				"c1-ro",
				"c1-ro.ns",
				"c1-ro.ns.svc",
				"c1-rw",
				"c1-rw.ns",
				"c1-rw.ns.svc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			result := getClusterDomainNames(ctx, tt.clusterName, tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCredentials(t *testing.T) {
	tests := []struct {
		desc                string
		cluster             *apiv1.Cluster
		servingCertMountDir string
		clientCertMountDir  string
		expectedCredentials *cnpg.KubernetesSecretCredentials
	}{
		{
			desc: "basic credentials",
			cluster: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
			servingCertMountDir: "/certs/server",
			clientCertMountDir:  "/certs/client",
			expectedCredentials: &cnpg.KubernetesSecretCredentials{
				Host:                         "test-cluster-rw.test-ns.svc",
				ServingCertificateCAFilePath: "/certs/server/tls.crt",
				ClientCertificateFilePath:    "/certs/client/tls.crt",
				ClientPrivateKeyFilePath:     "/certs/client/tls.key",
			},
		},
		{
			desc: "different mount paths",
			cluster: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prod-db",
					Namespace: "prod",
				},
			},
			servingCertMountDir: "/var/run/secrets/server-cert",
			clientCertMountDir:  "/var/run/secrets/client-cert",
			expectedCredentials: &cnpg.KubernetesSecretCredentials{
				Host:                         "prod-db-rw.prod.svc",
				ServingCertificateCAFilePath: "/var/run/secrets/server-cert/tls.crt",
				ClientCertificateFilePath:    "/var/run/secrets/client-cert/tls.crt",
				ClientPrivateKeyFilePath:     "/var/run/secrets/client-cert/tls.key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{
				cluster: tt.cluster,
			}

			creds := cc.GetCredentials(tt.servingCertMountDir, tt.clientCertMountDir)
			assert.Equal(t, tt.expectedCredentials, creds)
		})
	}
}

func TestNewClonedCluster(t *testing.T) {
	t.Run("returns new ClonedCluster with client reference set", func(t *testing.T) {
		p := NewProvider(nil, nil, nil, nil, nil)
		cc := newClonedCluster(p)
		casted := cc.(*ClonedCluster)

		require.NotNil(t, cc)
		assert.Equal(t, p, casted.p)
	})
}

func TestCloneClusterOptions(t *testing.T) {
	th.OptStructTest[CloneClusterOptions](t)
}

func barmanCloudPlugin(params map[string]string, isWALArchiver bool) apiv1.PluginConfiguration {
	return apiv1.PluginConfiguration{
		Name:          cnpg.BarmanCloudPluginName,
		IsWALArchiver: new(isWALArchiver),
		Parameters:    params,
	}
}

func snapshotBackup(elements ...apiv1.BackupSnapshotElementStatus) *apiv1.Backup {
	return &apiv1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "test-ns"},
		Status: apiv1.BackupStatus{
			BackupSnapshotStatus: apiv1.BackupSnapshotStatus{Elements: elements},
		},
	}
}

func TestConfigureWALRecovery(t *testing.T) {
	namespace := "test-ns"
	clusterName := "source-cluster"
	dataElement := apiv1.BackupSnapshotElementStatus{Name: "snap-data", Type: string(utils.PVCRolePgData)}
	walElement := apiv1.BackupSnapshotElementStatus{Name: "snap-wal", Type: string(utils.PVCRolePgWal)}

	pluginWALSource := func(serverName string) apiv1.ExternalCluster {
		return apiv1.ExternalCluster{
			Name: serverName,
			PluginConfiguration: &apiv1.PluginConfiguration{
				Name: cnpg.BarmanCloudPluginName,
				Parameters: map[string]string{
					"barmanObjectName": "store",
					"serverName":       serverName,
				},
			},
		}
	}

	tests := []struct {
		desc            string
		plugins         []apiv1.PluginConfiguration
		backup          *apiv1.Backup
		objectStore     *barmancloudv1.ObjectStore
		objectStoreErr  bool
		expectGetStore  bool
		expectErr       bool
		expectedOptions cnpg.CreateClusterOptions
	}{
		{
			desc:            "in-tree barman (no plugins) recovers from the backup object",
			backup:          snapshotBackup(dataElement),
			expectedOptions: cnpg.CreateClusterOptions{BackupName: "test-backup"},
		},
		{
			desc:            "non-barman plugin is ignored and treated as in-tree",
			plugins:         []apiv1.PluginConfiguration{{Name: "some-other.plugin", IsWALArchiver: new(true)}},
			backup:          snapshotBackup(dataElement),
			expectedOptions: cnpg.CreateClusterOptions{BackupName: "test-backup"},
		},
		{
			desc:    "plugin with explicit serverName parameter",
			plugins: []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"barmanObjectName": "store", "serverName": "custom-server"}, true)},
			backup:  snapshotBackup(dataElement),
			expectedOptions: cnpg.CreateClusterOptions{
				VolumeSnapshotRecovery: &cnpg.VolumeSnapshotRecovery{
					DataSnapshotName: "snap-data",
					WALSource:        pluginWALSource("custom-server"),
				},
			},
		},
		{
			desc:    "plugin with separate WAL volume snapshot",
			plugins: []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"barmanObjectName": "store", "serverName": "custom-server"}, true)},
			backup:  snapshotBackup(dataElement, walElement),
			expectedOptions: cnpg.CreateClusterOptions{
				VolumeSnapshotRecovery: &cnpg.VolumeSnapshotRecovery{
					DataSnapshotName: "snap-data",
					WALSnapshotName:  "snap-wal",
					WALSource:        pluginWALSource("custom-server"),
				},
			},
		},
		{
			desc:           "plugin without serverName falls back to the object store's server name",
			plugins:        []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"barmanObjectName": "store"}, true)},
			backup:         snapshotBackup(dataElement),
			objectStore:    &barmancloudv1.ObjectStore{Spec: barmancloudv1.ObjectStoreSpec{Configuration: barmanapi.BarmanObjectStoreConfiguration{ServerName: "store-server"}}},
			expectGetStore: true,
			expectedOptions: cnpg.CreateClusterOptions{
				VolumeSnapshotRecovery: &cnpg.VolumeSnapshotRecovery{
					DataSnapshotName: "snap-data",
					WALSource: apiv1.ExternalCluster{
						Name: "store-server",
						PluginConfiguration: &apiv1.PluginConfiguration{
							Name:       cnpg.BarmanCloudPluginName,
							Parameters: map[string]string{"barmanObjectName": "store", "serverName": "store-server"},
						},
					},
				},
			},
		},
		{
			desc:           "plugin without serverName and empty object store server name defaults to cluster name",
			plugins:        []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"barmanObjectName": "store"}, true)},
			backup:         snapshotBackup(dataElement),
			objectStore:    &barmancloudv1.ObjectStore{Spec: barmancloudv1.ObjectStoreSpec{Configuration: barmanapi.BarmanObjectStoreConfiguration{}}},
			expectGetStore: true,
			expectedOptions: cnpg.CreateClusterOptions{
				VolumeSnapshotRecovery: &cnpg.VolumeSnapshotRecovery{
					DataSnapshotName: "snap-data",
					WALSource:        pluginWALSource(clusterName),
				},
			},
		},
		{
			desc:      "plugin missing barmanObjectName parameter is an error",
			plugins:   []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"serverName": "custom-server"}, true)},
			backup:    snapshotBackup(dataElement),
			expectErr: true,
		},
		{
			desc:      "plugin backup without a PG_DATA snapshot is an error",
			plugins:   []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"barmanObjectName": "store", "serverName": "custom-server"}, true)},
			backup:    snapshotBackup(walElement),
			expectErr: true,
		},
		{
			desc:           "object store lookup failure is an error",
			plugins:        []apiv1.PluginConfiguration{barmanCloudPlugin(map[string]string{"barmanObjectName": "store"}, true)},
			backup:         snapshotBackup(dataElement),
			objectStoreErr: true,
			expectGetStore: true,
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			p := newMockProvider(t)

			sourceCluster := &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: namespace},
				Spec:       apiv1.ClusterSpec{Plugins: tt.plugins},
			}

			if tt.expectGetStore {
				p.barmanCloudClient.EXPECT().GetObjectStore(mock.Anything, namespace, "store").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*barmancloudv1.ObjectStore, error) {
						return th.ErrOr1Val(tt.objectStore, tt.objectStoreErr)
					})
			}

			clusterOpts := cnpg.CreateClusterOptions{}
			isPluginRecovery, err := p.configureWALRecovery(ctx, namespace, sourceCluster, tt.backup, &clusterOpts)

			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedOptions, clusterOpts)
			// The plugin path is exactly the one that recovers from volume snapshots.
			assert.Equal(t, tt.expectedOptions.VolumeSnapshotRecovery != nil, isPluginRecovery)
		})
	}
}

func TestClonedClusterWhenFailToParseExistingClusterStorageSize(t *testing.T) {
	p := newMockProvider(t)
	ctx := th.NewTestContext()

	p.cnpgClient.EXPECT().GetCluster(mock.Anything, "test-ns", "existing-cluster").
		RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*apiv1.Cluster, error) {
			assert.True(t, calledCtx.IsChildOf(ctx))
			return &apiv1.Cluster{
				Spec: apiv1.ClusterSpec{
					StorageConfiguration: apiv1.StorageConfiguration{
						Size: "not-a-size",
					},
				},
			}, nil
		})
	p.clonedCluster.EXPECT().Delete(mock.Anything).Return(nil)

	// Call CloneClusterFromBackup directly so the test focuses on the storage-size parse failure
	// without needing to drive the base backup creation that CloneCluster performs first.
	clonedCluster, err := p.CloneClusterFromBackup(ctx, "test-ns", "existing-cluster", "new-cluster", "issuer-1", "issuer-2", &apiv1.Backup{}, CloneClusterOptions{})
	require.Error(t, err)
	require.Nil(t, clonedCluster)
}

func TestClonedClusterDelete(t *testing.T) {
	cluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
	}

	servingCert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "serving-cert", Namespace: "test-ns"},
	}

	clientCACert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: "client-ca-cert", Namespace: "test-ns"},
	}

	clientCAIssuer := &certmanagerv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{Name: "client-ca-issuer", Namespace: "test-ns"},
	}

	allResourcesCluster := ClonedCluster{
		cluster:             cluster,
		servingCertificate:  servingCert,
		clientCACertificate: clientCACert,
		clientCAIssuer:      clientCAIssuer,
	}

	tests := []struct {
		desc                                        string
		cc                                          ClonedCluster
		includePostgresUserCert                     bool // Workaround as these are mocked types that require test-specific t
		includeStreamingReplicaUserCert             bool
		simulateClusterDeleteError                  bool
		simulateStreamingReplicaUserCertDeleteError bool
		simulatePostgresUserCertDeleteError         bool
		simulateClientCAIssuerDeleteError           bool
		simulateClientCACertDeleteError             bool
		simulateServingCertDeleteError              bool
		expectedErrorsInMessage                     int
	}{
		{
			desc:                            "delete all resources",
			cc:                              allResourcesCluster,
			includePostgresUserCert:         true,
			includeStreamingReplicaUserCert: true,
		},
		{
			desc: "delete with no cluster",
			cc: ClonedCluster{
				servingCertificate:  servingCert,
				clientCACertificate: clientCACert,
			},
		},
		{
			desc: "delete with just serving cert",
			cc: ClonedCluster{
				servingCertificate: servingCert,
			},
		},
		{
			desc: "delete with just client cert",
			cc: ClonedCluster{
				servingCertificate:  servingCert,
				clientCACertificate: clientCACert,
			},
		},
		{
			desc: "delete with just cluster",
			cc: ClonedCluster{
				cluster: cluster,
			},
		},
		{
			desc: "delete empty cloned cluster",
		},
		{
			desc:                            "all deletions fail",
			cc:                              allResourcesCluster,
			includePostgresUserCert:         true,
			includeStreamingReplicaUserCert: true,
			simulateClusterDeleteError:      true,
			simulateStreamingReplicaUserCertDeleteError: true,
			simulatePostgresUserCertDeleteError:         true,
			simulateClientCAIssuerDeleteError:           true,
			simulateClientCACertDeleteError:             true,
			simulateServingCertDeleteError:              true,
			expectedErrorsInMessage:                     6,
		},
		{
			desc:                       "cluster deletion fails",
			cc:                         allResourcesCluster,
			simulateClusterDeleteError: true,
			expectedErrorsInMessage:    1,
		},
		{
			desc:                            "certificate deletions fail",
			cc:                              allResourcesCluster,
			simulateClientCACertDeleteError: true,
			simulateServingCertDeleteError:  true,
			expectedErrorsInMessage:         2,
		},
		{
			desc:                           "serving certificate deletion fails",
			cc:                             allResourcesCluster,
			simulateServingCertDeleteError: true,
			expectedErrorsInMessage:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			p := newMockProvider(t)
			tt.cc.p = p

			if tt.cc.cluster != nil {
				p.cnpgClient.EXPECT().DeleteCluster(mock.Anything, tt.cc.cluster.Namespace, tt.cc.cluster.Name).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateClusterDeleteError)
					})
			}

			if tt.includeStreamingReplicaUserCert {
				streamingReplicaUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
				tt.cc.streamingReplicaUserCertificate = streamingReplicaUserCert
				streamingReplicaUserCert.EXPECT().Delete(mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateStreamingReplicaUserCertDeleteError)
					})
			}

			if tt.includePostgresUserCert {
				postgresUserCert := clusterusercert.NewMockClusterUserCertInterface(t)
				tt.cc.postgresUserCertificate = postgresUserCert
				postgresUserCert.EXPECT().Delete(mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulatePostgresUserCertDeleteError)
					})
			}

			if tt.cc.clientCAIssuer != nil {
				p.cmClient.EXPECT().DeleteIssuer(mock.Anything, tt.cc.clientCAIssuer.Namespace, tt.cc.clientCAIssuer.Name).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateClientCAIssuerDeleteError)
					})
			}

			if tt.cc.clientCACertificate != nil {
				p.cmClient.EXPECT().DeleteCertificate(mock.Anything, tt.cc.clientCACertificate.Namespace, tt.cc.clientCACertificate.Name).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateClientCACertDeleteError)
					})
			}

			if tt.cc.servingCertificate != nil {
				p.cmClient.EXPECT().DeleteCertificate(mock.Anything, tt.cc.servingCertificate.Namespace, tt.cc.servingCertificate.Name).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateServingCertDeleteError)
					})
			}

			err := tt.cc.Delete(ctx)
			if tt.expectedErrorsInMessage == 0 {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			if tErr, ok := err.(trace.Error); ok {
				if oErrs, ok := tErr.OrigError().(trace.Aggregate); ok {
					assert.Equal(t, tt.expectedErrorsInMessage, len(oErrs.Errors()))
				}
			} else {
				require.Fail(t, "error is not a trace.Error")
			}
		})
	}
}

func TestClonedClusterSetServingCert(t *testing.T) {
	tests := []struct {
		desc string
		cert *certmanagerv1.Certificate
	}{
		{
			desc: "set non-nil certificate",
			cert: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setServingCert(tt.cert)
			assert.Equal(t, tt.cert, cc.servingCertificate)
		})
	}
}

func TestClonedClusterGetServingCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   ClonedCluster
		want *certmanagerv1.Certificate
	}{
		{
			desc: "get existing serving certificate",
			cc: ClonedCluster{
				servingCertificate: &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cert",
						Namespace: "test-ns",
					},
				},
			},
			want: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil serving certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetServingCert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetCluster(t *testing.T) {
	tests := []struct {
		desc    string
		cluster *apiv1.Cluster
	}{
		{
			desc: "set non-nil cluster",
			cluster: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setCluster(tt.cluster)
			assert.Equal(t, tt.cluster, cc.cluster)
		})
	}
}

func TestClonedClusterGetCluster(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want *apiv1.Cluster
	}{
		{
			desc: "get existing cluster",
			cc: &ClonedCluster{
				cluster: &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-ns",
					},
				},
			},
			want: &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil cluster",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetCluster()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetClientCACert(t *testing.T) {
	tests := []struct {
		desc string
		cert *certmanagerv1.Certificate
	}{
		{
			desc: "set non-nil certificate",
			cert: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setClientCACert(tt.cert)
			assert.Equal(t, tt.cert, cc.clientCACertificate)
		})
	}
}

func TestClonedClusterGetClientCACert(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want *certmanagerv1.Certificate
	}{
		{
			desc: "get existing client CA certificate",
			cc: &ClonedCluster{
				clientCACertificate: &certmanagerv1.Certificate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cert",
						Namespace: "test-ns",
					},
				},
			},
			want: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil client CA certificate",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetClientCACert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetClientCAIssuer(t *testing.T) {
	tests := []struct {
		desc   string
		issuer *certmanagerv1.Issuer
	}{
		{
			desc: "set non-nil issuer",
			issuer: &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-issuer",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "set nil issuer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setClientCAIssuer(tt.issuer)
			assert.Equal(t, tt.issuer, cc.clientCAIssuer)
		})
	}
}

func TestClonedClusterGetClientCAIssuer(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want *certmanagerv1.Issuer
	}{
		{
			desc: "get existing client CA issuer",
			cc: &ClonedCluster{
				clientCAIssuer: &certmanagerv1.Issuer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-issuer",
						Namespace: "test-ns",
					},
				},
			},
			want: &certmanagerv1.Issuer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-issuer",
					Namespace: "test-ns",
				},
			},
		},
		{
			desc: "get nil client CA issuer",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetClientCAIssuer()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetPostgresUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cert clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "set non-nil certificate",
			cert: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setPostgresUserCert(tt.cert)
			assert.Equal(t, tt.cert, cc.postgresUserCertificate)
		})
	}
}

func TestClonedClusterGetPostgresUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "get existing postgres user certificate",
			cc: &ClonedCluster{
				postgresUserCertificate: &clusterusercert.ClusterUserCert{},
			},
			want: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "get nil postgres user certificate",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetPostgresUserCert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClonedClusterSetStreamingReplicaUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cert clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "set non-nil certificate",
			cert: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "set nil certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cc := &ClonedCluster{}
			cc.setStreamingReplicaUserCert(tt.cert)
			assert.Equal(t, tt.cert, cc.streamingReplicaUserCertificate)
		})
	}
}

func TestClonedClusterGetStreamingReplicaUserCert(t *testing.T) {
	tests := []struct {
		desc string
		cc   *ClonedCluster
		want clusterusercert.ClusterUserCertInterface
	}{
		{
			desc: "get existing streaming replica user certificate",
			cc: &ClonedCluster{
				streamingReplicaUserCertificate: &clusterusercert.ClusterUserCert{},
			},
			want: &clusterusercert.ClusterUserCert{},
		},
		{
			desc: "get nil streaming replica user certificate",
			cc:   &ClonedCluster{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := tt.cc.GetStreamingReplicaUserCert()
			assert.Equal(t, tt.want, got)
		})
	}
}

func readyCluster(name string) *apiv1.Cluster {
	return &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Status: apiv1.ClusterStatus{
			Conditions: []metav1.Condition{{
				Type:   string(apiv1.ConditionClusterReady),
				Status: metav1.ConditionTrue,
			}},
		},
	}
}

func TestWaitForCloneRecovery(t *testing.T) {
	namespace := "test-ns"
	clusterName := "new-cluster"
	targetOpts := CloneClusterOptions{RecoveryTargetTime: time.Now().Format(time.RFC3339)}

	t.Run("no target time waits for ready", func(t *testing.T) {
		p := newMockProvider(t)
		ctx := th.NewTestContext()
		want := readyCluster(clusterName)
		p.cnpgClient.EXPECT().WaitForReadyCluster(mock.Anything, namespace, clusterName, mock.Anything).Return(want, nil)

		got, err := p.waitForCloneRecovery(ctx, namespace, clusterName, false, CloneClusterOptions{})
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("target time, recovery job completes then cluster becomes ready", func(t *testing.T) {
		p := newMockProvider(t)
		ctx := th.NewTestContext()
		want := readyCluster(clusterName)
		// The recovery Job is selected (by an empty name) via the cluster + jobRole labels.
		p.coreClient.EXPECT().WaitForJobCompletion(mock.Anything, namespace, "", core.WaitForJobCompletionOpts{LabelSelector: "cnpg.io/cluster=new-cluster,cnpg.io/jobRole"}).Return(&batchv1.Job{}, nil)
		p.cnpgClient.EXPECT().WaitForReadyCluster(mock.Anything, namespace, clusterName, mock.Anything).Return(want, nil)

		got, err := p.waitForCloneRecovery(ctx, namespace, clusterName, true, targetOpts)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("target time, recovery job fails returns ErrRecoveryTargetNotReached", func(t *testing.T) {
		p := newMockProvider(t)
		ctx := th.NewTestContext()
		p.coreClient.EXPECT().WaitForJobCompletion(mock.Anything, namespace, mock.Anything, mock.Anything).Return(nil, core.ErrJobFailed)

		_, err := p.waitForCloneRecovery(ctx, namespace, clusterName, true, targetOpts)
		assert.ErrorIs(t, err, ErrRecoveryTargetNotReached)
	})

	t.Run("target time, recovery job wait error propagates", func(t *testing.T) {
		p := newMockProvider(t)
		ctx := th.NewTestContext()
		p.coreClient.EXPECT().WaitForJobCompletion(mock.Anything, namespace, mock.Anything, mock.Anything).Return(nil, assert.AnError)

		_, err := p.waitForCloneRecovery(ctx, namespace, clusterName, true, targetOpts)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRecoveryTargetNotReached)
	})
}
