package grpc

const (
	GRPCPort      = 40983
	SystemService = "" // Empty string is the default healthcheck service name. Not exported by the grpc lib for some reason, but it's the standard name.
)
