package kubecluster

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	client := NewClient(
		certmanager.NewMockClientInterface(t),
		cnpg.NewMockClientInterface(t),
		externalsnapshotter.NewMockClientInterface(t),
		core.NewMockClientInterface(t),
		approverpolicy.NewMockClientInterface(t),
	)

	assert.NotNil(t, client)
	assert.NotNil(t, client.CM())
	assert.NotNil(t, client.CNPG())
	assert.NotNil(t, client.ES())
	assert.NotNil(t, client.Core())
	assert.NotNil(t, client.AP())
	assert.NotNil(t, client.backupToolInstanceProvider)
	assert.NotNil(t, client.clonedClusterProvider)
	assert.NotNil(t, client.clonePVCProvider)
	assert.NotNil(t, client.clusterUserCertProvider)
	assert.NotNil(t, client.createCRPForProfileProvider)
}
