package kubecluster

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/stretchr/testify/assert"
)

// Embeds `Client`, while providing references to the mock implementations of primative clients.
type mockClient struct {
	*Client
	cmClient           *certmanager.MockClientInterface
	cnpgClient         *cnpg.MockClientInterface
	esClient           *externalsnapshotter.MockClientInterface
	coreClient         *core.MockClientInterface
	clonedCluster      *MockClonedClusterInterface
	backupToolInstance *MockBackupToolInstanceInterface
}

func newMockClient(t *testing.T) *mockClient {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)
	esClient := externalsnapshotter.NewMockClientInterface(t)
	coreClient := core.NewMockClientInterface(t)
	clonedCluster := NewMockClonedClusterInterface(t)
	backupToolInstance := NewMockBackupToolInstanceInterface(t)
	client := NewClient(cmClient, cnpgClient, esClient, coreClient)
	casted := client.(*Client)
	casted.newClonedCluster = func() ClonedClusterInterface {
		return clonedCluster
	}
	casted.newBackupToolInstance = func() BackupToolInstanceInterface {
		return backupToolInstance
	}

	return &mockClient{
		Client:             casted,
		cmClient:           cmClient,
		cnpgClient:         cnpgClient,
		esClient:           esClient,
		coreClient:         coreClient,
		clonedCluster:      clonedCluster,
		backupToolInstance: backupToolInstance,
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient(
		certmanager.NewMockClientInterface(t),
		cnpg.NewMockClientInterface(t),
		externalsnapshotter.NewMockClientInterface(t),
		core.NewMockClientInterface(t),
	)

	assert.NotNil(t, client)
	assert.NotNil(t, client.CM())
	assert.NotNil(t, client.CNPG())
	assert.NotNil(t, client.ES())
	assert.NotNil(t, client.Core())

	casted := client.(*Client)

	// Ensure that the `newClonedCluster` function returns a new instance each time.
	// The mock client does not do this.
	cc1 := casted.newClonedCluster()
	cc2 := casted.newClonedCluster()
	assert.NotSame(t, cc1, cc2)

	// Ensure that the `newBackupToolInstance` function returns a new instance each time.
	bt1 := casted.newBackupToolInstance()
	bt2 := casted.newBackupToolInstance()
	assert.NotSame(t, bt1, bt2)
}
