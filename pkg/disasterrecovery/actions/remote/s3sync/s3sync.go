package s3sync

import (
	"path/filepath"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/s3"
)

type Direction int

func (d Direction) String() string {
	switch d {
	case DirectionDownload:
		return "download"
	case DirectionUpload:
		return "upload"
	default:
		return "unknown"
	}
}

const (
	DirectionDownload Direction = iota
	DirectionUpload
)

type S3SyncOptions struct{}

type S3SyncInterface interface {
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, drVolName, backupDirRelPath, s3Path string, credentials s3.CredentialsInterface, direction Direction, opts S3SyncOptions) error
	Validate(ctx *contexts.Context) error
	Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) error
	Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) error
}

type configureState struct {
	uid               string // Unique identifier to prevent accidental collisions between multiple instances
	isConfigured      bool
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	drVolName         string
	backupDirRelPath  string
	s3Path            string
	credentials       s3.CredentialsInterface
	direction         Direction
	opts              S3SyncOptions
}

func (cs *configureState) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, drVolName, backupDirRelPath, s3Path string, credentials s3.CredentialsInterface, direction Direction, opts S3SyncOptions) error {
	if cs.isConfigured {
		return trace.Errorf("attempted to configure multiple times")
	}

	cs.uid = uuid.NewString()
	cs.kubeClusterClient = kubeClusterClient
	cs.namespace = namespace
	cs.drVolName = drVolName
	cs.backupDirRelPath = backupDirRelPath
	cs.s3Path = s3Path
	cs.credentials = credentials
	cs.direction = direction
	cs.opts = opts

	cs.isConfigured = true
	return nil
}

func (cs *configureState) ctxLogWith(ctx *contexts.Context) *contexts.LoggerContext {
	return ctx.Log.With("s3Path", cs.s3Path, "uid", cs.uid, "direction", cs.direction.String())
}

type setupStateMountPaths struct {
	drVolume string
}

type validateState struct {
	configureState
	isValidated bool
}

func (vs *validateState) Validate(ctx *contexts.Context) (err error) {
	vs.ctxLogWith(ctx).Info("Validating configuration for S3 sync")
	defer ctx.Log.Info("Completed S3 sync configuration validation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !vs.isConfigured {
		return trace.Errorf("attempted to validate without configuring")
	}

	if vs.direction != DirectionUpload && vs.direction != DirectionDownload {
		return trace.Errorf("invalid direction %q", vs.direction)
	}

	vs.isValidated = true
	return nil
}

type setupState struct {
	validateState
	mountPaths setupStateMountPaths
	isSetup    bool
}

func (ss *setupState) Setup(ctx *contexts.Context, btiOpts *backuptoolinstance.CreateBackupToolInstanceOptions) (err error) {
	ss.ctxLogWith(ctx).Info("Setting up for CNPG restore")
	defer ctx.Log.Info("CNPG restore setup complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !ss.isValidated {
		return trace.Errorf("attempted to setup without validating")
	}

	if ss.isSetup {
		return trace.Errorf("attempted to setup multiple times")
	}

	baseMountPath := filepath.Join("/mnt", "s3sync", ss.uid)
	ss.mountPaths = setupStateMountPaths{
		drVolume: filepath.Join(baseMountPath, "dr"),
	}

	btiOpts.Volumes = append(btiOpts.Volumes, core.NewSingleContainerPVC(ss.drVolName, ss.mountPaths.drVolume))

	ss.isSetup = true
	return nil
}

type executeState struct {
	setupState
}

func (es *executeState) Execute(ctx *contexts.Context, backupToolClient clients.ClientInterface) (err error) {
	es.ctxLogWith(ctx).Info("Executing S3 sync")
	defer ctx.Log.Info("S3 sync complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if !es.isSetup {
		return trace.Errorf("attempted to execute without setting up")
	}

	backupPath := filepath.Join(es.mountPaths.drVolume, es.backupDirRelPath)

	source := es.s3Path
	destination := backupPath
	if es.direction == DirectionUpload {
		source, destination = destination, source
	}

	err = backupToolClient.S3().Sync(ctx.Child(), es.credentials, source, destination)
	return trace.Wrap(err, "failed to sync files", "source", source, "destination", destination)
}

type S3Sync struct {
	executeState
}

func NewS3Sync() S3SyncInterface {
	return &S3Sync{}
}
