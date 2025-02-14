package postgres

import (
	"context"
	"os/exec"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

// The dumps (currently) rely on psql local commands (i.e. `\c`)
const restoreCommandName = "psql"

type RestoreOptions struct{}

func (lr *LocalRuntime) Restore(ctx *contexts.Context, credentials Credentials, inputFilePath string, opts RestoreOptions) (err error) {
	ctx.Log.With("serverAddress", GetServerAddress(credentials), "username", credentials.GetUsername()).Info("Restoring all databases", "inputFilePath", inputFilePath)
	defer ctx.Log.Info("Database restoration complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	// This will cause the process to be terminated if the function returns before the process is done.
	commandCtx, ctxCancel := context.WithCancel(ctx.Child())
	defer ctxCancel()

	cmd := lr.wrapCommand(exec.CommandContext(commandCtx, restoreCommandName, "-X", "-f", inputFilePath))
	cmd.Env = credentials.GetVariables().SetDatabaseName("postgres").ToEnvSlice()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return trace.Wrap(err, "database restoration command failed: %s", string(output))
	}
	return nil
}
