package grpc_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/grpc"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/grpc/servers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Perform a basic client/server integration test
func TestClientServerCompatibility(t *testing.T) {
	servingCtx := th.NewTestContext()
	go func() {
		err := servers.StartServer(servingCtx)
		assert.NoError(t, err)
	}()

	// Wait a moment for the server to start
	time.Sleep(50 * time.Millisecond)

	clientCtx := th.NewTestContext()
	client, err := clients.NewClient(clientCtx, net.JoinHostPort("localhost", fmt.Sprintf("%d", grpc.GRPCPort)))
	require.NoError(t, err)

	resp, err := client.Health().Check(clientCtx, &grpc_health_v1.HealthCheckRequest{})
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)

	watchResp, err := client.Health().Watch(clientCtx, &grpc_health_v1.HealthCheckRequest{})
	assert.NoError(t, err)
	require.NotNil(t, watchResp)

	resp, err = watchResp.Recv()
	assert.NoError(t, err)
	defer func() {
		err := watchResp.CloseSend()
		assert.NoError(t, err)
	}()

	require.NotNil(t, resp)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
}
