package clients

import (
	"time"

	"github.com/gravitational/trace/trail"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	s3_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/s3/v1"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/utils/ptr"
)

type S3Client struct {
	client s3_v1.S3Client
}

func NewS3Client(grpcConnection grpc.ClientConnInterface) *S3Client {
	return &S3Client{
		client: s3_v1.NewS3Client(grpcConnection),
	}
}

func encodedS3Credentials(credentials s3.CredentialsInterface) *s3_v1.Credentials {
	return s3_v1.Credentials_builder{
		AccessKeyId:      ptr.To(credentials.GetAccessKeyID()),
		SecretAccessKey:  ptr.To(credentials.GetSecretAccessKey()),
		SessionToken:     ptr.To(credentials.GetSessionToken()),
		Region:           ptr.To(credentials.GetRegion()),
		Endpoint:         ptr.To(credentials.GetEndpoint()),
		S3ForcePathStyle: ptr.To(credentials.GetS3ForcePathStyle()),
	}.Build()
}

func (s3c *S3Client) Sync(ctx *contexts.Context, credentials s3.CredentialsInterface, src, dest string, asOf time.Time) error {
	ctx.Log.With("src", src, "dest", dest).Info("Syncing files")
	defer ctx.Log.Info("Finished syncing files", ctx.Stopwatch.Keyval())

	// Carry the consistency point only when set; a zero asOf leaves the field unset, which the handler
	// reads as "no consistency point" -> latest-state sync.
	var asOfTimestamp *timestamppb.Timestamp
	if !asOf.IsZero() {
		asOfTimestamp = timestamppb.New(asOf)
	}

	request := s3_v1.SyncRequest_builder{
		Credentials: encodedS3Credentials(credentials),
		Source:      &src,
		Dest:        &dest,
		AsOf:        asOfTimestamp,
	}.Build()

	var header metadata.MD
	_, err := s3c.client.Sync(ctx.Child(), request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}
