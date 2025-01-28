package backuptoolinstance

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	coreClient         *core.MockClientInterface
	backupToolInstance *MockBackupToolInstanceInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	coreClient := core.NewMockClientInterface(t)
	backupToolInstance := NewMockBackupToolInstanceInterface(t)

	provider := NewProvider(coreClient)
	provider.newBackupToolInstance = func() BackupToolInstanceInterface {
		return backupToolInstance
	}

	return &mockProvider{
		Provider:           provider,
		coreClient:         coreClient,
		backupToolInstance: backupToolInstance,
	}
}

func TestNewProvider(t *testing.T) {
	coreClient := core.NewMockClientInterface(t)

	provider := NewProvider(coreClient)
	require.NotNil(t, provider)

	assert.Equal(t, coreClient, provider.coreClient)
}
