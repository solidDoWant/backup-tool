package cnpg

import (
	"testing"

	cnpgfake "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/client-go/rest"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient(&rest.Config{})
	assert.NoError(t, err)

	assert.NotNil(t, client)
	assert.NotNil(t, client.cnpgClient)
	assert.NotNil(t, client.apiExtensionsClient)
	var casted ClientInterface = client
	assert.Implements(t, (*ClientInterface)(nil), casted)
}

func createTestClient() (*Client, *cnpgfake.Clientset, *apiextensionsfake.Clientset) {
	fakeCNPGClient := cnpgfake.NewSimpleClientset()
	// This cannot be updated to `NewClientset` until
	// https://github.com/kubernetes/kubernetes/issues/126850 is fixed
	fakeApiExtensionsClient := apiextensionsfake.NewSimpleClientset()
	return &Client{
		cnpgClient:          fakeCNPGClient,
		apiExtensionsClient: fakeApiExtensionsClient,
	}, fakeCNPGClient, fakeApiExtensionsClient
}
