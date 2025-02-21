package s3

import (
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

	os.Setenv("AWS_ACCESS_KEY_ID", accesKeyId)
	os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey)
	os.Setenv("AWS_SESSION_TOKEN", sessionToken)
	os.Setenv("AWS_ENDPOINT", endpoint)
	os.Setenv("AWS_REGION", region)

	credentials := NewCredentialsFromEnv()

	assert.Equal(t, credentials.AccessKeyID, accesKeyId)
	assert.Equal(t, credentials.SecretAccessKey, secretAccessKey)
	assert.Equal(t, credentials.SessionToken, sessionToken)
	assert.Equal(t, credentials.Endpoint, endpoint)
	assert.Equal(t, credentials.Region, region)
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

func TestAWSConfig(t *testing.T) {
	accesKeyId := "accessKeyId"
	secretAccessKey := "secretAccessKey"
	sessionToken := "sessionToken"
	endpoint := "endpoint"
	region := "region"

	credentials := NewCredentials(accesKeyId, secretAccessKey).
		WithSessionToken(sessionToken).
		WithEndpoint(endpoint).
		WithRegion(region)

	config := credentials.AWSConfig()
	require.NotNil(t, config)
	require.NotNil(t, config.Credentials)

	credValues, err := config.Credentials.Get()
	require.NoError(t, err)

	assert.Equal(t, credValues.AccessKeyID, accesKeyId)
	assert.Equal(t, credValues.SecretAccessKey, secretAccessKey)
	assert.Equal(t, credValues.SessionToken, sessionToken)

	require.NotNil(t, config.Endpoint)
	require.NotNil(t, config.Region)

	assert.Equal(t, *config.Endpoint, endpoint)
	assert.Equal(t, *config.Region, region)
}
