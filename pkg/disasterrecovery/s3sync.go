package disasterrecovery

import (
	"fmt"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/s3"
)

// Synces files to or from S3. Fields are for state tracking. Callers should:
// 1. Populate the struct with `Configure`
// 2. Sync the files with `Sync`
type S3SyncInterface interface {
	Configure(kubeClusterClient kubecluster.ClientInterface, namespace, drVolName, dirName, s3Path, eventName string, credentials s3.CredentialsInterface, opts s3SyncOpts)
	Sync(ctx *contexts.Context) error
}

type s3SyncOpts struct {
	RemoteBackupToolOptions     backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	CleanupTimeout              helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
	ClusterServiceSearchDomains []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
}

type S3Sync struct {
	kubeClusterClient kubecluster.ClientInterface
	namespace         string
	drVolName         string
	dirName           string
	s3Path            string
	credentials       s3.CredentialsInterface
	eventName         string
	opts              s3SyncOpts
}

func NewS3Sync() S3SyncInterface {
	return &S3Sync{}
}

func (s3s *S3Sync) Configure(kubeClusterClient kubecluster.ClientInterface, namespace, drVolName, dirName, s3Path, eventName string, credentials s3.CredentialsInterface, opts s3SyncOpts) {
	s3s.kubeClusterClient = kubeClusterClient
	s3s.namespace = namespace
	s3s.drVolName = drVolName
	s3s.dirName = dirName
	s3s.s3Path = s3Path
	s3s.credentials = credentials
	s3s.eventName = eventName
	s3s.opts = opts
}

func (s3s *S3Sync) Sync(ctx *contexts.Context) error {
	ctx.Log.Step().Info("Creating backup tool instance")
	drVolumeMountPath := filepath.Join(teleportBaseMountPath, "dr")
	btOpts := backuptoolinstance.CreateBackupToolInstanceOptions{
		NamePrefix: fmt.Sprintf("%s-%s-%s", constants.ToolName, s3s.eventName, "s3sync"),
		Volumes: []core.SingleContainerVolume{
			core.NewSingleContainerPVC(s3s.drVolName, drVolumeMountPath),
		},
		CleanupTimeout: s3s.opts.CleanupTimeout,
	}
	mergo.MergeWithOverwrite(&btOpts, s3s.opts.RemoteBackupToolOptions)
	btInstance, err := s3s.kubeClusterClient.CreateBackupToolInstance(ctx.Child(), s3s.namespace, s3s.eventName, btOpts)
	if err != nil {
		return trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", s3s.eventName).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(s3s.opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child(), s3s.opts.ClusterServiceSearchDomains...)
	if err != nil {
		return trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	ctx.Log.Step().Info("Syncing files")
	auditSessionLogsPath := filepath.Join(drVolumeMountPath, s3s.dirName)

	err = backupToolClient.S3().Sync(ctx.Child(), s3s.credentials, s3s.s3Path, auditSessionLogsPath)
	return trace.Wrap(err, "failed to sync files with %q", s3s.s3Path)
}
