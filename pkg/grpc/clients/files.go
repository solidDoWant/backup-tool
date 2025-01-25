package clients

import (
	"context"

	"github.com/gravitational/trace/trail"
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

func (fc *FilesClient) CopyFiles(ctx context.Context, src, dest string) error {
	request := files_v1.CopyFilesRequest_builder{
		Source: &src,
		Dest:   &dest,
	}.Build()

	var header metadata.MD
	_, err := fc.client.CopyFiles(ctx, request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}

func (fc *FilesClient) SyncFiles(ctx context.Context, src, dest string) error {
	request := files_v1.SyncFilesRequest_builder{
		Source: &src,
		Dest:   &dest,
	}.Build()

	var header metadata.MD
	_, err := fc.client.SyncFiles(ctx, request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}
