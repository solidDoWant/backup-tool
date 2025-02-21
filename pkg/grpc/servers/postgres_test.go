package servers

import (
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/utils/ptr"
)

func TestNewPostgresServer(t *testing.T) {
	server := NewPostgresServer()

	assert.NotNil(t, server)
	assert.NotNil(t, server.runtime)
}

func TestDecodePostgresCredentials(t *testing.T) {
	testCases := []struct {
		desc     string
		input    *postgres_v1.EnvironmentCredentials
		expected postgres.Credentials
	}{
		{
			desc: "empty credentials",
			input: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{},
			}.Build(),
			expected: postgres.EnvironmentCredentials(postgres.CredentialVariables{}),
		},
		{
			desc: "single credential",
			input: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  postgres_v1.VarName_PGHOST.Enum(),
						Value: ptr.To("localhost"),
					}.Build(),
				},
			}.Build(),
			expected: postgres.EnvironmentCredentials(postgres.CredentialVariables{
				postgres.HostVarName: "localhost",
			}),
		},
		{
			desc: "multiple credentials",
			input: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  postgres_v1.VarName_PGHOST.Enum(),
						Value: ptr.To("localhost"),
					}.Build(),
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  postgres_v1.VarName_PGPORT.Enum(),
						Value: ptr.To("1234"),
					}.Build(),
				},
			}.Build(),
			expected: postgres.EnvironmentCredentials(postgres.CredentialVariables{
				postgres.HostVarName: "localhost",
				postgres.PortVarName: "1234",
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := decodePostgresCredentials(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDecodePostgresDumpAllOptions(t *testing.T) {
	tests := []struct {
		name  string
		input *postgres_v1.DumpAllOptions
		want  postgres.DumpAllOptions
	}{

		{
			name:  "no options",
			input: &postgres_v1.DumpAllOptions{},
			want:  postgres.DumpAllOptions{CleanupTimeout: 0},
		},
		{
			name: "All options",
			input: postgres_v1.DumpAllOptions_builder{
				CleanupTimeout: durationpb.New(5 * time.Second),
			}.Build(),
			want: postgres.DumpAllOptions{CleanupTimeout: helpers.MaxWaitTime(5 * time.Second)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodePostgresDumpAllOptions(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDumpAll(t *testing.T) {
	testCases := []struct {
		desc        string
		credentials *postgres_v1.EnvironmentCredentials
		outputPath  string
		opts        *postgres_v1.DumpAllOptions
		runtimeErr  error
		shouldError bool
	}{
		{
			desc: "successful dump",
			credentials: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  postgres_v1.VarName_PGHOST.Enum(),
						Value: ptr.To("localhost"),
					}.Build(),
				},
			}.Build(),
			opts: postgres_v1.DumpAllOptions_builder{
				CleanupTimeout: durationpb.New(10 * time.Second),
			}.Build(),
			outputPath: "/tmp/dump.sql",
		},
		{
			desc: "runtime error",
			credentials: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{},
			}.Build(),
			outputPath:  "/tmp/dump.sql",
			runtimeErr:  assert.AnError,
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Setup mock
			runtime := postgres.NewMockRuntime(t)
			server := NewPostgresServer()
			server.runtime = runtime

			ctx := th.NewTestContext()
			decodedCredentials := decodePostgresCredentials(tc.credentials)
			decodedOpts := decodePostgresDumpAllOptions(tc.opts)
			runtime.EXPECT().DumpAll(contexts.UnwrapHandlerContext(ctx), decodedCredentials, tc.outputPath, decodedOpts).Return(tc.runtimeErr)

			// Create request
			req := postgres_v1.DumpAllRequest_builder{
				Credentials:    tc.credentials,
				OutputFilePath: &tc.outputPath,
				Options:        tc.opts,
			}.Build()

			// Execute test
			result, err := server.DumpAll(ctx, req)

			// Assert results
			if tc.shouldError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestDecodeRestoreOptions(t *testing.T) {
	assert.Equal(t, postgres.RestoreOptions{}, decodePostgresRestoreOptions(&postgres_v1.RestoreOptions{}))
}

func TestRestore(t *testing.T) {
	testCases := []struct {
		desc        string
		credentials *postgres_v1.EnvironmentCredentials
		inputPath   string
		opts        *postgres_v1.RestoreOptions
		runtimeErr  error
		shouldError bool
	}{
		{
			desc: "successful restore",
			credentials: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  postgres_v1.VarName_PGHOST.Enum(),
						Value: ptr.To("localhost"),
					}.Build(),
				},
			}.Build(),
			inputPath: "/tmp/restore.sql",
		},
		{
			desc: "runtime error",
			credentials: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{},
			}.Build(),
			inputPath:   "/tmp/restore.sql",
			runtimeErr:  assert.AnError,
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Setup mock
			runtime := postgres.NewMockRuntime(t)
			server := NewPostgresServer()
			server.runtime = runtime

			ctx := th.NewTestContext()
			decodedCredentials := decodePostgresCredentials(tc.credentials)
			decodedOpts := decodePostgresRestoreOptions(tc.opts)
			runtime.EXPECT().Restore(contexts.UnwrapHandlerContext(ctx), decodedCredentials, tc.inputPath, decodedOpts).Return(tc.runtimeErr)

			// Create request
			req := postgres_v1.RestoreRequest_builder{
				Credentials:   tc.credentials,
				InputFilePath: &tc.inputPath,
				Options:       tc.opts,
			}.Build()

			// Execute test
			result, err := server.Restore(ctx, req)

			// Assert results
			if tc.shouldError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
