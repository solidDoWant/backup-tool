package servers

import (
	"fmt"
	"net"

	"github.com/gravitational/trace"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	"google.golang.org/grpc"
)

const GRPCPort = 40983

// This is to allow for dep injection during tests
var startServerListener = func(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}

var registerServers = func(registrar grpc.ServiceRegistrar) {
	files_v1.RegisterFilesServer(registrar, NewFilesServer())
	postgres_v1.RegisterPostgresServer(registrar, NewPostgresServer())
}

func StartServer() error {
	listener, err := startServerListener(GRPCPort)
	if err != nil {
		return trace.Wrap(err, "failed to listen on port %d", GRPCPort)
	}

	grpcServer := grpc.NewServer()
	registerServers(grpcServer)
	err = grpcServer.Serve(listener)
	return trace.Wrap(err, "grpc server failed")
}
