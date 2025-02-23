package cnpgrestore

import (
	"path/filepath"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
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

type CNPGRestoreOptionsCert struct {
	Subject            *certmanagerv1.X509Subject                `yaml:"subject,omitempty"`
	CRPOpts            clusterusercert.NewClusterUserCertOptsCRP `yaml:"certificateRequestPolicy,omitempty"`
	WaitForCertTimeout helpers.MaxWaitTime                       `yaml:"waitForCertTimeout,omitempty"`
}

type CNPGRestoreOptions struct {
	PostgresUserCert CNPGRestoreOptionsCert `yaml:"postgresUserCert,omitempty"`
	CleanupTimeout   helpers.MaxWaitTime    `yaml:"cleanupTimeout,omitempty"`
}

// Performs a CNPG logical recovery. Fields are for state tracking. Callers should:
// 1. Populate the struct with `Configure`
// 2. Validate that the required resources are ready with `CheckResourcesReady`
// 3. Mutate a backup tool pod to be able to perform the restore with `PrepareBackupToolPod`
// 3. Restore the backup with `Restore`
// Separating this out into multiple states/steps/stages allows callers that need to
// restore multiple clusters to perform each step for all clusters before moving on
// to the next one.
type CNPGRestoreInterface interface {
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, clusterName, servingCertName, clientCertIssuerName, drVolName, backupFileRelPath string, opts CNPGRestoreOptions) error
	Validate(ctx *contexts.Context) error
	Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) error
	Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) error
	Cleanup(ctx *contexts.Context) error
}

type configureState struct {
	uid                  string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured         bool
	kubeClusterClient    kubecluster.ClientInterface
	namespace            string
	drVolName            string
	backupFileRelPath    string
	clusterName          string
	servingCertName      string
	clientCertIssuerName string
	opts                 CNPGRestoreOptions
}

// Configures the action prior to validation and execution. This should be called before
// any other methods. Returns an error if the action is already configured.
func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, clusterName, servingCertName, clientCertIssuerName, drVolName, backupFileRelPath string, opts CNPGRestoreOptions) error {
	if cs.isConfigured {
		return trace.Errorf("attempted to configure multiple times")
	}

	cs.uid = uuid.NewString()
	cs.kubeClusterClient = kubeClusterClient
	cs.namespace = namespace
	cs.clusterName = clusterName
	cs.servingCertName = servingCertName
	cs.clientCertIssuerName = clientCertIssuerName
	cs.drVolName = drVolName
	cs.backupFileRelPath = backupFileRelPath
	cs.opts = opts

	cs.isConfigured = true
	return nil
}

func (cs *configureState) ctxLogWith(ctx *contexts.Context) *contexts.LoggerContext {
	return ctx.Log.With("clusterName", cs.clusterName, "uid", cs.uid)
}

type validateState struct {
	configureState
	isValidated      bool
	cluster          *apiv1.Cluster
	servingCert      *certmanagerv1.Certificate
	clientCertIssuer *certmanagerv1.Issuer
}

// Validates that the required resources are ready. This should be called after `Configure`
// and before `Setup`. Returns an error if the resources are not ready.
func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for CNPG restore")
	defer ctx.Log.Info("Completed CNPG restore configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !vs.isConfigured {
		return trace.Errorf("attempted to validate without configuring")
	}

	cluster, err := vs.kubeClusterClient.CNPG().GetCluster(ctx.Child(), vs.namespace, vs.clusterName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster %q", vs.clusterName)
	}
	if !cnpg.IsClusterReady(cluster) {
		return trace.Errorf("CNPG cluster %q is not ready", vs.clusterName)
	}
	vs.cluster = cluster

	servingCert, err := vs.kubeClusterClient.CM().GetCertificate(ctx.Child(), vs.namespace, vs.servingCertName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster serving cert %q", vs.clusterName)
	}
	vs.servingCert = servingCert

	clientCertIssuer, err := vs.kubeClusterClient.CM().GetIssuer(ctx.Child(), vs.namespace, vs.clientCertIssuerName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster client cert issuer %q", vs.clientCertIssuerName)
	}
	if !certmanager.IsIssuerReady(clientCertIssuer) {
		return trace.Errorf("CNPG cluster client cert issuer %q is not ready", vs.clientCertIssuerName)
	}
	vs.clientCertIssuer = clientCertIssuer

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.drVolName); err != nil {
		return trace.Wrap(err, "failed to get DR PVC %q", vs.drVolName)
	}

	vs.isValidated = true
	return nil
}

