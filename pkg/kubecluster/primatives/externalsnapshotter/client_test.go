package externalsnapshotter

import (
	"testing"

	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient(&rest.Config{})
	assert.NoError(t, err)

	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	// This helps static analysis tools catch when the implementation drifts from the interface
	var casted ClientInterface = client
	assert.Implements(t, (*ClientInterface)(nil), casted)
}

func createTestClient() (*Client, *fake.Clientset) {
	fakeClient := fake.NewSimpleClientset()
	return &Client{
		client: fakeClient,
	}, fakeClient
}
