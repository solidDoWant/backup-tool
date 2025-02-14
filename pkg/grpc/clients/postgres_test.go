package clients

import (
	"fmt"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	postgres_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/postgres/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/utils/ptr"
)

func TestNewPostgresClient(t *testing.T) {
	// Create mock gRPC connection
	mockConn := &grpc.ClientConn{}

	// Call NewPostgresClient with mock connection
	client := NewPostgresClient(mockConn)

	// Assert client was created and is not nil
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Implements(t, (*postgres.Runtime)(nil), client)
}

func TestEncodeCredentialVariable(t *testing.T) {
	tests := []struct {
		name     string
		variable postgres.CredentialVariable
		want     postgres_v1.VarName
		wantErr  bool
	}{
		{
			name:     "valid variable",
			variable: postgres.HostVarName,
			want:     postgres_v1.VarName_PGHOST,
		},
		{
			name:     "invalid variable",
			variable: "INVALID_VAR",
			want:     postgres_v1.VarName(0),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeCredentialVariable(tt.variable)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEncodeCredentials(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[postgres.CredentialVariable]string
		want        *postgres_v1.EnvironmentCredentials // Note: credentials must be sorted by name.
		wantErr     bool
	}{
		{
			name: "valid credentials",
			credentials: map[postgres.CredentialVariable]string{
				postgres.HostVarName:     "localhost",
				postgres.PortVarName:     "5432",
				postgres.DatabaseVarName: "testdb",
			},
			want: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  ptr.To(postgres_v1.VarName_PGDATABASE),
						Value: ptr.To("testdb"),
					}.Build(),
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  ptr.To(postgres_v1.VarName_PGHOST),
						Value: ptr.To("localhost"),
					}.Build(),
					postgres_v1.EnvironmentCredentials_EnvironmentVariable_builder{
						Name:  ptr.To(postgres_v1.VarName_PGPORT),
						Value: ptr.To("5432"),
					}.Build(),
				},
			}.Build(),
		},
		{
			name:        "no credentials",
			credentials: map[postgres.CredentialVariable]string{},
			want: postgres_v1.EnvironmentCredentials_builder{
				Credentials: []*postgres_v1.EnvironmentCredentials_EnvironmentVariable{},
			}.Build(),
		},
		{
			name: "invalid variable",
			credentials: map[postgres.CredentialVariable]string{
				"INVALID_VAR": "value",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeCredentials(postgres.Credentials(postgres.EnvironmentCredentials(tt.credentials)))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEncodeDumpAllOptions(t *testing.T) {
	tests := []struct {
		name string
		opts postgres.DumpAllOptions
		want *postgres_v1.DumpAllOptions
	}{
		{
			name: "zero cleanup timeout",
			want: &postgres_v1.DumpAllOptions{},
		},
		{
			name: "non-zero cleanup timeout",
			opts: postgres.DumpAllOptions{CleanupTimeout: helpers.MaxWaitTime(5 * time.Second)},
			want: postgres_v1.DumpAllOptions_builder{
				CleanupTimeout: durationpb.New(5 * time.Second),
			}.Build(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeDumpAllOptions(tt.opts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDumpAll(t *testing.T) {
	tests := []struct {
		name          string
		credentials   map[postgres.CredentialVariable]string
		outputPath    string
		opts          postgres.DumpAllOptions
		mockResponse  *postgres_v1.DumpAllResponse
		mockError     error
		expectedError bool
	}{
		{
			name: "successful dump",
			credentials: map[postgres.CredentialVariable]string{
				postgres.HostVarName: "localhost",
				postgres.PortVarName: "5432",
			},
			outputPath: "/tmp/dump.sql",
			opts: postgres.DumpAllOptions{
				CleanupTimeout: helpers.MaxWaitTime(10 * time.Second),
			},
			mockResponse: &postgres_v1.DumpAllResponse{},
		},
		{
			name: "credentials encoding error",
			credentials: map[postgres.CredentialVariable]string{
				"INVALID_VAR": "value",
			},
			outputPath:    "/tmp/dump.sql",
			expectedError: true,
		},
		{
			name: "grpc error",
			credentials: map[postgres.CredentialVariable]string{
				postgres.HostVarName: "localhost",
			},
			outputPath:    "/tmp/dump.sql",
			mockError:     fmt.Errorf("grpc error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := postgres_v1.NewMockPostgresClient()

			ctx := th.NewTestContext()
			credentials := postgres.Credentials(postgres.EnvironmentCredentials(tt.credentials))

			// Setup the mock function call
			// The returned error is ignored to ensure that the function under test errors if the credentials are invalid
			encodedCredentials, credErr := encodeCredentials(credentials)
			encodedOpts := encodeDumpAllOptions(tt.opts)
			expectedRequest := postgres_v1.DumpAllRequest_builder{
				Credentials:    encodedCredentials,
				OutputFilePath: &tt.outputPath,
				Options:        encodedOpts,
			}.Build()
			mockClient.On("DumpAll", mock.Anything, expectedRequest, mock.Anything).
				Run(func(args mock.Arguments) {
					calledCtx := args.Get(0).(*contexts.Context)
					calledCtx.IsChildOf(ctx)
				}).
				Return(tt.mockResponse, tt.mockError)

			pc := &PostgresClient{client: mockClient}

			// Test
			err := pc.DumpAll(ctx, credentials, tt.outputPath, tt.opts)

			// Validate
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if credErr == nil {
				mockClient.AssertExpectations(t)
			}
		})
	}
}

func TestEncodeRestoreOptions(t *testing.T) {
	assert.Equal(t, &postgres_v1.RestoreOptions{}, encodeRestoreOptions(postgres.RestoreOptions{}))
}

func TestRestore(t *testing.T) {
	tests := []struct {
		name          string
		credentials   map[postgres.CredentialVariable]string
		inputPath     string
		mockResponse  *postgres_v1.RestoreResponse
		mockError     error
		expectedError bool
	}{
		{
			name: "successful restore",
			credentials: map[postgres.CredentialVariable]string{
				postgres.HostVarName: "localhost",
				postgres.PortVarName: "5432",
			},
			inputPath:    "/tmp/restore.sql",
			mockResponse: &postgres_v1.RestoreResponse{},
		},
		{
			name: "credentials encoding error",
			credentials: map[postgres.CredentialVariable]string{
				"INVALID_VAR": "value",
			},
			inputPath:     "/tmp/restore.sql",
			expectedError: true,
		},
		{
			name: "grpc error",
			credentials: map[postgres.CredentialVariable]string{
				postgres.HostVarName: "localhost",
			},
			inputPath:     "/tmp/restore.sql",
			mockError:     fmt.Errorf("grpc error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := postgres_v1.NewMockPostgresClient()

			ctx := th.NewTestContext()
			credentials := postgres.Credentials(postgres.EnvironmentCredentials(tt.credentials))

			// Setup the mock function call
			// The returned error is ignored to ensure that the function under test errors if the credentials are invalid
			encodedCredentials, credErr := encodeCredentials(credentials)
			expectedRequest := postgres_v1.RestoreRequest_builder{
				Credentials:   encodedCredentials,
				InputFilePath: &tt.inputPath,
				Options:       encodeRestoreOptions(postgres.RestoreOptions{}),
			}.Build()
			mockClient.On("Restore", mock.Anything, expectedRequest, mock.Anything).
				Run(func(args mock.Arguments) {
					calledCtx := args.Get(0).(*contexts.Context)
					calledCtx.IsChildOf(ctx)
				}).
				Return(tt.mockResponse, tt.mockError)

			pc := &PostgresClient{client: mockClient}

			// Test
			err := pc.Restore(ctx, credentials, tt.inputPath, postgres.RestoreOptions{})

			// Validate
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if credErr == nil {
				mockClient.AssertExpectations(t)
			}
		})
	}
}
