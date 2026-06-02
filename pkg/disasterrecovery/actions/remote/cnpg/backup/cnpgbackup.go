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
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/common"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
)

type CNPGBackupOptions struct {
	CloningOpts    clonedcluster.CloneClusterOptions `yaml:"clusterCloning,omitempty"`
	CleanupTimeout helpers.MaxWaitTime               `yaml:"cleanupTimeout,omitempty"`
}

// CNPGBackupInterface is a RemoteStage action. Beyond the base RemoteAction/CleanupAction contract it
// participates in the stage's cross-resource consistency-point protocol: as a PreConsistencyPointAction
// it takes its own base backup before the shared consistency point is established, and as a
// ConsistencyPointConsumer it receives that point so its clone recovers forward to it.
type CNPGBackupInterface interface {
	remote.CleanupAction
	remote.PreConsistencyPointAction
	remote.ConsistencyPointConsumer
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

	if err := common.ValidateIssuer(ctx.Child(), vs.kubeClusterClient, vs.namespace, vs.opts.CloningOpts.Certificates.ServingCert.IssuerKind, vs.servingCertIssuerName); err != nil {
		return trace.Wrap(err, "failed to validate CNPG cluster serving cert issuer %q", vs.servingCertIssuerName)
	}

	if err := common.ValidateIssuer(ctx.Child(), vs.kubeClusterClient, vs.namespace, vs.opts.CloningOpts.Certificates.ClientCACert.IssuerKind, vs.clientCertIssuerName); err != nil {
		return trace.Wrap(err, "failed to validate CNPG cluster client CA cert issuer %q", vs.clientCertIssuerName)
	}

	if _, err := vs.kubeClusterClient.Core().GetPVC(ctx.Child(), vs.namespace, vs.drVolName); err != nil {
		return trace.Wrap(err, "failed to get DR PVC %q", vs.drVolName)
	}

	vs.isValidated = true
	return nil
}

// baseBackupState takes this cluster's base backup before the shared consistency point is established,
// and receives that point once the stage has stamped it.
type baseBackupState struct {
	validateState
	baseBackup       *apiv1.Backup
	consistencyPoint time.Time // The shared consistency point the clone should recover forward to (zero until set).
	isBaseBackedUp   bool
}

// BeforeConsistencyPoint takes the base backup that fixes this cluster's recoverable state. It runs
// before the stage establishes the event's shared consistency point, so that point lands after this
// backup completes and the clone can recover forward to it. The base backup is owned by this action and
// torn down in Cleanup; it must outlive clone creation, since the clone's recovery volume snapshots are
// owned by it. Implements remote.PreConsistencyPointAction.
func (bs *baseBackupState) BeforeConsistencyPoint(ctx *contexts.Context) (err error) {
	bs.ctxLogWith(ctx).Info("Taking base backup for CNPG backup")
	defer ctx.Log.Info("CNPG base backup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !bs.isValidated {
		return trace.Errorf("attempted to create base backup without validating")
	}

	if bs.isBaseBackedUp {
		return trace.Errorf("attempted to create base backup multiple times")
	}

	if bs.opts.CloningOpts.CleanupTimeout == 0 {
		bs.opts.CloningOpts.CleanupTimeout = bs.opts.CleanupTimeout
	}

	backup, err := bs.kubeClusterClient.CreateClusterBackup(ctx.Child(), bs.namespace, bs.clusterName, bs.opts.CloningOpts)
	if err != nil {
		return trace.Wrap(err, "failed to back up cluster %q", bs.clusterName)
	}

	bs.baseBackup = backup
	bs.isBaseBackedUp = true
	return nil
}

// SetConsistencyPoint records the shared consistency point established by the stage. Implements
// remote.ConsistencyPointConsumer.
func (bs *baseBackupState) SetConsistencyPoint(c time.Time) {
	bs.consistencyPoint = c
}

type setupStateMountPaths struct {
	drVolume    string
	servingCert string
	clientCert  string
}

type setupState struct {
	baseBackupState
	clonedCluster clonedcluster.ClonedClusterInterface
	mountPaths    setupStateMountPaths
	isSetup       bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for CNPG backup")
	defer ctx.Log.Info("CNPG backup setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isBaseBackedUp {
		return trace.Errorf("attempted to setup without creating a base backup")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	clonedClusterName := helpers.CleanName(helpers.TruncateString(fmt.Sprintf("%s-%s-cloned-%s", constants.ToolShortName, ss.uid, ss.clusterName), 40, ""))

	// Recover the clone forward to the shared consistency point. If the source had no WAL at/after it (an
	// idle database), CloneClusterFromBackup falls back to the backup's own consistency point. When no
	// point was established (the action ran outside the stage's protocol), recover straight to the backup.
	cloneOpts := ss.opts.CloningOpts
	if !ss.consistencyPoint.IsZero() {
		cloneOpts.RecoveryTargetTime = ss.consistencyPoint.Format(time.RFC3339)
	}

	clonedCluster, err := ss.kubeClusterClient.CloneClusterFromBackup(ctx.Child(), ss.namespace, ss.clusterName,
		clonedClusterName, ss.servingCertIssuerName, ss.clientCertIssuerName, ss.baseBackup, cloneOpts)
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

// Cleanup tears down whatever this action created, tolerating partial state (e.g. a base backup taken
// but the clone never created because another action failed first). The clone is deleted before the base
// backup it recovered from. The base backup is deleted only here, at the end of the event, so it outlives
// clone creation.
func (ss *setupState) Cleanup(ctx *contexts.Context) error {
	cleanupErrs := make([]error, 0, 2)

	if ss.clonedCluster != nil {
		if err := cleanup.To(ss.clonedCluster.Delete).
			WithErrMessage("failed to cleanup cloned cluster resources").
			WithParentCtx(ctx).WithTimeout(ss.opts.CleanupTimeout.MaxWait(10 * time.Minute)).
			RunError(); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}

	if ss.baseBackup != nil {
		if err := cleanup.To(func(ctx *contexts.Context) error {
			return ss.kubeClusterClient.CNPG().DeleteBackup(ctx, ss.namespace, ss.baseBackup.Name)
		}).WithErrMessage("failed to cleanup base backup %q", helpers.FullNameStr(ss.namespace, ss.baseBackup.Name)).
			WithParentCtx(ctx).WithTimeout(ss.opts.CleanupTimeout.MaxWait(time.Minute)).
			RunError(); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed to cleanup CNPG backup resources")
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
