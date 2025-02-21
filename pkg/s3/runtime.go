package s3

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/seqsense/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	Sync(ctx *contexts.Context, credentials CredentialsInterface, src string, dest string) error
}

type LocalRuntime struct {
	newSyncManager newSyncManagerFunc
}

func NewLocalRuntime() *LocalRuntime {
	return &LocalRuntime{
		newSyncManager: func(session *session.Session, options ...s3sync.Option) syncManager {
			return s3sync.New(session, options...)
		},
	}
}
