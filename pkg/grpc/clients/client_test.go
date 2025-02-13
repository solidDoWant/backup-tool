package clients

import (
	"context"
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
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, client)
			assert.NotNil(t, client.Files())
			assert.NotNil(t, client.Postgres())
			assert.NotNil(t, client.Health())
		})
	}
}

func TestUnaryLoggingInterceptor(t *testing.T) {
	ctx := th.NewTestContext()
	method := "method"
	req := "request"
	reply := "reply"
	cc := &grpc.ClientConn{}
	opts := []grpc.CallOption{
		grpc.HeaderCallOption{},
	}
	errResponse := assert.AnError

	invokerCalled := false
	invoker := func(calledCtx context.Context, calledMethod string, calledReq, calledReply any, calledCC *grpc.ClientConn, calledOpts ...grpc.CallOption) error {
		invokerCalled = true

		assert.Equal(t, ctx, calledCtx)
		assert.Equal(t, method, calledMethod)
		assert.Equal(t, req, calledReq)
		assert.Equal(t, reply, calledReply)
		assert.Equal(t, cc, calledCC)
		assert.Equal(t, opts, calledOpts)

		return errResponse
	}

	err := unaryLoggingInterceptor(ctx, method, req, reply, cc, invoker, opts...)
	assert.True(t, invokerCalled)
	assert.Equal(t, errResponse, err)
}

func TestStreamLoggingInterceptor(t *testing.T) {
	ctx := th.NewTestContext()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	opts := []grpc.CallOption{
		grpc.HeaderCallOption{},
	}
	errResponse := assert.AnError

	streamerCalled := false
	streamer := func(calledCtx context.Context, calledDesc *grpc.StreamDesc, calledCC *grpc.ClientConn, calledMethod string, calledOpts ...grpc.CallOption) (stream grpc.ClientStream, err error) {
		streamerCalled = true

		assert.Equal(t, ctx, calledCtx)
		assert.Equal(t, desc, calledDesc)
		assert.Equal(t, cc, calledCC)
		assert.Equal(t, method, calledMethod)
		assert.Equal(t, opts, calledOpts)

		return nil, errResponse
	}

	stream, err := streamLoggingInterceptor(ctx, desc, cc, method, streamer, opts...)
	assert.True(t, streamerCalled)
	assert.Nil(t, stream)
	assert.Equal(t, errResponse, err)
}

func TestGrpcKeyvals(t *testing.T) {
	calledMethod := "called/method"
	assert.Equal(t, []interface{}{"method", calledMethod}, grpcKeyvals(calledMethod))
}
