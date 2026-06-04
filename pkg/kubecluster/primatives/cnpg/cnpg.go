package cnpg

import (
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BarmanCloudPluginName is the CNPG-I plugin name of the barman-cloud plugin, which handles WAL
// archiving for the source clusters this tool clones.
const BarmanCloudPluginName = "barman-cloud.cloudnative-pg.io"

// volumeSnapshotAPIGroup is the API group of the external-snapshotter VolumeSnapshot resources
// referenced when recovering a cluster from volume snapshots.
const volumeSnapshotAPIGroup = "snapshot.storage.k8s.io"

type CreateBackupOptions struct {
	helpers.GenerateName
	Method *apiv1.BackupMethod
	Target *apiv1.BackupTarget
}

func (cnpgc *Client) CreateBackup(ctx *contexts.Context, namespace, backupName, clusterName string, opts CreateBackupOptions) (*apiv1.Backup, error) {
	ctx.Log.With("backupName", backupName).Info("Creating backup")
	ctx.Log.Debug("Call parameters", "clusterName", clusterName, "opts", opts)

	backup := &apiv1.Backup{
		Spec: apiv1.BackupSpec{
			Cluster: apiv1.LocalObjectReference{
				Name: clusterName,
			},
			Method: apiv1.BackupMethodVolumeSnapshot,
			Target: apiv1.DefaultBackupTarget,
			Online: new(true),
			OnlineConfiguration: &apiv1.OnlineConfiguration{
				WaitForArchive: new(true), // Needed to ensure that backups are consistent
			},
		},
	}

	opts.SetName(&backup.ObjectMeta, backupName)

	if opts.Target != nil {
		backup.Spec.Target = *opts.Target
	}

	if opts.Method != nil {
		backup.Spec.Method = *opts.Method
	}

	backup, err := cnpgc.cnpgClient.PostgresqlV1().Backups(namespace).Create(ctx.Child(), backup, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create backup %q", helpers.FullNameStr(namespace, backupName))
	}

	return backup, nil
}

type WaitForReadyBackupOpts struct {
	helpers.MaxWaitTime
}

func (cnpgc *Client) WaitForReadyBackup(ctx *contexts.Context, namespace, name string, opts WaitForReadyBackupOpts) (backup *apiv1.Backup, err error) {
	ctx.Log.With("name", name).Info("Waiting for backup to become ready")
	defer ctx.Log.Info("Finished waiting for backup to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	precondition := func(ctx *contexts.Context, backup *apiv1.Backup) (*apiv1.Backup, bool, error) {
		ctx.Log.Debug("Backup phase", "phase", backup.Status.Phase)

		switch backup.Status.Phase {
		case apiv1.BackupPhaseCompleted:
			return backup, true, nil
		case apiv1.BackupPhaseFailed:
			fallthrough
		case apiv1.BackupPhaseWalArchivingFailing:
			return nil, false, trace.Errorf("backup %q failed in state %q", helpers.FullName(backup), backup.Status.Phase)
		default:
			return nil, false, nil
		}
	}
	backup, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(10*time.Minute), cnpgc.cnpgClient.PostgresqlV1().Backups(namespace), name, precondition)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for backup %q to become ready", helpers.FullNameStr(namespace, name))
	}

	return backup, nil
}

