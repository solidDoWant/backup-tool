package disasterrecovery

import (
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	cnpgbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/backup"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	filesbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/backup"
	filesgroupbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/groupbackup"
	filesgrouprestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/grouprestore"
	filesrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/restore"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// validBackupConfig returns a minimal, valid Vaultwarden-shaped backup config.
func validBackupConfig() GenericBackupConfig {
	return GenericBackupConfig{
		Namespace:    "vaultwarden",
		BackupName:   "vaultwarden",
		BackupVolume: GenericBackupVolume{Size: resource.MustParse("10Gi")},
		Postgres: []GenericPostgresBackupSource{{
			Name:    "main",
			Cluster: "vw-db",
			ClusterCloning: clonedcluster.CloneClusterOptions{
				Certificates: clonedcluster.CloneClusterOptionsCertificates{
					ServingCert:  clonedcluster.CloneClusterOptionsCertificate{CRPOpts: clusterusercert.NewClusterUserCertOptsCRP{WaitForCRPTimeout: helpers.ShortWaitTime}},
					ClientCACert: clonedcluster.CloneClusterOptionsCertificate{CRPOpts: clusterusercert.NewClusterUserCertOptsCRP{WaitForCRPTimeout: helpers.ShortWaitTime}},
				},
			},
		}},
		Files: []GenericFilesBackupSource{{GenericFilesSource: GenericFilesSource{Name: "data", PVC: "vw-data"}, SnapshotClass: "ceph-block-snap"}},
		FileGroups: []GenericFileGroupBackupSource{{
			GenericFileGroupSource: GenericFileGroupSource{Name: "shards", Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "vw-shard"}}},
			SnapshotClass:          "ceph-block-group-snap",
		}},
		S3: []GenericS3Source{{
			Name:        "media",
			Path:        "s3://media-bucket/vw",
			Credentials: s3.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "secret"},
		}},
	}
}

func validRestoreConfig() GenericRestoreConfig {
	return GenericRestoreConfig{
		Namespace:  "vaultwarden",
		BackupName: "vaultwarden",
		Postgres: []GenericPostgresRestoreSource{{
			Name:           "main",
			Cluster:        "vw-db",
			ClientCAIssuer: cmmeta.IssuerReference{Name: "cnpg-client-ca"},
			ServingCert:    "vw-db-serving",
		}},
		Files: []GenericFilesSource{{Name: "data", PVC: "vw-data"}},
		FileGroups: []GenericFileGroupSource{{
			Name:     "shards",
			Selector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "vw-shard"}},
		}},
		S3: []GenericS3Source{{
			Name:        "media",
			Path:        "s3://media-bucket/vw",
			Credentials: s3.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "secret"},
		}},
	}
}

func TestGenericBackupConfigValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, validBackupConfig().Validate())
	})

	t.Run("files-only may omit size", func(t *testing.T) {
		c := GenericBackupConfig{
			Namespace:  "ns",
			BackupName: "b",
			Files:      []GenericFilesBackupSource{{GenericFilesSource: GenericFilesSource{Name: "data", PVC: "vw-data"}}},
		}
		require.NoError(t, c.Validate())
	})

	t.Run("s3 credentials optional (env fallback)", func(t *testing.T) {
		c := validBackupConfig()
		c.S3[0].Credentials = s3.Credentials{}
		require.NoError(t, c.Validate())
	})

	tests := []struct {
		name      string
		mutate    func(c *GenericBackupConfig)
		errSubstr string
	}{
		{
			name:      "no sources",
			mutate:    func(c *GenericBackupConfig) { c.Postgres = nil; c.Files = nil; c.FileGroups = nil; c.S3 = nil },
			errSubstr: "at least one source",
		},
		{
			name:      "size required with postgres source",
			mutate:    func(c *GenericBackupConfig) { c.BackupVolume.Size = resource.Quantity{} },
			errSubstr: "backupVolume.size is required",
		},
		{
			name: "size required with s3 source only",
			mutate: func(c *GenericBackupConfig) {
				c.Postgres = nil
				c.Files = nil
				c.FileGroups = nil
				c.BackupVolume.Size = resource.Quantity{}
			},
			errSubstr: "backupVolume.size is required",
		},
		{
			name: "size required with fileGroup source only",
			mutate: func(c *GenericBackupConfig) {
				c.Postgres = nil
				c.Files = nil
				c.S3 = nil
				c.BackupVolume.Size = resource.Quantity{}
			},
			errSubstr: "backupVolume.size is required",
		},
		{
			name:      "duplicate fileGroup slot name",
			mutate:    func(c *GenericBackupConfig) { c.FileGroups = append(c.FileGroups, c.FileGroups[0]) },
			errSubstr: "duplicate fileGroup slot name",
		},
		{
			name:      "empty fileGroup selector",
			mutate:    func(c *GenericBackupConfig) { c.FileGroups[0].Selector = metav1.LabelSelector{} },
			errSubstr: "selector must match",
		},
		{
			name:      "duplicate postgres slot name",
			mutate:    func(c *GenericBackupConfig) { c.Postgres = append(c.Postgres, c.Postgres[0]) },
			errSubstr: "duplicate postgres slot name",
		},
		{
			name:      "missing postgres cluster",
			mutate:    func(c *GenericBackupConfig) { c.Postgres[0].Cluster = "" },
			errSubstr: "cluster is required",
		},
		{
			name:      "missing files pvc",
			mutate:    func(c *GenericBackupConfig) { c.Files[0].PVC = "" },
			errSubstr: "pvc is required",
		},
		{
			name:      "missing s3 path",
			mutate:    func(c *GenericBackupConfig) { c.S3[0].Path = "" },
			errSubstr: "path is required",
		},
		{
			name:      "invalid slot name",
			mutate:    func(c *GenericBackupConfig) { c.Files[0].Name = "Bad_Name" },
			errSubstr: "not DNS/path-safe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validBackupConfig()
			tt.mutate(&c)
			err := c.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSubstr)
		})
	}
}

func TestGenericRestoreConfigValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, validRestoreConfig().Validate())
	})

	tests := []struct {
		name      string
		mutate    func(c *GenericRestoreConfig)
		errSubstr string
	}{
		{
			name:      "no sources",
			mutate:    func(c *GenericRestoreConfig) { c.Postgres = nil; c.Files = nil; c.FileGroups = nil; c.S3 = nil },
			errSubstr: "at least one source",
		},
		{
			name:      "missing postgres servingCert",
			mutate:    func(c *GenericRestoreConfig) { c.Postgres[0].ServingCert = "" },
			errSubstr: "servingCert is required",
		},
		{
			name:      "duplicate s3 slot name",
			mutate:    func(c *GenericRestoreConfig) { c.S3 = append(c.S3, c.S3[0]) },
			errSubstr: "duplicate s3 slot name",
		},
		{
			name:      "empty fileGroup selector",
			mutate:    func(c *GenericRestoreConfig) { c.FileGroups[0].Selector = metav1.LabelSelector{} },
			errSubstr: "selector must match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validRestoreConfig()
			tt.mutate(&c)
			err := c.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSubstr)
		})
	}
}

// TestGenericConfigYAMLRoundTrip exercises the full authoring path: the doc's example configs parse under
// goccy strict mode (every field is recognized) and then pass validation. A backup-only field in a restore
// file (and vice-versa) must be rejected by strict mode.
func TestGenericConfigYAMLRoundTrip(t *testing.T) {
	const backupYAML = `
namespace: vaultwarden
backupName: vaultwarden
backupVolume:
  storageClass: ceph-block
  snapshotClass: ceph-block-snap
  size: 10Gi
cleanupTimeout: 5m
postgres:
  - name: main
    cluster: vw-db
    clusterCloning:
      certificates:
        servingCert:
          certificateRequestPolicy:
            waitForCRPTimeout: 250ms
        clientCACert:
          certificateRequestPolicy:
            waitForCRPTimeout: 250ms
files:
  - name: data
    pvc: vw-data
fileGroups:
  - name: shards
    snapshotClass: ceph-block-group-snap
    selector:
      matchLabels:
        app: vw-shard
s3:
  - name: media
    path: s3://media-bucket/vw
    credentials:
      accessKeyId: AKIA
      secretAccessKey: secret
`

	const restoreYAML = `
namespace: vaultwarden
backupName: vaultwarden
cleanupTimeout: 5m
postgres:
  - name: main
    cluster: vw-db
    clientCAIssuer: { name: cnpg-client-ca, kind: ClusterIssuer }
    servingCert: vw-db-serving
    postgresUserCert:
      subject: { organizations: [vw] }
files:
  - name: data
    pvc: vw-data
fileGroups:
  - name: shards
    selector:
      matchLabels:
        app: vw-shard
s3:
  - name: media
    path: s3://media-bucket/vw
    # credentials omitted: falls back to AWS environment variables
`

	t.Run("backup", func(t *testing.T) {
		var c GenericBackupConfig
		require.NoError(t, yaml.UnmarshalWithOptions([]byte(backupYAML), &c, yaml.Strict()))
		require.NoError(t, c.Validate())
		assert.Equal(t, "ceph-block-snap", c.BackupVolume.SnapshotClass)
		require.Len(t, c.Postgres, 1)
		assert.Equal(t, "vw-db", c.Postgres[0].Cluster)
		require.Len(t, c.FileGroups, 1)
		assert.Equal(t, "ceph-block-group-snap", c.FileGroups[0].SnapshotClass)
		assert.Equal(t, map[string]string{"app": "vw-shard"}, c.FileGroups[0].Selector.MatchLabels)
	})

	t.Run("restore", func(t *testing.T) {
		var c GenericRestoreConfig
		require.NoError(t, yaml.UnmarshalWithOptions([]byte(restoreYAML), &c, yaml.Strict()))
		require.NoError(t, c.Validate())
		require.Len(t, c.Postgres, 1)
		require.NotNil(t, c.Postgres[0].PostgresUserCert.Subject)
		assert.Equal(t, []string{"vw"}, c.Postgres[0].PostgresUserCert.Subject.Organizations)
		require.Len(t, c.FileGroups, 1)
		assert.Equal(t, map[string]string{"app": "vw-shard"}, c.FileGroups[0].Selector.MatchLabels)
	})

	t.Run("backup-only field rejected in restore file", func(t *testing.T) {
		var c GenericRestoreConfig
		err := yaml.UnmarshalWithOptions([]byte(backupYAML), &c, yaml.Strict())
		require.Error(t, err)
	})
}

