package s3

import (
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

type CredentialsInterface interface {
	AWSConfig() *aws.Config
}

type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Endpoint        string
	Region          string
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
		WithRegion(os.Getenv("AWS_REGION"))
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

func (c *Credentials) AWSConfig() *aws.Config {
	config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(
			c.AccessKeyID,
			c.SecretAccessKey,
			c.SessionToken,
		),
	}

	if c.Endpoint != "" {
		config.Endpoint = aws.String(c.Endpoint)
	}

	if c.Region != "" {
		config.Region = aws.String(c.Region)
	}

	return config
}
