package servers

import (
	"context"
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/files"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewFilesServer(t *testing.T) {
	server := NewFilesServer()
	assert.NotNil(t, server)
	assert.NotNil(t, server.runtime)
}

func transferTest(t *testing.T, onExpectCall func(expecter *files.MockRuntime_Expecter, ctx context.Context, src, dest string) *mock.Call,
	call func(fs *FilesServer, ctx context.Context, src, dest string) (interface{}, error)) {
	tests := []struct {
		desc        string
		returnValue error
		shouldError bool
	}{
		{
			desc: "successful",
		},
		{
			desc:        "failure",
			returnValue: assert.AnError,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			runtime := files.NewMockRuntime(t)
			server := NewFilesServer()
			server.runtime = runtime

			ctx := context.Background()
			src := "src"
			dest := "dest"
			onExpectCall(runtime.EXPECT(), ctx, src, dest).Return(tt.returnValue)

			resp, err := call(server, ctx, src, dest)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			}
		})
	}
}

func TestCopyFiles(t *testing.T) {
	onExpectCall := func(expecter *files.MockRuntime_Expecter, ctx context.Context, src, dest string) *mock.Call {
		return expecter.CopyFiles(ctx, src, dest).Call
	}

	call := func(fs *FilesServer, ctx context.Context, src, dest string) (interface{}, error) {
		req := files_v1.CopyFilesRequest_builder{
			Source: &src,
			Dest:   &dest,
		}.Build()

		return fs.CopyFiles(ctx, req)
	}

	transferTest(t, onExpectCall, call)
}

func TestSyncFiles(t *testing.T) {
	onExpectCall := func(expecter *files.MockRuntime_Expecter, ctx context.Context, src, dest string) *mock.Call {
		return expecter.SyncFiles(ctx, src, dest).Call
	}

	call := func(fs *FilesServer, ctx context.Context, src, dest string) (interface{}, error) {
		req := files_v1.SyncFilesRequest_builder{
			Source: &src,
			Dest:   &dest,
		}.Build()

		return fs.SyncFiles(ctx, req)
	}

	transferTest(t, onExpectCall, call)
}