func TestNewGenericApp(t *testing.T) {
	mockClient := kubecluster.NewMockClientInterface(t)
	g := NewGenericApp(mockClient)

	require.NotNil(t, g)
	assert.Equal(t, mockClient, g.kubeClusterClient)
	assert.NotNil(t, g.newCNPGBackup)
	assert.NotNil(t, g.newCNPGRestore)
	assert.NotNil(t, g.newFilesBackup)
	assert.NotNil(t, g.newFilesRestore)
	assert.NotNil(t, g.newFilesGroupBackup)
	assert.NotNil(t, g.newFilesGroupRestore)
	assert.NotNil(t, g.newS3Sync)
	assert.NotNil(t, g.newRemoteStage)
}

func TestResolveS3Credentials(t *testing.T) {
	t.Run("inline credentials are used", func(t *testing.T) {
		resolved := resolveS3Credentials(s3.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "secret"})
		assert.Equal(t, "AKIA", resolved.GetAccessKeyID())
		assert.Equal(t, "secret", resolved.GetSecretAccessKey())
	})

	t.Run("empty credentials fall back to the environment", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "ENV-KEY")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "ENV-SECRET")
		resolved := resolveS3Credentials(s3.Credentials{})
		assert.Equal(t, "ENV-KEY", resolved.GetAccessKeyID())
		assert.Equal(t, "ENV-SECRET", resolved.GetSecretAccessKey())
	})
}

func pvcWithStorageRequest(t *testing.T, quantity string) *corev1.PersistentVolumeClaim {
	t.Helper()
	return &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(quantity)},
			},
		},
	}
}

