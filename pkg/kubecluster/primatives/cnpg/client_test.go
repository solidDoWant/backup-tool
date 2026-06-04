package cnpg

import (
	"testing"

	cnpgfake "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient(&rest.Config{})
	assert.NoError(t, err)

	assert.NotNil(t, client)
	assert.NotNil(t, client.cnpgClient)
	var casted ClientInterface = client
	assert.Implements(t, (*ClientInterface)(nil), casted)
}

func createTestClient() (*Client, *cnpgfake.Clientset) {
	fakeCNPGClient := cnpgfake.NewSimpleClientset()
	return &Client{
		cnpgClient: fakeCNPGClient,
	}, fakeCNPGClient
}
