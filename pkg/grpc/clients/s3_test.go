package clients

import (
	"testing"
	"time"

	s3_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/s3/v1"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/utils/ptr"
)

func TestNewS3Client(t *testing.T) {
	// Create mock gRPC connection
	mockConn := &grpc.ClientConn{}

	// Call NewS3Client with mock connection
	client := NewS3Client(mockConn)

	// Assert client was created and is not nil
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Implements(t, (*s3.Runtime)(nil), client)
}

func TestEncodedS3Credentials(t *testing.T) {
	credentials := &s3.Credentials{
		AccessKeyID:      "accessKeyID",
		SecretAccessKey:  "secretAccessKey",
		SessionToken:     "sessionToken",
		Region:           "region",
		Endpoint:         "endpoint",
		S3ForcePathStyle: true,
	}

	encodedCredentials := encodedS3Credentials(credentials)
	assert.NotNil(t, encodedCredentials)
	assert.Equal(t, credentials.AccessKeyID, encodedCredentials.GetAccessKeyId())
	assert.Equal(t, credentials.SecretAccessKey, encodedCredentials.GetSecretAccessKey())
	assert.Equal(t, credentials.SessionToken, encodedCredentials.GetSessionToken())
	assert.Equal(t, credentials.Region, encodedCredentials.GetRegion())
	assert.Equal(t, credentials.Endpoint, encodedCredentials.GetEndpoint())
	assert.Equal(t, credentials.S3ForcePathStyle, encodedCredentials.GetS3ForcePathStyle())
}

func TestS3Sync(t *testing.T) {
	src := "src"
	dest := "dest"
	credentials := &s3.Credentials{
		AccessKeyID:     "accessKeyID",
		SecretAccessKey: "secretAccessKey",
	}
	asOf := time.Date(2026, time.June, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		desc         string
		asOf         time.Time
		expectedAsOf *timestamppb.Timestamp // expected as_of field on the request the client builds
		returnValues []interface{}
		errFunc      assert.ErrorAssertionFunc
	}{
		{
			desc:         "successful",
			returnValues: []interface{}{s3_v1.SyncResponse_builder{}.Build(), nil},
			errFunc:      assert.NoError,
		},
		{
			desc:         "encodes the consistency point when set",
			asOf:         asOf,
			expectedAsOf: timestamppb.New(asOf),
			returnValues: []interface{}{s3_v1.SyncResponse_builder{}.Build(), nil},
			errFunc:      assert.NoError,
		},
		{
			desc:         "failure",
			returnValues: []interface{}{nil, assert.AnError},
			errFunc:      assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			request := s3_v1.SyncRequest_builder{
				Credentials: encodedS3Credentials(credentials),
				Source:      ptr.To(src),
				Dest:        ptr.To(dest),
				AsOf:        tt.expectedAsOf,
			}.Build()

			mockClient := s3_v1.NewMockS3Client()
			mockClient.OnSync(mock.Anything, request, mock.Anything).
				Return(tt.returnValues...)

			s3c := &S3Client{client: mockClient}
			err := s3c.Sync(th.NewTestContext(), credentials, src, dest, tt.asOf)

			tt.errFunc(t, err)
			mockClient.AssertExpectations(t)
		})
	}
}
