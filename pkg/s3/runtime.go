package s3

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	// Sync copies objects from src to dest. Exactly one of src/dest is an s3://bucket/prefix URL and the
	// other is a local directory path. asOf is the event's shared consistency point: on a download
	// (s3 -> local) a non-zero asOf captures the bucket as of that instant (point-in-time) rather than its
	// latest state, provided the bucket has versioning enabled; a zero asOf (and every upload) is a
	// latest-state sync.
	Sync(ctx *contexts.Context, credentials CredentialsInterface, src string, dest string, asOf time.Time) error
}

// s3API is the subset of the aws-sdk-go-v2 *s3.Client used by the sync engine. It exists as a seam for
// unit testing and is satisfied by *s3.Client (and by the paginators, which accept this interface).
// Objects are transferred with a single streaming GetObject/PutObject each; downloads have no size limit,
// while a single PutObject caps an uploaded object at 5 GiB (well beyond DR media/audit-log object sizes).
type s3API interface {
	GetBucketVersioning(context.Context, *s3.GetBucketVersioningInput, ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	ListObjectVersions(context.Context, *s3.ListObjectVersionsInput, ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// newS3ClientFunc builds an s3API from a config and client options. It is a field on LocalRuntime so tests
// can inject a mock client.
type newS3ClientFunc func(cfg aws.Config, optFns ...func(*s3.Options)) s3API

type LocalRuntime struct {
	newS3Client newS3ClientFunc
}

func NewLocalRuntime() *LocalRuntime {
	return &LocalRuntime{
		newS3Client: func(cfg aws.Config, optFns ...func(*s3.Options)) s3API {
			return s3.NewFromConfig(cfg, optFns...)
		},
	}
}
