package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
	restfake "k8s.io/client-go/rest/fake"
)

func TestNewClient(t *testing.T) {
	mockRESTClient := &restfake.RESTClient{}
	client := NewClient(mockRESTClient)

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
