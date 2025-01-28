package createcrpforcertificate

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	apClient *approverpolicy.MockClientInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	apClient := approverpolicy.NewMockClientInterface(t)
	provider := NewProvider(apClient)

	return &mockProvider{
		Provider: provider,
		apClient: apClient,
	}
}

func TestNewProvider(t *testing.T) {
	apClient := approverpolicy.NewMockClientInterface(t)

	provider := NewProvider(apClient)
	require.NotNil(t, provider)

	assert.Equal(t, apClient, provider.apClient)
}
