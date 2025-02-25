package drvolume

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	coreClient *core.MockClientInterface
	esClient   *externalsnapshotter.MockClientInterface
	cnpgClient *cnpg.MockClientInterface
	drv        *MockDRVolumeInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	coreClient := core.NewMockClientInterface(t)
	esClient := externalsnapshotter.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)

	drv := NewMockDRVolumeInterface(t)
	provider := NewProvider(coreClient, esClient, cnpgClient)
	provider.newDRVolume = func() DRVolumeInterface {
		return drv
	}

	return &mockProvider{
		Provider:   provider,
		coreClient: coreClient,
		esClient:   esClient,
		cnpgClient: cnpgClient,
		drv:        drv,
	}
}

func TestNewProvider(t *testing.T) {
	coreClient := core.NewMockClientInterface(t)
	esClient := externalsnapshotter.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)

	provider := NewProvider(coreClient, esClient, cnpgClient)
	require.NotNil(t, provider)

	assert.Equal(t, coreClient, provider.coreClient)
	assert.Equal(t, esClient, provider.esClient)
	assert.Equal(t, cnpgClient, provider.cnpgClient)
	assert.NotNil(t, provider.newDRVolume)
}
