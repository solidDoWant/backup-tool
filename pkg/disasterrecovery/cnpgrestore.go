package disasterrecovery

import (
	"fmt"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
)

// Performs a CNPG logical recovery. Fields are for state tracking. Callers should:
// 1. Populate the struct with `Configure`
// 2. Validate that the required resources are ready with `CheckResourcesReady`
// 3. Restore the backup with `Restore`
// Separating this out into multiple states/steps/stages allows callers that need to
// restore multiple clusters to perform each step for all clusters before moving on
// to the next one.
type CNPGRestoreInterface interface {
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, clusterName, servingCertName,
		clientCertIssuerName, drVolName, fullRestoreName, backupFileRelPath string, opts CNPGRestoreOpts)
	CheckResourcesReady(ctx *contexts.Context) error
	Restore(ctx *contexts.Context) error
}

type CNPGRestoreOpts struct {
	PostgresUserCert            OptionsClusterUserCert                             `yaml:"postgresUserCert,omitempty"`
	RemoteBackupToolOptions     backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	CleanupTimeout              helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
	ClusterServiceSearchDomains []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
}

type CNPGRestore struct {
	// Provided by caller
	kubeClusterClient    kubecluster.ClientInterface
	namespace            string
	clusterName          string
	servingCertName      string
	clientCertIssuerName string
	drVolName            string
	fullRestoreName      string

	backupFileRelPath string
	opts              CNPGRestoreOpts
	// Populated during ready check to avoid redundant lookups
	cluster          *apiv1.Cluster
	servingCert      *certmanagerv1.Certificate
	clientCertIssuer *certmanagerv1.Issuer
}

func NewCNPGRestore() CNPGRestoreInterface {
	return &CNPGRestore{}
}

func (cnpgr *CNPGRestore) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, clusterName, servingCertName,
	clientCertIssuerName, drVolName, fullRestoreName, backupFileRelPath string, opts CNPGRestoreOpts) {
	cnpgr.kubeClusterClient = kubeClusterClient
	cnpgr.namespace = namespace
	cnpgr.clusterName = clusterName
	cnpgr.servingCertName = servingCertName
	cnpgr.clientCertIssuerName = clientCertIssuerName
	cnpgr.drVolName = drVolName
	cnpgr.fullRestoreName = fullRestoreName
	cnpgr.backupFileRelPath = backupFileRelPath
	cnpgr.opts = opts
}

// Return value is just for chaining convenience
func (cnpgr *CNPGRestore) ctxLogWith(ctx *contexts.Context) *contexts.LoggerContext {
	return ctx.Log.With("clusterName", cnpgr.clusterName)
}

func (cnpgr *CNPGRestore) CheckResourcesReady(ctx *contexts.Context) error {
	cnpgr.ctxLogWith(ctx)

	cluster, err := cnpgr.kubeClusterClient.CNPG().GetCluster(ctx.Child(), cnpgr.namespace, cnpgr.clusterName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster %q", cnpgr.clusterName)
	}
	if !cnpg.IsClusterReady(cluster) {
		return trace.Errorf("CNPG cluster %q is not ready", cnpgr.clusterName)
	}
	cnpgr.cluster = cluster

	servingCert, err := cnpgr.kubeClusterClient.CM().GetCertificate(ctx.Child(), cnpgr.namespace, cnpgr.servingCertName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster serving cert %q", cnpgr.clusterName)
	}
	cnpgr.servingCert = servingCert

	clientCertIssuer, err := cnpgr.kubeClusterClient.CM().GetIssuer(ctx.Child(), cnpgr.namespace, cnpgr.clientCertIssuerName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster client cert issuer %q", cnpgr.clientCertIssuerName)
	}
	if !certmanager.IsIssuerReady(clientCertIssuer) {
		return trace.Errorf("CNPG cluster client cert issuer %q is not ready", cnpgr.clientCertIssuerName)
	}
	cnpgr.clientCertIssuer = clientCertIssuer

	return nil
}

