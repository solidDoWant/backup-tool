package servers

import (
	"context"
	"fmt"
	"net"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
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

	serveCtx := contexts.NewContext(context.TODO()) // TODO add logger, etc.
	grpcServer := grpc.NewServer(
		// This is a workaround to provide the serve context to handler functions
		// Handlers should call detatchHandlerContext to get the serve context
		grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(attachHandlerContext(ctx, serveCtx), req)
		}),
		grpc.StreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(attachHandlerContext(ss.Context(), serveCtx), ss)
		}),
	)
	registerServers(grpcServer)
	err = grpcServer.Serve(listener)
	return trace.Wrap(err, "grpc server failed")
}

// These are a workaround to provide the serve context to handler functions
func attachHandlerContext(ctx context.Context, realCtx *contexts.Context) context.Context {
	realCtx.Context = ctx
	return realCtx
}

func detatchHandlerContext(ctx context.Context) *contexts.Context {
	return ctx.(*contexts.Context)
}
