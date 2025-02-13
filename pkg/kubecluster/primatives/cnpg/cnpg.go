package cnpg

import (
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const podMonitorCRDName = "podmonitors.monitoring.coreos.com"

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
			Online: ptr.To(true),
			OnlineConfiguration: &apiv1.OnlineConfiguration{
				WaitForArchive: ptr.To(true), // Needed to ensure that backups are consistent
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

// This doesn't need to support every option - just the ones that may be relavent to backups.
type CreateClusterOptions struct {
	helpers.GenerateName
	BackupName           string                // Create a new cluster from a backup
	RecoveryTarget       *apiv1.RecoveryTarget // Only valid if BackupName is set
	DatabaseName         string
	OwnerName            string
	StorageClass         string
	ResourceRequirements corev1.ResourceRequirements
}

// Create a cluster for backup/recovery purposes specifically. Not intended for use general use. The created cluster should not be used by applications.
// Created cluster contains a single database server instance. Cluster can optionally be created from a backup. TLS authentication is required.
func (cnpgc *Client) CreateCluster(ctx *contexts.Context, namespace, clusterName string,
	volumeSize resource.Quantity, servingCertificateSecretName, clientCASecretName, replicationUserCertName string,
	opts CreateClusterOptions) (*apiv1.Cluster, error) {
	ctx.Log.With("clusterName", clusterName).Info("Creating cluster")
	ctx.Log.Debug("Call parameters", "volumeSize", volumeSize.String(), "servingCertificateSecretName", servingCertificateSecretName, "clientCASecretName", clientCASecretName, "replicationUserCertName", replicationUserCertName, "opts", opts)

	cluster := &apiv1.Cluster{
		Spec: apiv1.ClusterSpec{
			Instances: 1,
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

	if opts.BackupName != "" {
		cluster.Spec.Bootstrap.Recovery = &apiv1.BootstrapRecovery{
			Backup: &apiv1.BackupSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: opts.BackupName,
				},
			},
			RecoveryTarget: opts.RecoveryTarget,
			Database:       opts.DatabaseName,
			Owner:          opts.OwnerName,
		}
	} else {
		if opts.DatabaseName != "" && opts.OwnerName != "" {
			cluster.Spec.Bootstrap.InitDB = &apiv1.BootstrapInitDB{
				Database: opts.DatabaseName,
				Owner:    opts.OwnerName,
			}
		}
	}

	if opts.StorageClass != "" {
		cluster.Spec.StorageConfiguration.StorageClass = &opts.StorageClass
	}

	cluster.Spec.PostgresConfiguration.PgHBA = []string{
		// Require TLS auth for all connection
		"hostssl all all all cert",
	}

	_, err := cnpgc.apiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx.Child(), podMonitorCRDName, metav1.GetOptions{})
	if err == nil {
		// Enable metrics if the required CRD exists
		cluster.Spec.Monitoring = &apiv1.MonitoringConfiguration{
			EnablePodMonitor: true,
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, trace.Wrap(err, "failed to check if cluster has %q CRD", podMonitorCRDName)
	}

	cluster, err = cnpgc.cnpgClient.PostgresqlV1().Clusters(namespace).Create(ctx.Child(), cluster, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create cluster %q")
	}

	return cluster, nil
}

type WaitForReadyClusterOpts struct {
	helpers.MaxWaitTime
}

func (cnpgc *Client) WaitForReadyCluster(ctx *contexts.Context, namespace, name string, opts WaitForReadyClusterOpts) (cluster *apiv1.Cluster, err error) {
	ctx.Log.With("name", name).Info("Waiting for cluster to become ready")
	defer ctx.Log.Info("Finished waiting for cluster to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	precondition := func(ctx *contexts.Context, cluster *apiv1.Cluster) (*apiv1.Cluster, bool, error) {
		ctx.Log.Debug("Cluster conditions", "conditions", cluster.Status.Conditions)

		isReady := false
		for _, condition := range cluster.Status.Conditions {
			if condition.Type != string(apiv1.ConditionClusterReady) {
				continue
			}

			isReady = condition.Status == metav1.ConditionStatus(apiv1.ConditionTrue)
			break
		}

		if isReady {
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
