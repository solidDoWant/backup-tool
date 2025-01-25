package servers

import (
	"context"

	"github.com/gravitational/trace/trail"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
)

type PostgresServer struct {
	postgres_v1.UnimplementedPostgresServer
	runtime postgres.Runtime
}

func NewPostgresServer() *PostgresServer {
	return &PostgresServer{
		runtime: postgres.NewLocalRuntime(),
	}
}

func decodeCredentials(encodedCredentials *postgres_v1.EnvironmentCredentials) postgres.Credentials {
	encodedCredentialEntries := encodedCredentials.GetCredentials()
	decodedCredentials := make(map[postgres.CredentialVariable]string, len(encodedCredentialEntries))
	for _, encodedCredentialEntry := range encodedCredentialEntries {
		// TODO this is a vulnerability that allows for setting arbitrary environment variables
		// This should be validated against a list of known environment variables, which is a PITA to do due to
		// golang not having enums
		decodedCredentials[postgres.CredentialVariable(encodedCredentialEntry.GetName().String())] = encodedCredentialEntry.GetValue()
	}

	return postgres.EnvironmentCredentials(postgres.CredentialVariables(decodedCredentials))
}

func decodeDumpAllOptions(encodedOptions *postgres_v1.DumpAllOptions) postgres.DumpAllOptions {
	opts := postgres.DumpAllOptions{}

	timeout := encodedOptions.GetCleanupTimeout()
	if timeout != nil {
		opts.CleanupTimeout = timeout.AsDuration()
	}

	return opts
}

func (ps *PostgresServer) DumpAll(ctx context.Context, req *postgres_v1.DumpAllRequest) (*postgres_v1.DumpAllResponse, error) {
	err := ps.runtime.DumpAll(ctx, decodeCredentials(req.GetCredentials()), req.GetOutputFilePath(), decodeDumpAllOptions(req.GetOptions()))
	if err != nil {
		return nil, trail.Send(ctx, err)
	}

	return &postgres_v1.DumpAllResponse{}, nil
}