func (cnpgr *CNPGRestore) Restore(ctx *contexts.Context) (err error) {
	cnpgr.ctxLogWith(ctx).Info("Restoring backup to CNPG cluster")

	// 1. Create postgres user certs for the cluster
	ctx.Log.Step().Info("Creating CNPG cluster client cert")
	cucOptions := clusterusercert.NewClusterUserCertOpts{
		Subject:            cnpgr.opts.PostgresUserCert.Subject,
		CRPOpts:            cnpgr.opts.PostgresUserCert.CRPOpts,
		WaitForCertTimeout: cnpgr.opts.PostgresUserCert.WaitForReadyTimeout,
		CleanupTimeout:     cnpgr.opts.CleanupTimeout,
	}
	postgresUserCert, err := cnpgr.kubeClusterClient.NewClusterUserCert(ctx.Child(), cnpgr.namespace, "postgres", cnpgr.clientCertIssuerName, cnpgr.clusterName, cucOptions)
	if err != nil {
		return trace.Wrap(err, "failed to create postgres user CNPG cluster client cert")
	}
	defer cleanup.To(postgresUserCert.Delete).WithErrMessage("failed to cleanup postgres user CNPG cluster client cert resources").WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(cnpgr.opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	// 2. Spawn a new backup-tool pod with postgres auth and serving certs, and DR mounts attached
	ctx.Log.Step().Info("Creating backup tool instance")
	drVolumeMountPath := filepath.Join(teleportBaseMountPath, "dr")
	secretsVolumeMountPath := filepath.Join(teleportBaseMountPath, "secrets")
	servingCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "serving-cert")
	clientCertVolumeMountPath := filepath.Join(secretsVolumeMountPath, "client-cert")
	btOpts := backuptoolinstance.CreateBackupToolInstanceOptions{
		NamePrefix: fmt.Sprintf("%s-%s-%s", constants.ToolName, cnpgr.fullRestoreName, "core"),
		Volumes: []core.SingleContainerVolume{
			core.NewSingleContainerPVC(cnpgr.drVolName, drVolumeMountPath),
			core.NewSingleContainerSecret(cnpgr.servingCert.Spec.SecretName, servingCertVolumeMountPath, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
			core.NewSingleContainerSecret(postgresUserCert.GetCertificate().Spec.SecretName, clientCertVolumeMountPath),
		},
		CleanupTimeout: cnpgr.opts.CleanupTimeout,
	}
	mergo.MergeWithOverwrite(&btOpts, cnpgr.opts.RemoteBackupToolOptions)
	btInstance, err := cnpgr.kubeClusterClient.CreateBackupToolInstance(ctx.Child(), cnpgr.namespace, cnpgr.fullRestoreName, btOpts)
	if err != nil {
		return trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", cnpgr.fullRestoreName).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(cnpgr.opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child(), cnpgr.opts.ClusterServiceSearchDomains...)
	if err != nil {
		return trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	// 3. Perform a CNPG logical recovery of the core and audit CNPG clusters
	ctx.Log.Step().Info("Performing Postgres logical recovery")
	podSQLFilePath := filepath.Join(drVolumeMountPath, cnpgr.backupFileRelPath)
	clusterCredentials := &postgres.EnvironmentCredentials{
		postgres.HostVarName:        fmt.Sprintf("%s.%s.svc", cnpgr.cluster.Status.WriteService, cnpgr.namespace),
		postgres.UserVarName:        "postgres",
		postgres.RequireAuthVarName: "none",        // Require TLS auth. Don't allow the server to ask the client for a password/similar.
		postgres.SSLModeVarName:     "verify-full", // Check the server hostname against the cert, and validate the cert chain
		postgres.SSLCertVarName:     filepath.Join(clientCertVolumeMountPath, "tls.crt"),
		postgres.SSLKeyVarName:      filepath.Join(clientCertVolumeMountPath, "tls.key"),
		postgres.SSLRootCertVarName: filepath.Join(servingCertVolumeMountPath, "tls.crt"),
	}
	err = backupToolClient.Postgres().Restore(ctx.Child(), clusterCredentials, podSQLFilePath, postgres.RestoreOptions{})
	return trace.Wrap(err, "failed to restore logical backup for postgres server at %q", postgres.GetServerAddress(clusterCredentials))
}
