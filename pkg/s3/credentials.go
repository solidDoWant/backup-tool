package s3

import (
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

type CredentialsInterface interface {
	GetAccessKeyID() string
	GetSecretAccessKey() string
	GetSessionToken() string
	GetRegion() string
	GetEndpoint() string
	GetS3ForcePathStyle() bool
	// AWSConfig returns a v2 SDK config carrying the static credentials and region. The endpoint and
	// path-style settings are not part of aws.Config in the v2 SDK; they are applied as s3.Options when
	// the client is constructed (see LocalRuntime.Sync).
	AWSConfig() aws.Config
}

type Credentials struct {
	AccessKeyID      string `yaml:"accessKeyId" jsonschema:"required"`
	SecretAccessKey  string `yaml:"secretAccessKey" jsonschema:"required"`
	SessionToken     string `yaml:"sessionToken,omitempty"`
	Endpoint         string `yaml:"endpoint,omitempty"`
	Region           string `yaml:"region,omitempty"`
	S3ForcePathStyle bool   `yaml:"s3ForcePathStyle,omitempty"`
}

func NewCredentials(accessKeyID, secretAccessKey string) *Credentials {
	return &Credentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}
}

// Create new credentials from the standard AWS environment variables.
func NewCredentialsFromEnv() *Credentials {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	return NewCredentials(accessKeyID, secretAccessKey).
		WithSessionToken(os.Getenv("AWS_SESSION_TOKEN")).
		WithEndpoint(os.Getenv("AWS_ENDPOINT")).
		WithRegion(os.Getenv("AWS_REGION")).
		WithS3ForcePathStyle(os.Getenv("AWS_S3_FORCE_PATH_STYLE") == "true")
}

func (c *Credentials) WithAccessKeyID(id string) *Credentials {
	c.AccessKeyID = id
	return c
}

func (c *Credentials) WithSecretAccessKey(key string) *Credentials {
	c.SecretAccessKey = key
	return c
}

func (c *Credentials) WithSessionToken(token string) *Credentials {
	c.SessionToken = token
	return c
}

func (c *Credentials) WithEndpoint(endpoint string) *Credentials {
	c.Endpoint = endpoint
	return c
}

func (c *Credentials) WithRegion(region string) *Credentials {
	c.Region = region
	return c
}

func (c *Credentials) WithS3ForcePathStyle(forcePathStyle bool) *Credentials {
	c.S3ForcePathStyle = forcePathStyle
	return c
}

func (c *Credentials) GetAccessKeyID() string {
	return c.AccessKeyID
}

func (c *Credentials) GetSecretAccessKey() string {
	return c.SecretAccessKey
}

func (c *Credentials) GetSessionToken() string {
	return c.SessionToken
}

func (c *Credentials) GetRegion() string {
	return c.Region
}

func (c *Credentials) GetEndpoint() string {
	return c.Endpoint
}

func (c *Credentials) GetS3ForcePathStyle() bool {
	return c.S3ForcePathStyle
}

func (c *Credentials) AWSConfig() aws.Config {
	region := c.Region
	if region == "" && c.Endpoint != "" {
		// S3-compatible stores (e.g. MinIO, SeaweedFS) ignore the region, but SigV4 signing still
		// requires one to be set. Supply a placeholder so requests to a custom endpoint can be signed.
		region = "us-east-1"
	}

	return aws.Config{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(
			c.AccessKeyID,
			c.SecretAccessKey,
			c.SessionToken,
		),
	}
}