func TestGenericAppBackupVolumeSize(t *testing.T) {
	t.Run("explicit size is used verbatim", func(t *testing.T) {
		g := &GenericApp{}
		size, err := g.backupVolumeSize(th.NewTestContext(), GenericBackupConfig{
			BackupVolume: GenericBackupVolume{Size: resource.MustParse("7Gi")},
		})
		require.NoError(t, err)
		assert.Equal(t, "7Gi", size.String())
	})

	t.Run("files-only sums the source PVC requests and doubles", func(t *testing.T) {
		mockClient := kubecluster.NewMockClientInterface(t)
		mockCore := core.NewMockClientInterface(t)
		mockClient.EXPECT().Core().Return(mockCore)
		mockCore.EXPECT().GetPVC(mock.Anything, "ns", "pvc-a").Return(pvcWithStorageRequest(t, "3Gi"), nil)
		mockCore.EXPECT().GetPVC(mock.Anything, "ns", "pvc-b").Return(pvcWithStorageRequest(t, "2Gi"), nil)

		g := &GenericApp{kubeClusterClient: mockClient}
		size, err := g.backupVolumeSize(th.NewTestContext(), GenericBackupConfig{
			Namespace: "ns",
			Files:     []GenericFilesBackupSource{{GenericFilesSource: GenericFilesSource{Name: "a", PVC: "pvc-a"}}, {GenericFilesSource: GenericFilesSource{Name: "b", PVC: "pvc-b"}}},
		})
		require.NoError(t, err)
		assert.Equal(t, "10Gi", size.String()) // (3 + 2) * 2
	})

	t.Run("error getting a files PVC", func(t *testing.T) {
		mockClient := kubecluster.NewMockClientInterface(t)
		mockCore := core.NewMockClientInterface(t)
		mockClient.EXPECT().Core().Return(mockCore)
		mockCore.EXPECT().GetPVC(mock.Anything, "ns", "pvc-a").Return(nil, assert.AnError)

		g := &GenericApp{kubeClusterClient: mockClient}
		_, err := g.backupVolumeSize(th.NewTestContext(), GenericBackupConfig{
			Namespace: "ns",
			Files:     []GenericFilesBackupSource{{GenericFilesSource: GenericFilesSource{Name: "a", PVC: "pvc-a"}}},
		})
		require.Error(t, err)
	})
}

