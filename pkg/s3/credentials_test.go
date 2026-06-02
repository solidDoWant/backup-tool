package s3

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentials(t *testing.T) {
	assert.Implements(t, (*CredentialsInterface)(nil), new(Credentials))
}

func TestNewCredentials(t *testing.T) {
	accessKeyID := "accessKeyID"
	secretAccessKey := "secretAccessKey"

	credentials := NewCredentials(accessKeyID, secretAccessKey)
	assert.Equal(t, credentials.AccessKeyID, accessKeyID)
	assert.Equal(t, credentials.SecretAccessKey, secretAccessKey)
}

func TestNewCredentialsFromEnv(t *testing.T) {
	accesKeyId := "accessKeyId"
	secretAccessKey := "secretAccessKey"
	sessionToken := "sessionToken"
	endpoint := "endpoint"
	region := "region"
	s3ForcePathStyle := true

	require.NoError(t, os.Setenv("AWS_ACCESS_KEY_ID", accesKeyId))
	require.NoError(t, os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey))
	require.NoError(t, os.Setenv("AWS_SESSION_TOKEN", sessionToken))
	require.NoError(t, os.Setenv("AWS_ENDPOINT", endpoint))
	require.NoError(t, os.Setenv("AWS_REGION", region))
	require.NoError(t, os.Setenv("AWS_S3_FORCE_PATH_STYLE", fmt.Sprintf("%t", s3ForcePathStyle)))

	credentials := NewCredentialsFromEnv()

	assert.Equal(t, credentials.AccessKeyID, accesKeyId)
	assert.Equal(t, credentials.SecretAccessKey, secretAccessKey)
	assert.Equal(t, credentials.SessionToken, sessionToken)
	assert.Equal(t, credentials.Endpoint, endpoint)
	assert.Equal(t, credentials.Region, region)
	assert.Equal(t, credentials.S3ForcePathStyle, s3ForcePathStyle)
}

func TestWithAccessKeyID(t *testing.T) {
	accessKeyID := "accessKeyID"
	credentials := NewCredentials("", "").WithAccessKeyID(accessKeyID)
	assert.Equal(t, credentials.AccessKeyID, accessKeyID)
}

func TestWithSecretAccessKey(t *testing.T) {
	secretAccessKey := "secretAccessKey"
	credentials := NewCredentials("", "").WithSecretAccessKey(secretAccessKey)
	assert.Equal(t, credentials.SecretAccessKey, secretAccessKey)
}

func TestWithSessionToken(t *testing.T) {
	sessionToken := "sessionToken"
	credentials := NewCredentials("", "").WithSessionToken(sessionToken)
	assert.Equal(t, credentials.SessionToken, sessionToken)
}

func TestWithEndpoint(t *testing.T) {
	endpoint := "endpoint"
	credentials := NewCredentials("", "").WithEndpoint(endpoint)
	assert.Equal(t, credentials.Endpoint, endpoint)
}

func TestWithRegion(t *testing.T) {
	region := "region"
	credentials := NewCredentials("", "").WithRegion(region)
	assert.Equal(t, credentials.Region, region)
}

func TestWithS3ForcePathStyle(t *testing.T) {
	s3ForcePathStyle := true
	credentials := NewCredentials("", "").WithS3ForcePathStyle(s3ForcePathStyle)
	assert.Equal(t, credentials.S3ForcePathStyle, s3ForcePathStyle)
}

func TestGetAccessKeyID(t *testing.T) {
	accessKeyID := "accessKeyID"
	credentials := NewCredentials(accessKeyID, "")
	assert.Equal(t, credentials.GetAccessKeyID(), accessKeyID)
}

func TestGetSecretAccessKey(t *testing.T) {
	secretAccessKey := "secretAccessKey"
	credentials := NewCredentials("", secretAccessKey)
	assert.Equal(t, credentials.GetSecretAccessKey(), secretAccessKey)
}

func TestGetSessionToken(t *testing.T) {
	sessionToken := "sessionToken"
	credentials := NewCredentials("", "").WithSessionToken(sessionToken)
	assert.Equal(t, credentials.GetSessionToken(), sessionToken)
}

func TestGetEndpoint(t *testing.T) {
	endpoint := "endpoint"
	credentials := NewCredentials("", "").WithEndpoint(endpoint)
	assert.Equal(t, credentials.GetEndpoint(), endpoint)
}

func TestGetRegion(t *testing.T) {
	region := "region"
	credentials := NewCredentials("", "").WithRegion(region)
	assert.Equal(t, credentials.GetRegion(), region)
}

func TestGetS3ForcePathStyle(t *testing.T) {
	s3ForcePathStyle := true
	credentials := NewCredentials("", "").WithS3ForcePathStyle(s3ForcePathStyle)
	assert.Equal(t, credentials.GetS3ForcePathStyle(), s3ForcePathStyle)
}

func TestAWSConfig(t *testing.T) {
	accesKeyId := "accessKeyId"
	secretAccessKey := "secretAccessKey"
	sessionToken := "sessionToken"
	endpoint := "endpoint"
	region := "region"
	s3ForcePathStyle := true

	credentials := NewCredentials(accesKeyId, secretAccessKey).
		WithSessionToken(sessionToken).
		WithEndpoint(endpoint).
		WithRegion(region).
		WithS3ForcePathStyle(s3ForcePathStyle)

	config := credentials.AWSConfig()
	require.NotNil(t, config.Credentials)

	credValues, err := config.Credentials.Retrieve(context.Background())
	require.NoError(t, err)

	assert.Equal(t, accesKeyId, credValues.AccessKeyID)
	assert.Equal(t, secretAccessKey, credValues.SecretAccessKey)
	assert.Equal(t, sessionToken, credValues.SessionToken)

	// In the v2 SDK the endpoint and path-style settings are not part of aws.Config; they are applied as
	// s3.Options at client construction time. Only the region lives on the config.
	assert.Equal(t, region, config.Region)
}

func TestAWSConfigRegionFallback(t *testing.T) {
	// A custom endpoint with no region gets a placeholder region so SigV4 signing succeeds (S3-compatible
	// stores ignore the region).
	withEndpoint := NewCredentials("id", "secret").WithEndpoint("https://minio.example.com")
	assert.Equal(t, "us-east-1", withEndpoint.AWSConfig().Region)

	// With no endpoint and no region, the region is left empty for the SDK's normal resolution to apply.
	bare := NewCredentials("id", "secret")
	assert.Empty(t, bare.AWSConfig().Region)
}
