package certmanager

import (
	"testing"

	cmfake "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient(&rest.Config{})
	assert.NoError(t, err)

	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	var casted ClientInterface = client
	assert.Implements(t, (*ClientInterface)(nil), casted)
}

func createTestClient() (*Client, *cmfake.Clientset) {
	fakeClient := cmfake.NewSimpleClientset()
	return &Client{
		client: fakeClient,
	}, fakeClient
}