func TestGenericAppBackup(t *testing.T) {
	tests := []struct {
		desc                          string
		simulateNewDRVolumeError      bool
		simulateConfigurePgErr        bool
		simulateConfigureFilesErr     bool
		simulateConfigureFileGroupErr bool
		simulateConfigureS3Err        bool
		simulateRunError              bool
		simulateSnapshotError         bool
	}{
		{desc: "success"},
		{desc: "error creating DR volume", simulateNewDRVolumeError: true},
		{desc: "error configuring postgres", simulateConfigurePgErr: true},
		{desc: "error configuring files", simulateConfigureFilesErr: true},
		{desc: "error configuring fileGroup", simulateConfigureFileGroupErr: true},
		{desc: "error configuring s3", simulateConfigureS3Err: true},
		{desc: "error running", simulateRunError: true},
		{desc: "error snapshotting", simulateSnapshotError: true},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			config := validBackupConfig()
			namespace := config.Namespace
			backupName := config.BackupName

			mockClient := kubecluster.NewMockClientInterface(t)
			mockDRVolume := drvolume.NewMockDRVolumeInterface(t)
			mockStage := remote.NewMockRemoteStageInterface(t)
			mockPg := cnpgbackup.NewMockCNPGBackupInterface(t)
			mockFiles := filesbackup.NewMockFilesBackupInterface(t)
			mockFilesGroup := filesgroupbackup.NewMockFilesGroupBackupInterface(t)
			mockS3 := s3sync.NewMockS3SyncInterface(t)

			var registered []string

			g := &GenericApp{
				kubeClusterClient:   mockClient,
				newCNPGBackup:       func() cnpgbackup.CNPGBackupInterface { return mockPg },
				newFilesBackup:      func() filesbackup.FilesBackupInterface { return mockFiles },
				newFilesGroupBackup: func() filesgroupbackup.FilesGroupBackupInterface { return mockFilesGroup },
				newS3Sync:           func() s3sync.S3SyncInterface { return mockS3 },
				newRemoteStage: func(c kubecluster.ClientInterface, ns, eventName string, opts remote.RemoteStageOptions) remote.RemoteStageInterface {
					assert.Equal(t, mockClient, c)
					assert.Equal(t, namespace, ns)
					assert.Contains(t, eventName, backupName)
					assert.Equal(t, config.CleanupTimeout, opts.CleanupTimeout)
					return mockStage
				},
			}

			rootCtx := th.NewTestContext()
			wantErr := th.ErrExpected(
				tt.simulateNewDRVolumeError,
				tt.simulateConfigurePgErr,
				tt.simulateConfigureFilesErr,
				tt.simulateConfigureFileGroupErr,
				tt.simulateConfigureS3Err,
				tt.simulateRunError,
				tt.simulateSnapshotError,
			)

			mockStage.EXPECT().WithAction(mock.Anything, mock.Anything).RunAndReturn(
				func(name string, action remote.RemoteAction) remote.RemoteStageInterface {
					registered = append(registered, name)
					return mockStage
				}).Maybe()

			func() {
				mockClient.EXPECT().NewDRVolume(mock.Anything, namespace, backupName, mock.Anything, mock.Anything).
					RunAndReturn(func(ctx *contexts.Context, ns, name string, size resource.Quantity, opts drvolume.DRVolumeCreateOptions) (drvolume.DRVolumeInterface, error) {
						assert.True(t, ctx.IsChildOf(rootCtx))
						assert.Equal(t, "10Gi", size.String())
						assert.ElementsMatch(t, []string{"vw-db"}, opts.CNPGClusterNames)
						return th.ErrOr1Val(mockDRVolume, tt.simulateNewDRVolumeError)
					})
				if tt.simulateNewDRVolumeError {
					return
				}

				mockPg.EXPECT().Configure(mockClient, namespace, "vw-db", backupName, "main.sql", cnpgbackup.CNPGBackupOptions{
					CloningOpts:    config.Postgres[0].ClusterCloning,
					CleanupTimeout: config.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigurePgErr))
				if tt.simulateConfigurePgErr {
					return
				}

				mockFiles.EXPECT().Configure(mockClient, namespace, "vw-data", backupName, "data", filesbackup.FilesBackupOptions{
					SnapshotClass:  config.Files[0].SnapshotClass,
					CleanupTimeout: config.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigureFilesErr))
				if tt.simulateConfigureFilesErr {
					return
				}

				mockFilesGroup.EXPECT().Configure(mockClient, namespace, config.FileGroups[0].Selector, backupName, "shards", filesgroupbackup.FilesGroupBackupOptions{
					SnapshotClass:  config.FileGroups[0].SnapshotClass,
					CleanupTimeout: config.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigureFileGroupErr))
				if tt.simulateConfigureFileGroupErr {
					return
				}

				mockS3.EXPECT().Configure(mockClient, namespace, backupName, "media", "s3://media-bucket/vw", mock.Anything, s3sync.DirectionDownload, s3sync.S3SyncOptions{}).
					RunAndReturn(func(c kubecluster.ClientInterface, ns, drVolName, backupDirRelPath, s3Path string, creds s3.CredentialsInterface, direction s3sync.Direction, opts s3sync.S3SyncOptions) error {
						assert.Equal(t, "AKIA", creds.GetAccessKeyID())
						return th.ErrIfTrue(tt.simulateConfigureS3Err)
					})
				if tt.simulateConfigureS3Err {
					return
				}

				mockStage.EXPECT().Run(mock.Anything).RunAndReturn(func(ctx *contexts.Context) error {
					assert.True(t, ctx.IsChildOf(rootCtx))
					return th.ErrIfTrue(tt.simulateRunError)
				})
				if tt.simulateRunError {
					return
				}

				mockDRVolume.EXPECT().SnapshotAndWaitReady(mock.Anything, mock.Anything, drvolume.DRVolumeSnapshotAndWaitOptions{}).
					RunAndReturn(func(ctx *contexts.Context, snapshotName string, opts drvolume.DRVolumeSnapshotAndWaitOptions) error {
						assert.True(t, ctx.IsChildOf(rootCtx))
						assert.Contains(t, snapshotName, helpers.CleanName(backupName))
						return th.ErrIfTrue(tt.simulateSnapshotError)
					})
			}()

			backup, err := g.Backup(rootCtx, config)

			require.NotNil(t, backup)
			if wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Fixed, consistency-correct registration order: postgres, then files, then fileGroups, then s3.
				assert.Equal(t, []string{`postgres "main" backup`, `files "data" backup`, `fileGroup "shards" backup`, `s3 "media" sync`}, registered)
			}
		})
	}
}

