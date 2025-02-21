package clients

import (
	"maps"
	"slices"
	"time"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
)

type PostgresClient struct {
	client postgres_v1.PostgresClient
}

func NewPostgresClient(grpcConnection grpc.ClientConnInterface) *PostgresClient {
	return &PostgresClient{
		client: postgres_v1.NewPostgresClient(grpcConnection),
	}
}

func encodePostgresCredentialVariable(variable postgres.CredentialVariable) (postgres_v1.VarName, error) {
	if i, ok := postgres_v1.VarName_value[string(variable)]; ok {
		return postgres_v1.VarName(i), nil
	}

	// If this is hit then the protobuf enum is out of sync with the CredentialVariable enum
	var defaultVal postgres_v1.VarName
	return defaultVal, trace.Errorf("unknown variable: %s", variable)
}

// Encode a credential implementation instance into a protobuf-compatible value
func encodePostgresCredentials(credentials postgres.Credentials) (*postgres_v1.EnvironmentCredentials, error) {
	variables := credentials.GetVariables()
	encodedEnvironmentVariables := make([]*postgres_v1.EnvironmentCredentials_EnvironmentVariable, 0, len(variables))

	// Iterate over the map in a deterministic order. This isn't very important normally, but it makes testing easier.
	variableNames := slices.Collect(maps.Keys(variables))
	slices.Sort(variableNames)

	for _, variableName := range variableNames {
		variableValue := variables[variableName]
		encodedVariable, err := encodePostgresCredentialVariable(variableName)
		if err != nil {
			return nil, trace.Wrap(err, "failed to encode credential variable")
		}

		encodedEnvironmentVariable := &postgres_v1.EnvironmentCredentials_EnvironmentVariable{}
		encodedEnvironmentVariable.SetName(encodedVariable)
		encodedEnvironmentVariable.SetValue(variableValue)
		encodedEnvironmentVariables = append(encodedEnvironmentVariables, encodedEnvironmentVariable)
	}

	encodedCredentials := &postgres_v1.EnvironmentCredentials{}
	encodedCredentials.SetCredentials(encodedEnvironmentVariables)
	return encodedCredentials, nil
}

func encodePostgresDumpAllOptions(opts postgres.DumpAllOptions) *postgres_v1.DumpAllOptions {
	encodedOpts := &postgres_v1.DumpAllOptions{}

	if opts.CleanupTimeout != 0 {
		encodedOpts.SetCleanupTimeout(durationpb.New(time.Duration(opts.CleanupTimeout)))
	}

	return encodedOpts
}

func (pc *PostgresClient) DumpAll(ctx *contexts.Context, credentials postgres.Credentials, outputFilePath string, opts postgres.DumpAllOptions) error {
	ctx.Log.With("outputFilePath", outputFilePath, "address", postgres.GetServerAddress(credentials), "username", credentials.GetUsername()).Info("Dumping all databases")
	defer ctx.Log.Info("Finished dumping databases", ctx.Stopwatch.Keyval())

	encodedCredentials, err := encodePostgresCredentials(credentials)
	if err != nil {
		return trace.Wrap(err, "failed to encode credentials")
	}

	request := postgres_v1.DumpAllRequest_builder{
		Credentials:    encodedCredentials,
		OutputFilePath: &outputFilePath,
		Options:        encodePostgresDumpAllOptions(opts),
	}.Build()

	var header metadata.MD
	_, err = pc.client.DumpAll(ctx.Child(), request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}

func encodePostgresRestoreOptions(_ postgres.RestoreOptions) *postgres_v1.RestoreOptions {
	return &postgres_v1.RestoreOptions{}
}

func (pc *PostgresClient) Restore(ctx *contexts.Context, credentials postgres.Credentials, inputFilePath string, opts postgres.RestoreOptions) error {
	ctx.Log.With("inputFilePath", inputFilePath, "address", postgres.GetServerAddress(credentials), "username", credentials.GetUsername()).Info("Restoring all databases")
	defer ctx.Log.Info("Finished restoring databases", ctx.Stopwatch.Keyval())

	encodedCredentials, err := encodePostgresCredentials(credentials)
	if err != nil {
		return trace.Wrap(err, "failed to encode credentials")
	}

	request := postgres_v1.RestoreRequest_builder{
		Credentials:   encodedCredentials,
		InputFilePath: &inputFilePath,
		Options:       encodePostgresRestoreOptions(opts),
	}.Build()

	var header metadata.MD
	_, err = pc.client.Restore(ctx.Child(), request, grpc.Header(&header))
	return trail.FromGRPC(err, header)
}
