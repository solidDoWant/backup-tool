package postgres

import (
	"fmt"
	"net"
)

const PostgresDefaultPort = "5432"

type CredentialVariable string

const (
	HostVarName        CredentialVariable = "PGHOST"
	PortVarName        CredentialVariable = "PGPORT"
	DatabaseVarName    CredentialVariable = "PGDATABASE"
	UserVarName        CredentialVariable = "PGUSER"
	RequireAuthVarName CredentialVariable = "PGREQUIREAUTH"
	SSLModeVarName     CredentialVariable = "PGSSLMODE"
	SSLCertVarName     CredentialVariable = "PGSSLCERT"
	SSLKeyVarName      CredentialVariable = "PGSSLKEY"
	SSLRootCertVarName CredentialVariable = "PGSSLROOTCERT"
)

type CredentialVariables map[CredentialVariable]string

func (cv CredentialVariables) SetDatabaseName(name string) CredentialVariables {
	if cv == nil {
		cv = make(CredentialVariables, 1)
	}

	cv[DatabaseVarName] = name
	return cv
}

func (cv CredentialVariables) ToEnvSlice() []string {
	vars := make([]string, 0, len(cv))
	for name, val := range cv {
		vars = append(vars, fmt.Sprintf("%s=%s", name, val))
	}

	return vars
}

type Credentials interface {
	GetVariables() CredentialVariables
	GetUsername() string
	GetHost() string
	GetPort() string
}

func GetServerAddress(creds Credentials) string {
	return net.JoinHostPort(creds.GetHost(), creds.GetPort())
}

type EnvironmentCredentials CredentialVariables

func (ec EnvironmentCredentials) GetVariables() CredentialVariables {
	return CredentialVariables(ec)
}

func (ec EnvironmentCredentials) GetUsername() string {
	return ec[UserVarName]
}

func (ec EnvironmentCredentials) GetHost() string {
	return ec[HostVarName]
}

func (ec EnvironmentCredentials) GetPort() string {
	if port, ok := ec[PortVarName]; ok && port != "" {
		return port
	}

	return PostgresDefaultPort
}