func TestGenericAppBackupInvalidConfig(t *testing.T) {
	// An invalid config is rejected before any resource is touched (no client calls).
	g := &GenericApp{kubeClusterClient: kubecluster.NewMockClientInterface(t)}
	_, err := g.Backup(th.NewTestContext(), GenericBackupConfig{Namespace: "ns", BackupName: "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one source")
}

func TestGenericAppRestore(t *testing.T) {
	tests := []struct {
		desc                          string
		simulateConfigurePgErr        bool
		simulateConfigureFilesErr     bool
		simulateConfigureFileGroupErr bool
		simulateConfigureS3Err        bool
		simulateRunError              bool
	}{
		{desc: "success"},
		{desc: "error configuring postgres", simulateConfigurePgErr: true},
		{desc: "error configuring files", simulateConfigureFilesErr: true},
		{desc: "error configuring fileGroup", simulateConfigureFileGroupErr: true},
		{desc: "error configuring s3", simulateConfigureS3Err: true},
		{desc: "error running", simulateRunError: true},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			config := validRestoreConfig()
			namespace := config.Namespace
			restoreName := config.BackupName

			mockClient := kubecluster.NewMockClientInterface(t)
			mockStage := remote.NewMockRemoteStageInterface(t)
			mockPg := cnpgrestore.NewMockCNPGRestoreInterface(t)
			mockFiles := filesrestore.NewMockFilesRestoreInterface(t)
			mockFilesGroup := filesgrouprestore.NewMockFilesGroupRestoreInterface(t)
			mockS3 := s3sync.NewMockS3SyncInterface(t)

			var registered []string

			g := &GenericApp{
				kubeClusterClient:    mockClient,
				newCNPGRestore:       func() cnpgrestore.CNPGRestoreInterface { return mockPg },
				newFilesRestore:      func() filesrestore.FilesRestoreInterface { return mockFiles },
				newFilesGroupRestore: func() filesgrouprestore.FilesGroupRestoreInterface { return mockFilesGroup },
				newS3Sync:            func() s3sync.S3SyncInterface { return mockS3 },
				newRemoteStage: func(c kubecluster.ClientInterface, ns, eventName string, opts remote.RemoteStageOptions) remote.RemoteStageInterface {
					assert.Equal(t, mockClient, c)
					assert.Equal(t, namespace, ns)
					assert.Contains(t, eventName, restoreName)
					return mockStage
				},
			}

			rootCtx := th.NewTestContext()
			wantErr := th.ErrExpected(
				tt.simulateConfigurePgErr,
				tt.simulateConfigureFilesErr,
				tt.simulateConfigureFileGroupErr,
				tt.simulateConfigureS3Err,
				tt.simulateRunError,
			)

			mockStage.EXPECT().WithAction(mock.Anything, mock.Anything).RunAndReturn(
				func(name string, action remote.RemoteAction) remote.RemoteStageInterface {
					registered = append(registered, name)
					return mockStage
				}).Maybe()

			func() {
				mockPg.EXPECT().Configure(mockClient, namespace, "vw-db", "vw-db-serving", config.Postgres[0].ClientCAIssuer, restoreName, "main.sql", cnpgrestore.CNPGRestoreOptions{
					PostgresUserCert: config.Postgres[0].PostgresUserCert,
					CleanupTimeout:   config.CleanupTimeout,
				}).Return(th.ErrIfTrue(tt.simulateConfigurePgErr))
				if tt.simulateConfigurePgErr {
					return
				}

				mockFiles.EXPECT().Configure(mockClient, namespace, "vw-data", restoreName, "data", filesrestore.FilesRestoreOptions{}).
					Return(th.ErrIfTrue(tt.simulateConfigureFilesErr))
				if tt.simulateConfigureFilesErr {
					return
				}

				mockFilesGroup.EXPECT().Configure(mockClient, namespace, config.FileGroups[0].Selector, restoreName, "shards", filesgrouprestore.FilesGroupRestoreOptions{}).
					Return(th.ErrIfTrue(tt.simulateConfigureFileGroupErr))
				if tt.simulateConfigureFileGroupErr {
					return
				}

				mockS3.EXPECT().Configure(mockClient, namespace, restoreName, "media", "s3://media-bucket/vw", mock.Anything, s3sync.DirectionUpload, s3sync.S3SyncOptions{}).
					RunAndReturn(func(c kubecluster.ClientInterface, ns, drVolName, backupDirRelPath, s3Path string, creds s3.CredentialsInterface, direction s3sync.Direction, opts s3sync.S3SyncOptions) error {
						assert.Equal(t, "AKIA", creds.GetAccessKeyID())
						return th.ErrIfTrue(tt.simulateConfigureS3Err)
					})
				if tt.simulateConfigureS3Err {
					return
				}

				mockStage.EXPECT().Run(mock.Anything).RunAndReturn(func(ctx *contexts.Context) error {
					assert.True(t, ctx.IsChildOf(rootCtx))
					return th.ErrIfTrue(tt.simulateRunError)
				})
			}()

			restore, err := g.Restore(rootCtx, config)

			require.NotNil(t, restore)
			if wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, []string{`postgres "main" restore`, `files "data" restore`, `fileGroup "shards" restore`, `s3 "media" sync`}, registered)
			}
		})
	}
}
