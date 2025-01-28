package clonedcluster

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	cmClient      *certmanager.MockClientInterface
	cnpgClient    *cnpg.MockClientInterface
	clonedCluster *MockClonedClusterInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)
	clonedCluster := NewMockClonedClusterInterface(t)

	provider := NewProvider(cmClient, cnpgClient)
	provider.newClonedCluster = func() ClonedClusterInterface {
		return clonedCluster
	}

	return &mockProvider{
		Provider:      provider,
		cmClient:      cmClient,
		cnpgClient:    cnpgClient,
		clonedCluster: clonedCluster,
	}
}

func TestNewProvider(t *testing.T) {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)

	provider := NewProvider(cmClient, cnpgClient)
	require.NotNil(t, provider)

	assert.Equal(t, cmClient, provider.cmClient)
	assert.Equal(t, cnpgClient, provider.cnpgClient)
}
