package servers

import (
	"fmt"
	"net"

	"github.com/gravitational/trace"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpchealth_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	GRPCPort      = 40983
	SystemService = "" // Empty string is the default healthcheck service name. Not exported by the grpc lib for some reason, but it's the standard name.
)

// This is to allow for dep injection during tests
var startServerListener = func(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}

var registerServers = func(registrar grpc.ServiceRegistrar) {
	// This can be provided to other services if needed
	healthcheckService := health.NewServer()

	files_v1.RegisterFilesServer(registrar, NewFilesServer())
	postgres_v1.RegisterPostgresServer(registrar, NewPostgresServer())
	grpchealth_v1.RegisterHealthServer(registrar, healthcheckService)

	healthcheckService.SetServingStatus(SystemService, grpchealth_v1.HealthCheckResponse_SERVING)
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
