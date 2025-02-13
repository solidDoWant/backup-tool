package clients

import (
	"context"
	"net"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type ClientInterface interface {
	Files() files.Runtime
	Postgres() postgres.Runtime
	Close() error
}

type Client struct {
	conn     *grpc.ClientConn
	files    *FilesClient
	postgres *PostgresClient
	health   grpc_health_v1.HealthClient
}

func NewClient(ctx *contexts.Context, serverAddress string) (*Client, error) {
	ctx.Log.With("address", serverAddress).Infof("Creating %s GRPC client", constants.ToolName)

	// Leave authz to be handled by other cluster services, such as Istio.
	// TODO add option for this
	conn, err := grpc.NewClient(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(unaryLoggingInterceptor), grpc.WithStreamInterceptor(streamLoggingInterceptor))
	if err != nil {
		return nil, trace.Wrap(err, "failed to create new %s GRPC client for server at %q", constants.ToolName, serverAddress)
	}

	if err := verifyConnection(ctx.Child(), serverAddress); err != nil {
		return nil, trace.Wrap(err, "failed to verify connection to server at %q", serverAddress)
	}

	return &Client{
		conn:     conn,
		files:    NewFilesClient(conn),
		postgres: NewPostgresClient(conn),
		health:   grpc_health_v1.NewHealthClient(conn),
	}, nil
}

func unaryLoggingInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	handlerCtx := contexts.UnwrapHandlerContext(ctx)
	handlerCtx.Log.With(grpcKeyvals(method)...).Debug("Calling unary GRPC method")
	defer handlerCtx.Log.Debug("Finished calling unary GRPC method", handlerCtx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	return invoker(ctx, method, req, reply, cc, opts...)
}

func streamLoggingInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (stream grpc.ClientStream, err error) {
	handlerCtx := contexts.UnwrapHandlerContext(ctx)
	handlerCtx.Log.With(grpcKeyvals(method)...).Debug("Calling streaming GRPC method")
	defer handlerCtx.Log.Debug("Finished calling streaming GRPC method", handlerCtx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	return streamer(ctx, desc, cc, method, opts...)
}

func grpcKeyvals(method string) []interface{} {
	return []interface{}{"method", method}
}

// Verify the client _can_ connect to the server
// GRPC uses multiple connections, so all this does is verify that at least one connection can be established once
func verifyConnection(ctx *contexts.Context, serverAddress string) (err error) {
	ctx.Log.Info("Verifying connection to server")
	defer func() {
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Failed to verify connection to server", keyvals...)
			return
		}
		ctx.Log.Info("Successfully verified connection to server", keyvals...)
	}()

	ctx.Log.Debug("Resolving server address")
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddress)
	if err != nil {
		return trace.Wrap(err, "failed to resolve server address %q", serverAddress)
	}
	ctx.Log.With("resolvedAddress", tcpAddr.String()).Debug("Resolved server address")

	ctx.Log.Debug("Attempting to establish TCP connection")
	tcpConn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return trace.Wrap(err, "failed to establish TCP connection to %q", serverAddress)
	}

	err = tcpConn.Close()
	return trace.Wrap(err, "failed to close TCP connection to %q", serverAddress)
}

func (c *Client) Files() files.Runtime {
	return c.files
}

func (c *Client) Postgres() postgres.Runtime {
	return c.postgres
}

func (c *Client) Health() grpc_health_v1.HealthClient {
	return c.health
}

func (c *Client) Close() error {
	return c.conn.Close()
}
