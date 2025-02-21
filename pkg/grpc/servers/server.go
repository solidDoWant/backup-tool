package servers

import (
	"context"
	"fmt"
	"net"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	s3_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/s3/v1"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpchealth_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

// Wraps a server stream to allow for injecting a context
type streamWrapper struct {
	gogrpc.ServerStream
	ctx context.Context
}

func (sw *streamWrapper) Context() context.Context {
	return sw.ctx
}

// This is to allow for dep injection during tests
var startServerListener = func(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}

var registerServers = func(registrar gogrpc.ServiceRegistrar) {
	// This can be provided to other services if needed
	healthcheckService := health.NewServer()

	files_v1.RegisterFilesServer(registrar, NewFilesServer())
	postgres_v1.RegisterPostgresServer(registrar, NewPostgresServer())
	s3_v1.RegisterS3Server(registrar, NewS3Server())
	grpchealth_v1.RegisterHealthServer(registrar, healthcheckService)

	healthcheckService.SetServingStatus(grpc.SystemService, grpchealth_v1.HealthCheckResponse_SERVING)
}

func StartServer(ctx *contexts.Context) error {
	ctx.Log.Info("Starting GRPC server", "port", grpc.GRPCPort)
	defer ctx.Log.Info("GRPC server stopped", ctx.Stopwatch.Keyval())

	listener, err := startServerListener(grpc.GRPCPort)
	if err != nil {
		return trace.Wrap(err, "failed to listen on port %d", grpc.GRPCPort)
	}

	grpcServer := gogrpc.NewServer(
		// This is a workaround to provide the serve context to handler functions
		// Handlers should call detatchHandlerContext to get the serve context
		gogrpc.ChainUnaryInterceptor(unaryContextAttachInterceptor(ctx), unaryLoggingInterceptor(ctx)),
		gogrpc.ChainStreamInterceptor(streamContextAttachInterceptor(ctx), streamLoggingInterceptor(ctx)),
	)
	registerServers(grpcServer)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	err = grpcServer.Serve(listener)
	return trace.Wrap(err, "grpc server failed")
}

func unaryContextAttachInterceptor(ctx *contexts.Context) func(callCtx context.Context, req any, info *gogrpc.UnaryServerInfo, handler gogrpc.UnaryHandler) (resp any, err error) {
	return func(callCtx context.Context, req any, info *gogrpc.UnaryServerInfo, handler gogrpc.UnaryHandler) (resp any, err error) {
		return handler(contexts.WrapHandlerContext(callCtx, ctx.Child()), req)
	}
}

func streamContextAttachInterceptor(ctx *contexts.Context) func(srv any, ss gogrpc.ServerStream, info *gogrpc.StreamServerInfo, handler gogrpc.StreamHandler) error {
	return func(srv any, ss gogrpc.ServerStream, info *gogrpc.StreamServerInfo, handler gogrpc.StreamHandler) error {
		wrappedStream := &streamWrapper{
			ServerStream: ss,
			ctx:          contexts.WrapHandlerContext(ss.Context(), ctx.Child()),
		}
		return handler(srv, wrappedStream)
	}
}

func unaryLoggingInterceptor(ctx *contexts.Context) func(callCtx context.Context, req any, info *gogrpc.UnaryServerInfo, handler gogrpc.UnaryHandler) (resp any, err error) {
	return func(callCtx context.Context, req any, info *gogrpc.UnaryServerInfo, handler gogrpc.UnaryHandler) (resp any, err error) {
		ctx = ctx.Child() // Each request should have its own context
		ctx.Log.With(grpcKeyvals(info.FullMethod)...).Debug("Processing unary GRPC method invokation")
		defer ctx.Log.Debug("Finished processing unary GRPC method invokation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

		return handler(callCtx, req)
	}
}

func streamLoggingInterceptor(ctx *contexts.Context) func(srv any, ss gogrpc.ServerStream, info *gogrpc.StreamServerInfo, handler gogrpc.StreamHandler) error {
	return func(srv any, ss gogrpc.ServerStream, info *gogrpc.StreamServerInfo, handler gogrpc.StreamHandler) (err error) {
		ctx = ctx.Child() // Each request should have its own context
		ctx.Log.With(grpcKeyvals(info.FullMethod)...).Debug("Processing streaming GRPC method invokation")
		defer ctx.Log.Debug("Finished processing streaming GRPC method invokation", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

		return handler(srv, ss)
	}
}

func grpcKeyvals(method string) []interface{} {
	return []interface{}{"method", method}
}
