package clients

import (
	"context"
	"net"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
}

func NewClient(ctx context.Context, serverAddress string) (ClientInterface, error) {
	// Leave authz to be handled by other cluster services, such as Istio.
	// TODO add option for this
	conn, err := grpc.NewClient(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, trace.Wrap(err, "failed to create new backup-tool GRPC client for server at %q", serverAddress)
	}

	// Verify the client _can_ connect to the server
	// GRPC uses multiple connections, so all this does is verify that at least one connection can be established once
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddress)
	if err != nil {
		return nil, trace.Wrap(err, "failed to resolve server address %q", serverAddress)
	}

	tcpConn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, trace.Wrap(err, "failed to establish TCP connection to %q", serverAddress)
	}
	err = tcpConn.Close()
	if err != nil {
		return nil, trace.Wrap(err, "failed to close TCP connection to %q", serverAddress)
	}

	return &Client{
		conn:     conn,
		files:    NewFilesClient(conn),
		postgres: NewPostgresClient(conn),
	}, nil
}

func (c *Client) Files() files.Runtime {
	return c.files
}

func (c *Client) Postgres() postgres.Runtime {
	return c.postgres
}

func (c *Client) Close() error {
	return c.conn.Close()
}
