package clonedcluster

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
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
	cucp          *clusterusercert.MockProviderInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)
	cucp := clusterusercert.NewMockProviderInterface(t)
	clonedCluster := NewMockClonedClusterInterface(t)

	provider := NewProvider(cucp, cmClient, cnpgClient)
	provider.newClonedCluster = func() ClonedClusterInterface {
		return clonedCluster
	}

	return &mockProvider{
		Provider:      provider,
		cmClient:      cmClient,
		cnpgClient:    cnpgClient,
		clonedCluster: clonedCluster,
		cucp:          cucp,
	}
}

func TestNewProvider(t *testing.T) {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)
	cucp := clusterusercert.NewMockProviderInterface(t)

	provider := NewProvider(cucp, cmClient, cnpgClient)
	require.NotNil(t, provider)

	assert.Equal(t, cmClient, provider.cmClient)
	assert.Equal(t, cnpgClient, provider.cnpgClient)
}
