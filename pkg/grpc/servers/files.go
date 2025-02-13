package servers

import (
	"context"

	"github.com/gravitational/trace/trail"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/files"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
)

// TODO figure out a way to auto generate this and protobufs
type FilesServer struct {
	runtime files.Runtime
	files_v1.UnimplementedFilesServer
}

func NewFilesServer() *FilesServer {
	return &FilesServer{
		runtime: files.NewLocalRuntime(),
	}
}

func (fs *FilesServer) CopyFiles(ctx context.Context, req *files_v1.CopyFilesRequest) (*files_v1.CopyFilesResponse, error) {
	grpcCtx := contexts.UnwrapHandlerContext(ctx)
	err := fs.runtime.CopyFiles(grpcCtx, req.GetSource(), req.GetDest())
	if err != nil {
		return nil, trail.Send(grpcCtx, err)
	}

	return &files_v1.CopyFilesResponse{}, nil
}

func (fs *FilesServer) SyncFiles(ctx context.Context, req *files_v1.SyncFilesRequest) (*files_v1.SyncFilesResponse, error) {
	grpcCtx := contexts.UnwrapHandlerContext(ctx)
	err := fs.runtime.SyncFiles(grpcCtx, req.GetSource(), req.GetDest())
	if err != nil {
		return nil, trail.Send(grpcCtx, err)
	}

	return &files_v1.SyncFilesResponse{}, nil
}
