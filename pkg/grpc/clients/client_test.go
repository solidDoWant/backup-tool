package clients

import (
	"net"
	"testing"
	"time"

	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// Setup test gRPC server
func setupServer(t *testing.T) (net.Listener, *grpc.Server) {
	lis, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()

	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			t.Logf("failed to serve gRPC server: %v", err)
		}
		require.NoError(t, err)
	}()

	return lis, grpcServer
}

func TestNewClient(t *testing.T) {
	var casted ClientInterface = &Client{}
	assert.Implements(t, (*ClientInterface)(nil), casted)

	tests := []struct {
		name          string
		addr          string
		useServerAddr bool
		wantErr       bool
	}{
		{
			name:          "successful connection",
			useServerAddr: true,
			wantErr:       false,
		},
		{
			name:    "invalid address",
			addr:    "invalid:address",
			wantErr: true,
		},
		{
			name:    "empty address",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.useServerAddr {
				lis, grpcServer := setupServer(t)
				defer lis.Close()
				defer grpcServer.GracefulStop()
				tt.addr = lis.Addr().String()

				// Wait for the server to start
				time.Sleep(3 * time.Millisecond)
			}

			ctx := th.NewTestContext()

			client, err := NewClient(ctx, tt.addr)
			if err == nil {
				defer func() {
					err = client.Close()
					assert.NoError(t, err)
				}()
			}

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.Files())
				assert.NotNil(t, client.Postgres())
			}

			// TODO verify that the client actually attempts to connect to the server
		})
	}
}
