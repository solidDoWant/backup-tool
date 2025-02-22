package servers

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	s3_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/s3/v1"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestNewS3Server(t *testing.T) {
	server := NewS3Server()

	assert.NotNil(t, server)
	assert.NotNil(t, server.runtime)
}

func TestDecodeS3Credentials(t *testing.T) {
	credentials := s3_v1.Credentials_builder{
		AccessKeyId:      ptr.To("accessKeyID"),
		SecretAccessKey:  ptr.To("secretAccessKey"),
		SessionToken:     ptr.To("sessionToken"),
		Region:           ptr.To("region"),
		Endpoint:         ptr.To("endpoint"),
		S3ForcePathStyle: ptr.To(true),
	}.Build()

	decodedCredentials := decodeS3Credentials(credentials)
	require.NotNil(t, decodedCredentials)
	assert.Equal(t, credentials.GetAccessKeyId(), decodedCredentials.AccessKeyID)
	assert.Equal(t, credentials.GetSecretAccessKey(), decodedCredentials.SecretAccessKey)
	assert.Equal(t, credentials.GetSessionToken(), decodedCredentials.SessionToken)
	assert.Equal(t, credentials.GetRegion(), decodedCredentials.Region)
	assert.Equal(t, credentials.GetEndpoint(), decodedCredentials.Endpoint)
	assert.Equal(t, credentials.GetS3ForcePathStyle(), decodedCredentials.S3ForcePathStyle)
}

func TestS3Sync(t *testing.T) {
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
			runtime := s3.NewMockRuntime(t)
			server := NewS3Server()
			server.runtime = runtime

			ctx := th.NewTestContext()
			src := "src"
			dest := "dest"
			credentials := s3_v1.Credentials_builder{
				AccessKeyId:     ptr.To("accessKeyID"),
				SecretAccessKey: ptr.To("secretAccessKey"),
			}.Build()

			runtime.EXPECT().Sync(contexts.UnwrapHandlerContext(ctx), decodeS3Credentials(credentials), src, dest).Return(tt.returnValue)

			resp, err := server.Sync(ctx, s3_v1.SyncRequest_builder{
				Credentials: credentials,
				Source:      &src,
				Dest:        &dest,
			}.Build())
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
