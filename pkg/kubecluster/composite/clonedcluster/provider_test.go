package clonedcluster

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforcertificate"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/barmancloud"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	cmClient          *certmanager.MockClientInterface
	cnpgClient        *cnpg.MockClientInterface
	apClient          *approverpolicy.MockClientInterface
	barmanCloudClient *barmancloud.MockClientInterface
	coreClient        *core.MockClientInterface
	clonedCluster     *MockClonedClusterInterface
	cucp              *clusterusercert.MockProviderInterface
	ccfp              *createcrpforcertificate.MockProviderInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)
	apClient := approverpolicy.NewMockClientInterface(t)
	barmanCloudClient := barmancloud.NewMockClientInterface(t)
	coreClient := core.NewMockClientInterface(t)
	cucp := clusterusercert.NewMockProviderInterface(t)
	ccfp := createcrpforcertificate.NewMockProviderInterface(t)
	clonedCluster := NewMockClonedClusterInterface(t)

	provider := NewProvider(cucp, ccfp, cmClient, cnpgClient, apClient, barmanCloudClient, coreClient)
	provider.newClonedCluster = func() ClonedClusterInterface {
		return clonedCluster
	}

	return &mockProvider{
		Provider:          provider,
		cmClient:          cmClient,
		cnpgClient:        cnpgClient,
		apClient:          apClient,
		barmanCloudClient: barmanCloudClient,
		coreClient:        coreClient,
		clonedCluster:     clonedCluster,
		cucp:              cucp,
		ccfp:              ccfp,
	}
}

func TestNewProvider(t *testing.T) {
	cmClient := certmanager.NewMockClientInterface(t)
	cnpgClient := cnpg.NewMockClientInterface(t)
	apClient := approverpolicy.NewMockClientInterface(t)
	barmanCloudClient := barmancloud.NewMockClientInterface(t)
	coreClient := core.NewMockClientInterface(t)
	cucp := clusterusercert.NewMockProviderInterface(t)
	ccfp := createcrpforcertificate.NewMockProviderInterface(t)

	provider := NewProvider(cucp, ccfp, cmClient, cnpgClient, apClient, barmanCloudClient, coreClient)
	require.NotNil(t, provider)

	assert.Equal(t, cmClient, provider.cmClient)
	assert.Equal(t, cnpgClient, provider.cnpgClient)
	assert.Equal(t, apClient, provider.apClient)
	assert.Equal(t, barmanCloudClient, provider.barmanCloudClient)
	assert.Equal(t, coreClient, provider.coreClient)
	assert.Equal(t, ccfp, provider.ccfp)
}
