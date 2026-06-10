package clients

import (
	"github.com/gravitational/trace/trail"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/files"
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

// filePatternsToProto maps the local file filter patterns onto their wire representation.
func filePatternsToProto(patterns []files.FilePattern) []*files_v1.FilePattern {
	if len(patterns) == 0 {
		return nil
	}

	protoPatterns := make([]*files_v1.FilePattern, len(patterns))
	for i, pattern := range patterns {
		glob := pattern.Glob
		protoPatterns[i] = files_v1.FilePattern_builder{Glob: &glob}.Build()
	}
	return protoPatterns
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

func (fc *FilesClient) SyncFiles(ctx *contexts.Context, src, dest string, opts files.SyncFilesOptions) error {
	ctx.Log.With("src", src, "dest", dest).Info("Syncing files")
	defer ctx.Log.Info("Finished syncing files", ctx.Stopwatch.Keyval())

	request := files_v1.SyncFilesRequest_builder{
		Source:  &src,
		Dest:    &dest,
		Include: filePatternsToProto(opts.Filter.Include),
		Exclude: filePatternsToProto(opts.Filter.Exclude),
	}.Build()

	var header metadata.MD
	_, err := fc.client.SyncFiles(ctx.Child(), request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}

func (fc *FilesClient) ListDirectory(ctx *contexts.Context, path string) ([]string, error) {
	ctx.Log.With("path", path).Info("Listing directory")
	defer ctx.Log.Info("Finished listing directory", ctx.Stopwatch.Keyval())

	request := files_v1.ListDirectoryRequest_builder{
		Path: &path,
	}.Build()

	var header metadata.MD
	response, err := fc.client.ListDirectory(ctx.Child(), request, grpc.Header(&header))
	if err != nil {
		return nil, trail.FromGRPC(err, header)
	}

	return response.GetEntries(), nil
}
