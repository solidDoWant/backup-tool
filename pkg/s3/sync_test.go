package s3

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/seqsense/s3sync"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

func TestSync(t *testing.T) {
	tests := []struct {
		desc           string
		shouldSyncFail bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:           "fails when underlying sync fails",
			shouldSyncFail: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx := th.NewTestContext()
			creds := NewMockCredentialsInterface(t)
			src := "source"
			dest := "destination"
			runtime := NewLocalRuntime()

			creds.EXPECT().AWSConfig().Return(&aws.Config{})

			runtime.newSyncManager = func(session *session.Session, options ...s3sync.Option) syncManager {
				syncManager := NewMocksyncManager(t)
				syncManager.EXPECT().SyncWithContext(mock.Anything, src, dest).Return(th.ErrIfTrue(test.shouldSyncFail))
				return syncManager
			}

			err := runtime.Sync(ctx, creds, src, dest)
			if test.shouldSyncFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