func (cnpgc *Client) DeleteBackup(ctx *contexts.Context, namespace, name string) error {
	ctx.Log.With("name", name).Info("Deleting backup")

	// TODO maybe delete snapshot, if backup was created with snapshot and it wasn't deleted?
	// This would go against the configured policy (on cluster and/or snapshot class), but given that this
	// snapshot is applicaiton specific, this may make sense.
	err := cnpgc.cnpgClient.PostgresqlV1().Backups(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete backup %q", helpers.FullNameStr(namespace, name))
}

// VolumeSnapshotRecovery configures recovery of a new cluster from volume snapshots, with
// write-ahead logs fetched from an external cluster's WAL archive. This is the recovery path
// used for source clusters that archive WAL with the barman-cloud plugin.
type VolumeSnapshotRecovery struct {
	// DataSnapshotName is the name of the VolumeSnapshot holding PGDATA.
	DataSnapshotName string
	// WALSnapshotName is the name of the VolumeSnapshot holding the PG_WAL volume, if the source
	// cluster stores WAL on a separate volume. Optional.
	WALSnapshotName string
	// WALSource describes the external cluster from which write-ahead logs are fetched during
	// recovery. Its Name is used both as the bootstrap recovery source and, by CNPG convention,
	// as the server name (folder) the source cluster's backups are stored under, so it must match
	// the source cluster's barman server name.
	WALSource apiv1.ExternalCluster
	// RecoveryTargetTime chooses how far recovery proceeds. When set (RFC3339), the clone recovers
	// forward to that wall-clock instant (recoveryTarget.targetTime); when empty it stops at the
	// backup's consistency point (recoveryTarget.targetImmediate).
	RecoveryTargetTime string
}

// This doesn't need to support every option - just the ones that may be relavent to backups.
type CreateClusterOptions struct {
	helpers.GenerateName
	// VolumeSnapshotRecovery recovers the new cluster from volume snapshots, fetching WAL from an
	// external cluster (the recovery path for barman-cloud-plugin source clusters). It carries its
	// own recovery target.
	VolumeSnapshotRecovery *VolumeSnapshotRecovery
	DatabaseName           string
	OwnerName              string
	StorageClass           string
	ResourceRequirements   corev1.ResourceRequirements
	ImageName              string
}

// Create a cluster for backup/recovery purposes specifically. Not intended for use general use. The created cluster should not be used by applications.
// Created cluster contains a single database server instance. Cluster can optionally be created from a backup. TLS authentication is required.
func (cnpgc *Client) CreateCluster(ctx *contexts.Context, namespace, clusterName string,
	volumeSize resource.Quantity, servingCertificateSecretName, clientCASecretName, replicationUserCertName string,
	opts CreateClusterOptions) (*apiv1.Cluster, error) {
	ctx.Log.With("clusterName", clusterName).Info("Creating cluster")
	ctx.Log.Debug("Call parameters", "volumeSize", volumeSize.String(), "servingCertificateSecretName", servingCertificateSecretName, "clientCASecretName", clientCASecretName, "replicationUserCertName", replicationUserCertName, "opts", opts)

	cluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/component": "cnpg-cluster",
			},
		},
		Spec: apiv1.ClusterSpec{
			Instances: 1,
			ImageName: opts.ImageName,
			Bootstrap: &apiv1.BootstrapConfiguration{},
			StorageConfiguration: apiv1.StorageConfiguration{
				Size: volumeSize.String(),
			},
			Resources: opts.ResourceRequirements,
			Certificates: &apiv1.CertificatesConfiguration{
				ServerTLSSecret:      servingCertificateSecretName,
				ServerCASecret:       servingCertificateSecretName,
				ClientCASecret:       clientCASecretName,
				ReplicationTLSSecret: replicationUserCertName,
			},
		},
	}

	opts.SetName(&cluster.ObjectMeta, clusterName)

	if opts.VolumeSnapshotRecovery != nil {
		recoveryTarget := &apiv1.RecoveryTarget{TargetImmediate: new(true)}
		if opts.VolumeSnapshotRecovery.RecoveryTargetTime != "" {
			recoveryTarget = &apiv1.RecoveryTarget{TargetTime: opts.VolumeSnapshotRecovery.RecoveryTargetTime}
		}
		recovery := &apiv1.BootstrapRecovery{
			Source: opts.VolumeSnapshotRecovery.WALSource.Name,
			VolumeSnapshots: &apiv1.DataSource{
				Storage: corev1.TypedLocalObjectReference{
					APIGroup: new(volumeSnapshotAPIGroup),
					Kind:     "VolumeSnapshot",
					Name:     opts.VolumeSnapshotRecovery.DataSnapshotName,
				},
			},
			RecoveryTarget: recoveryTarget,
			Database:       opts.DatabaseName,
			Owner:          opts.OwnerName,
		}
		if opts.VolumeSnapshotRecovery.WALSnapshotName != "" {
			recovery.VolumeSnapshots.WalStorage = &corev1.TypedLocalObjectReference{
				APIGroup: new(volumeSnapshotAPIGroup),
				Kind:     "VolumeSnapshot",
				Name:     opts.VolumeSnapshotRecovery.WALSnapshotName,
			}
		}
		cluster.Spec.Bootstrap.Recovery = recovery
		cluster.Spec.ExternalClusters = []apiv1.ExternalCluster{opts.VolumeSnapshotRecovery.WALSource}
	} else if opts.DatabaseName != "" && opts.OwnerName != "" {
		cluster.Spec.Bootstrap.InitDB = &apiv1.BootstrapInitDB{
			Database: opts.DatabaseName,
			Owner:    opts.OwnerName,
		}
	}

	if opts.StorageClass != "" {
		cluster.Spec.StorageConfiguration.StorageClass = &opts.StorageClass
	}

	cluster.Spec.PostgresConfiguration.PgHBA = []string{
		// Require TLS auth for all connection
		"hostssl all all all cert",
	}

	cnpgc.LabelResource(cluster)

	cluster, err := cnpgc.cnpgClient.PostgresqlV1().Clusters(namespace).Create(ctx.Child(), cluster, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create cluster %q", clusterName)
	}

	return cluster, nil
}

