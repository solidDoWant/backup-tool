package backup

import (
	"fmt"
	"path/filepath"
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
)

type CNPGBackupOptions struct {
	CloningOpts    clonedcluster.CloneClusterOptions `yaml:"clusterCloning,omitempty"`
	CleanupTimeout helpers.MaxWaitTime               `yaml:"cleanupTimeout,omitempty"`
}

type CNPGBackupInterface interface {
	remote.CleanupAction
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, clusterName, servingCertIssuerName, clientCertIssuerName, drVolName, backupFileRelPath string, opts CNPGBackupOptions) error
}

type configureState struct {
	uid                   string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured          bool
	kubeClusterClient     kubecluster.ClientInterface
	clusterName           string
	servingCertIssuerName string
	clientCertIssuerName  string
	namespace             string
	drVolName             string
	backupFileRelPath     string
	opts                  CNPGBackupOptions
}

func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, clusterName, servingCertIssuerName, clientCertIssuerName, drVolName, backupFileRelPath string, opts CNPGBackupOptions) error {
	if cs.isConfigured {
		return trace.Errorf("attempted to configure multiple times")
	}

	cs.uid = uuid.NewString()
	cs.kubeClusterClient = kubeClusterClient
	cs.namespace = namespace
	cs.clusterName = clusterName
	cs.servingCertIssuerName = servingCertIssuerName
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
	isValidated bool
	cluster     *apiv1.Cluster
}

func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for CNPG backup")
	defer ctx.Log.Info("Completed CNPG backup configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

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

	servingCertIssuer, err := vs.kubeClusterClient.CM().GetIssuer(ctx.Child(), vs.namespace, vs.servingCertIssuerName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster serving cert issuer %q", vs.clusterName)
	}
	if !certmanager.IsIssuerReady(servingCertIssuer) {
		return trace.Errorf("CNPG cluster serving cert issuer %q is not ready", vs.servingCertIssuerName)
	}

	clientCertIssuer, err := vs.kubeClusterClient.CM().GetIssuer(ctx.Child(), vs.namespace, vs.clientCertIssuerName)
	if err != nil {
		return trace.Wrap(err, "failed to get CNPG cluster client cert issuer %q", vs.clientCertIssuerName)
	}
	if !certmanager.IsIssuerReady(clientCertIssuer) {
		return trace.Errorf("CNPG cluster client cert issuer %q is not ready", vs.clientCertIssuerName)
	}

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
	clonedCluster clonedcluster.ClonedClusterInterface
	mountPaths    setupStateMountPaths
	isSetup       bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for CNPG backup")
	defer ctx.Log.Info("CNPG backup setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isValidated {
		return trace.Errorf("attempted to setup without validating")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	clonedClusterName := helpers.CleanName(helpers.TruncateString(fmt.Sprintf("%s-%s-cloned-%s", constants.ToolName, ss.uid, ss.clusterName), 50, ""))
	if ss.opts.CloningOpts.CleanupTimeout == 0 {
		ss.opts.CloningOpts.CleanupTimeout = ss.opts.CleanupTimeout
	}

	clonedCluster, err := ss.kubeClusterClient.CloneCluster(ctx.Child(), ss.namespace, ss.clusterName,
		clonedClusterName, ss.servingCertIssuerName, ss.clientCertIssuerName, ss.opts.CloningOpts)
	if err != nil {
		return trace.Wrap(err, "failed to clone cluster")
	}
	ss.clonedCluster = clonedCluster

	baseMountPath := filepath.Join("/mnt", "cnpgbackup", ss.clusterName, ss.uid)
	secretsVolumeMountPath := filepath.Join(baseMountPath, "secrets")

	ss.mountPaths = setupStateMountPaths{
		drVolume:    filepath.Join(baseMountPath, "dr"),
		servingCert: filepath.Join(secretsVolumeMountPath, "serving-cert"),
		clientCert:  filepath.Join(secretsVolumeMountPath, "client-cert"),
	}

	btiOpts.Volumes = append(btiOpts.Volumes,
		core.NewSingleContainerPVC(ss.drVolName, ss.mountPaths.drVolume),
		core.NewSingleContainerSecret(clonedCluster.GetServingCert().Spec.SecretName, ss.mountPaths.servingCert, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
		core.NewSingleContainerSecret(clonedCluster.GetPostgresUserCert().GetCertificate().Spec.SecretName, ss.mountPaths.clientCert),
	)

	ss.isSetup = true
	return nil
}

func (ss *setupState) Cleanup(ctx *contexts.Context) error {
	if !ss.isSetup {
		return nil
	}

	err := cleanup.To(ss.clonedCluster.Delete).
		WithErrMessage("failed to cleanup cloned cluster resources").
		WithParentCtx(ctx).WithTimeout(ss.opts.CleanupTimeout.MaxWait(10 * time.Minute)).
		Run()
	return trace.Wrap(err, "failed to cleanup CNPG backup resources")
}

type executeState struct {
	setupState
}

func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing CNPG backup")
	defer ctx.Log.Info("CNPG backup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	podSQLFilePath := filepath.Join(es.mountPaths.drVolume, es.backupFileRelPath)
	credentials := es.clonedCluster.GetCredentials(es.mountPaths.servingCert, es.mountPaths.clientCert)
	err = backupToolClient.Postgres().DumpAll(ctx.Child(), credentials, podSQLFilePath, postgres.DumpAllOptions{CleanupTimeout: es.opts.CleanupTimeout})
	return trace.Wrap(err, "failed to create logical backup for postgres server at %q", postgres.GetServerAddress(credentials))
}

type CNPGBackup struct {
	executeState
}

func NewCNPGBackup() CNPGBackupInterface {
	return &CNPGBackup{}
}
