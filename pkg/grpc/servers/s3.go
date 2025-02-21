package servers

import (
	"context"

	"github.com/gravitational/trace/trail"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	s3_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/s3/v1"
	"github.com/solidDoWant/backup-tool/pkg/s3"
)

type S3Server struct {
	s3_v1.UnimplementedS3Server
	runtime s3.Runtime
}

func NewS3Server() *S3Server {
	return &S3Server{
		runtime: s3.NewLocalRuntime(),
	}
}

func decodeS3Credentials(encodedCredentials *s3_v1.Credentials) *s3.Credentials {
	return s3.NewCredentials(encodedCredentials.GetAccessKeyId(), encodedCredentials.GetSecretAccessKey()).
		WithSessionToken(encodedCredentials.GetSessionToken()).
		WithRegion(encodedCredentials.GetRegion()).
		WithEndpoint(encodedCredentials.GetEndpoint())
}

func (s3s *S3Server) Sync(ctx context.Context, req *s3_v1.SyncRequest) (*s3_v1.SyncResponse, error) {
	grpcCtx := contexts.UnwrapHandlerContext(ctx)
	err := s3s.runtime.Sync(grpcCtx, decodeS3Credentials(req.GetCredentials()), req.GetSource(), req.GetDest())
	if err != nil {
		return nil, trail.Send(grpcCtx, err)
	}

	return &s3_v1.SyncResponse{}, nil
}
