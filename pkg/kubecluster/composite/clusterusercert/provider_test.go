package clusterusercert

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforprofile"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	*Provider
	apClient        *approverpolicy.MockClientInterface
	cmClient        *certmanager.MockClientInterface
	ccfp            *createcrpforprofile.MockProviderInterface
	clusterUserCert *MockClusterUserCertInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	apClient := approverpolicy.NewMockClientInterface(t)
	cmClient := certmanager.NewMockClientInterface(t)
	ccfp := createcrpforprofile.NewMockProviderInterface(t)
	clusterUserCert := NewMockClusterUserCertInterface(t)
	provider := NewProvider(apClient, cmClient)
	provider.ccfp = ccfp
	provider.newClusterUserCert = func() ClusterUserCertInterface {
		return clusterUserCert
	}

	return &mockProvider{
		Provider:        provider,
		apClient:        apClient,
		cmClient:        cmClient,
		ccfp:            ccfp,
		clusterUserCert: clusterUserCert,
	}
}

func TestNewProvider(t *testing.T) {
	apClient := approverpolicy.NewMockClientInterface(t)
	cmClient := certmanager.NewMockClientInterface(t)

	provider := NewProvider(apClient, cmClient)
	require.NotNil(t, provider)

	assert.Equal(t, apClient, provider.apClient)
	assert.Equal(t, cmClient, provider.cmClient)
	assert.NotNil(t, provider.ccfp)
	assert.NotNil(t, provider.newClusterUserCert)
}
