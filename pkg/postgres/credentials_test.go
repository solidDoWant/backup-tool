package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetDatabaseName(t *testing.T) {
	dbName := "testdb"
	tests := []struct {
		desc        string
		initialVars CredentialVariables
		expected    CredentialVariables
	}{
		{
			desc:     "empty vars",
			expected: CredentialVariables{DatabaseVarName: dbName},
		},
		{
			desc:        "existing vars",
			initialVars: CredentialVariables{UserVarName: "testuser"},
			expected:    CredentialVariables{UserVarName: "testuser", DatabaseVarName: dbName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := tt.initialVars.SetDatabaseName(dbName)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestToEnvSlice(t *testing.T) {
	tests := []struct {
		desc     string
		vars     CredentialVariables
		expected []string
	}{
		{
			desc: "empty vars",
		},
		{
			desc:     "single var",
			vars:     CredentialVariables{UserVarName: "testuser"},
			expected: []string{"PGUSER=testuser"},
		},
		{
			desc: "multiple vars",
			vars: CredentialVariables{
				UserVarName:     "testuser",
				DatabaseVarName: "testdb",
				HostVarName:     "localhost",
			},
			expected: []string{
				"PGUSER=testuser",
				"PGDATABASE=testdb",
				"PGHOST=localhost",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := tt.vars.ToEnvSlice()
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestGetServerAddress(t *testing.T) {
	tests := []struct {
		desc     string
		creds    Credentials
		expected string
	}{
		{
			desc: "default port",
			creds: EnvironmentCredentials{
				HostVarName: "localhost",
			},
			expected: "localhost:5432",
		},
		{
			desc: "custom port",
			creds: EnvironmentCredentials{
				HostVarName: "localhost",
				PortVarName: "5433",
			},
			expected: "localhost:5433",
		},
		{
			desc: "empty host",
			creds: EnvironmentCredentials{
				PortVarName: "5433",
			},
			expected: ":5433",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := GetServerAddress(tt.creds)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetVariables(t *testing.T) {
	tests := []struct {
		desc     string
		creds    EnvironmentCredentials
		expected CredentialVariables
	}{
		{
			desc:     "empty credentials",
			creds:    EnvironmentCredentials{},
			expected: CredentialVariables{},
		},
		{
			desc: "single credential",
			creds: EnvironmentCredentials{
				UserVarName: "testuser",
			},
			expected: CredentialVariables{
				UserVarName: "testuser",
			},
		},
		{
			desc: "multiple credentials",
			creds: EnvironmentCredentials{
				UserVarName:     "testuser",
				DatabaseVarName: "testdb",
				HostVarName:     "localhost",
			},
			expected: CredentialVariables{
				UserVarName:     "testuser",
				DatabaseVarName: "testdb",
				HostVarName:     "localhost",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := tt.creds.GetVariables()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUsername(t *testing.T) {
	tests := []struct {
		desc     string
		creds    EnvironmentCredentials
		expected string
	}{
		{
			desc:     "username set",
			creds:    EnvironmentCredentials{UserVarName: "testuser"},
			expected: "testuser",
		},
		{
			desc:     "username not set",
			creds:    EnvironmentCredentials{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := tt.creds.GetUsername()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetHost(t *testing.T) {
	tests := []struct {
		desc     string
		creds    EnvironmentCredentials
		expected string
	}{
		{
			desc:     "host set",
			creds:    EnvironmentCredentials{HostVarName: "localhost"},
			expected: "localhost",
		},
		{
			desc:     "host not set",
			creds:    EnvironmentCredentials{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := tt.creds.GetHost()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPort(t *testing.T) {
	tests := []struct {
		desc     string
		creds    EnvironmentCredentials
		expected string
	}{
		{
			desc:     "port set",
			creds:    EnvironmentCredentials{PortVarName: "5433"},
			expected: "5433",
		},
		{
			desc:     "port not set",
			creds:    EnvironmentCredentials{},
			expected: PostgresDefaultPort,
		},
		{
			desc:     "port set to empty string",
			creds:    EnvironmentCredentials{PortVarName: ""},
			expected: PostgresDefaultPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := tt.creds.GetPort()
			require.Equal(t, tt.expected, result)
		})
	}
}
