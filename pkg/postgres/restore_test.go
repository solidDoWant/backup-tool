package postgres

import (
	"os/exec"
	"testing"

	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
)

func TestRestore(t *testing.T) {
	tests := []struct {
		desc        string
		shouldError bool
	}{
		{
			desc: "command succeeds",
		},
		{
			desc:        "command fails",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Capture values to verify later
			var funcCmd *exec.Cmd

			lr := &LocalRuntime{
				wrapCommand: func(cmd *exec.Cmd) *cmdWrapper {
					funcCmd = cmd

					cw := NewCmdWrapper(cmd)
					cw.combinedOutputCallback = func(_ *cmdWrapper) ([]byte, error) {
						if tt.shouldError {
							return nil, assert.AnError
						}

						return []byte("test output"), nil
					}

					return cw
				},
			}

			ctx := th.NewTestContext()
			creds := EnvironmentCredentials{
				HostVarName: "fakehost",
				UserVarName: "fakeuser",
			}
			inputFilePath := "test/dump.sql"
			opts := RestoreOptions{}

			err := lr.Restore(ctx, creds, inputFilePath, opts)

			// Verify that the set environment variables match the provided variables even if an error occurs
			assert.Subset(t, funcCmd.Env, []string{"PGHOST=fakehost", "PGUSER=fakeuser", "PGDATABASE=postgres"})

			if tt.shouldError {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