type setupStateMountPaths struct {
	drVolume    string
	servingCert string
	clientCert  string
}

type setupState struct {
	validateState
	postgresUserCert clusterusercert.ClusterUserCertInterface
	mountPaths       setupStateMountPaths
	isSetup          bool
}

// Prepares the backup tool pod to be able to perform the restore. This should be called
// after `Validate` and before `Execute`. Returns an error if the pod cannot be prepared.
func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for CNPG restore")
	defer ctx.Log.Info("CNPG restore setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isValidated {
		return trace.Errorf("attempted to setup without validating")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	cucOptions := clusterusercert.NewClusterUserCertOpts{
		Subject:            ss.opts.PostgresUserCert.Subject,
		CRPOpts:            ss.opts.PostgresUserCert.CRPOpts,
		WaitForCertTimeout: ss.opts.PostgresUserCert.WaitForCertTimeout,
		CleanupTimeout:     ss.opts.CleanupTimeout,
	}
	postgresUserCert, err := ss.kubeClusterClient.NewClusterUserCert(ctx.Child(), ss.namespace, "postgres", ss.clientCertIssuerName, ss.clusterName, cucOptions)
	if err != nil {
		return trace.Wrap(err, "failed to create postgres user CNPG cluster client cert")
	}
	ss.postgresUserCert = postgresUserCert

	baseMountPath := filepath.Join("/mnt", "cnpgrestore", ss.clusterName, ss.uid)
	secretsVolumeMountPath := filepath.Join(baseMountPath, "secrets")

	ss.mountPaths = setupStateMountPaths{
		drVolume:    filepath.Join(baseMountPath, "dr"),
		servingCert: filepath.Join(secretsVolumeMountPath, "serving-cert"),
		clientCert:  filepath.Join(secretsVolumeMountPath, "client-cert"),
	}

	btiOpts.Volumes = append(btiOpts.Volumes,
		core.NewSingleContainerPVC(ss.drVolName, ss.mountPaths.drVolume),
		core.NewSingleContainerSecret(ss.servingCert.Spec.SecretName, ss.mountPaths.servingCert, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
		core.NewSingleContainerSecret(postgresUserCert.GetCertificate().Spec.SecretName, ss.mountPaths.clientCert),
	)

	ss.isSetup = true
	return nil
}

func (ss *setupState) Cleanup(ctx *contexts.Context) error {
	if !ss.isSetup {
		return nil
	}

	err := cleanup.To(ss.postgresUserCert.Delete).
		WithErrMessage("failed to cleanup postgres user CNPG cluster postgres user client cert resources").
		WithParentCtx(ctx).WithTimeout(ss.opts.CleanupTimeout.MaxWait(time.Minute)).
		Run()
	return trace.Wrap(err, "failed to cleanup CNPG restore resources")
}

type executeState struct {
	setupState
}

func (es *executeState) clusterCredentials() postgres.Credentials {
	return &postgres.EnvironmentCredentials{
		postgres.HostVarName:        es.cluster.Status.WriteService + "." + es.namespace + ".svc",
		postgres.UserVarName:        "postgres",
		postgres.RequireAuthVarName: "none",
		postgres.SSLModeVarName:     "verify-full",
		postgres.SSLCertVarName:     filepath.Join(es.mountPaths.clientCert, "tls.crt"),
		postgres.SSLKeyVarName:      filepath.Join(es.mountPaths.clientCert, "tls.key"),
		postgres.SSLRootCertVarName: filepath.Join(es.mountPaths.servingCert, "tls.crt"),
	}
}

// Restores the backup. This should be called after `Setup`. Returns an error if the restore fails.
func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing CNPG restoration")
	defer ctx.Log.Info("CNPG restore complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	podSQLFilePath := filepath.Join(es.mountPaths.drVolume, es.backupFileRelPath)
	credentials := es.clusterCredentials()
	err = backupToolClient.Postgres().Restore(ctx.Child(), credentials, podSQLFilePath, postgres.RestoreOptions{})
	return trace.Wrap(err, "failed to restore logical backup for postgres server at %q", postgres.GetServerAddress(credentials))
}

type CNPGRestore struct {
	executeState
}

func NewCNPGRestore() CNPGRestoreInterface {
	return &CNPGRestore{}
}
