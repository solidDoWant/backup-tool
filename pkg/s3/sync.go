package s3

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gravitational/trace"
	"github.com/seqsense/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

type syncManager interface {
	SyncWithContext(ctx context.Context, src string, dest string) error
}

type newSyncManagerFunc func(session *session.Session, options ...s3sync.Option) syncManager

func (lr *LocalRuntime) Sync(ctx *contexts.Context, credentials CredentialsInterface, src string, dest string) (err error) {
	ctx.Log.With("src", src, "dest", dest).Info("Copying files")
	defer ctx.Log.Info("Finished copying files", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	session, err := session.NewSession(credentials.AWSConfig())
	if err != nil {
		return trace.Wrap(err, "failed to s3 create session")
	}

	// This library will eventually need to be replaced if it does not switch to the v2 AWS SDK.
	err = lr.newSyncManager(session).SyncWithContext(ctx.Child(), src, dest)
	return trace.Wrap(err, "sync failed")
}