func IsClusterReady(cluster *apiv1.Cluster) bool {
	for _, condition := range cluster.Status.Conditions {
		if condition.Type != string(apiv1.ConditionClusterReady) {
			continue
		}

		return condition.Status == metav1.ConditionStatus(apiv1.ConditionTrue)
	}

	return false
}

type WaitForReadyClusterOpts struct {
	helpers.MaxWaitTime
}

func (cnpgc *Client) WaitForReadyCluster(ctx *contexts.Context, namespace, name string, opts WaitForReadyClusterOpts) (cluster *apiv1.Cluster, err error) {
	ctx.Log.With("name", name).Info("Waiting for cluster to become ready")
	defer ctx.Log.Info("Finished waiting for cluster to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	precondition := func(ctx *contexts.Context, cluster *apiv1.Cluster) (*apiv1.Cluster, bool, error) {
		ctx.Log.Debug("Cluster conditions", "conditions", cluster.Status.Conditions)
		if IsClusterReady(cluster) {
			return cluster, true, nil
		}
		return nil, false, nil
	}
	cluster, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(10*time.Minute), cnpgc.cnpgClient.PostgresqlV1().Clusters(namespace), name, precondition)

	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for cluster %q to become ready", helpers.FullNameStr(namespace, name))
	}
	return cluster, nil
}

func (cnpgc *Client) GetCluster(ctx *contexts.Context, namespace, name string) (*apiv1.Cluster, error) {
	ctx.Log.With("name", name).Info("Getting cluster")

	cluster, err := cnpgc.cnpgClient.PostgresqlV1().Clusters(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to delete CNPG cluster %q", helpers.FullNameStr(namespace, name))
	}

	ctx.Log.Debug("Retrieved cluster", "cluster", cluster)
	return cluster, nil
}

func (cnpgc *Client) DeleteCluster(ctx *contexts.Context, namespace, name string) error {
	ctx.Log.With("name", name).Info("Deleting cluster")

	err := cnpgc.cnpgClient.PostgresqlV1().Clusters(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete CNPG cluster %q", helpers.FullNameStr(namespace, name))
}

type WaitForClusterDeletedOpts struct {
	helpers.MaxWaitTime
}

// WaitForClusterDeleted blocks until the named cluster no longer exists, or the timeout elapses.
// It is used after DeleteCluster when a cluster must be recreated under the same name (the recovery
// fallback), since CNPG cluster deletion is asynchronous (finalizers tear down pods/PVCs first).
func (cnpgc *Client) WaitForClusterDeleted(ctx *contexts.Context, namespace, name string, opts WaitForClusterDeletedOpts) (err error) {
	ctx.Log.With("name", name).Info("Waiting for cluster to be deleted")
	defer ctx.Log.Info("Finished waiting for cluster to be deleted", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	err = helpers.WaitForResourceDeletion[*apiv1.Cluster](ctx.Child(), opts.MaxWait(10*time.Minute), cnpgc.cnpgClient.PostgresqlV1().Clusters(namespace), name)
	return trace.Wrap(err, "failed waiting for cluster %q to be deleted", helpers.FullNameStr(namespace, name))
}

type KubernetesSecretCredentials struct {
	Host                         string
	Port                         string
	User                         string // TODO maybe pull this from client cert CN
	ServingCertificateCAFilePath string // Must be PEM encoded
	ClientCertificateFilePath    string // Must be PEM encoded
	ClientPrivateKeyFilePath     string // Must be PEM encoded
}

func (ksc *KubernetesSecretCredentials) GetUsername() string {
	if ksc.User == "" {
		return "postgres"
	}

	return ksc.User
}

func (ksc *KubernetesSecretCredentials) GetHost() string {
	return ksc.Host
}

func (ksc *KubernetesSecretCredentials) GetPort() string {
	if ksc.Port != "" {
		return ksc.Port
	}

	return postgres.PostgresDefaultPort
}

func (ksc *KubernetesSecretCredentials) GetVariables() postgres.CredentialVariables {
	return map[postgres.CredentialVariable]string{
		postgres.HostVarName:        ksc.GetHost(),
		postgres.PortVarName:        ksc.GetPort(),
		postgres.UserVarName:        ksc.GetUsername(),
		postgres.RequireAuthVarName: "none",        // Require TLS auth. Don't allow the server to ask the client for a password/similar.
		postgres.SSLModeVarName:     "verify-full", // Check the server hostname against the cert, and validate the cert chain
		postgres.SSLCertVarName:     ksc.ClientCertificateFilePath,
		postgres.SSLKeyVarName:      ksc.ClientPrivateKeyFilePath,
		postgres.SSLRootCertVarName: ksc.ServingCertificateCAFilePath,
	}
}
