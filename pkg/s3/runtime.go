package s3

import (
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/seqsense/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	// Sync copies objects from src to dest. asOf is the event's shared consistency point: when non-zero,
	// the source bucket is captured as of that instant rather than its latest state; a zero asOf means a
	// latest-state sync.
	Sync(ctx *contexts.Context, credentials CredentialsInterface, src string, dest string, asOf time.Time) error
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
