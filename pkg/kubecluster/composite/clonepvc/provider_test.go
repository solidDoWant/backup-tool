package clonepvc

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	coreClient *core.MockClientInterface
	esClient   *externalsnapshotter.MockClientInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	coreClient := core.NewMockClientInterface(t)
	esClient := externalsnapshotter.NewMockClientInterface(t)

	provider := NewProvider(coreClient, esClient)

	return &mockProvider{
		Provider:   provider,
		coreClient: coreClient,
		esClient:   esClient,
	}
}

func TestNewProvider(t *testing.T) {
	esClient := externalsnapshotter.NewMockClientInterface(t)
	coreClient := core.NewMockClientInterface(t)

	provider := NewProvider(coreClient, esClient)
	require.NotNil(t, provider)

	assert.Equal(t, coreClient, provider.coreClient)
	assert.Equal(t, esClient, provider.esClient)
}
