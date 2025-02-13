package clients

import (
	"github.com/gravitational/trace/trail"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type FilesClient struct {
	client files_v1.FilesClient
}

func NewFilesClient(grpcConnection grpc.ClientConnInterface) *FilesClient {
	return &FilesClient{
		client: files_v1.NewFilesClient(grpcConnection),
	}
}

func (fc *FilesClient) CopyFiles(ctx *contexts.Context, src, dest string) error {
	ctx.Log.With("src", src, "dest", dest).Info("Copying files")
	defer ctx.Log.Info("Finished copying files", ctx.Stopwatch.Keyval())

	request := files_v1.CopyFilesRequest_builder{
		Source: &src,
		Dest:   &dest,
	}.Build()

	var header metadata.MD
	_, err := fc.client.CopyFiles(ctx.Child(), request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}

func (fc *FilesClient) SyncFiles(ctx *contexts.Context, src, dest string) error {
	ctx.Log.With("src", src, "dest", dest).Info("Syncing files")
	defer ctx.Log.Info("Finished syncing files", ctx.Stopwatch.Keyval())

	request := files_v1.SyncFilesRequest_builder{
		Source: &src,
		Dest:   &dest,
	}.Build()

	var header metadata.MD
	_, err := fc.client.SyncFiles(ctx.Child(), request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}
