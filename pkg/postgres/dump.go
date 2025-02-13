package postgres

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

// Depending on a CLI tool is unfortunate but there are no viable golang replacements for this.
const commandName = "pg_dumpall"

// Tracks the _prefixes_ of lines that should be commented out in the produced file.
// This is used to drop problematic queries for DB resources that should be entirely managed by CNPG, such as the postgres role.
// Entries will only be matched once.
var ignoreLines = []string{
	"DROP ROLE IF EXISTS postgres",
	"DROP ROLE IF EXISTS streaming_replica",
	"ALTER ROLE postgres",
	"COMMENT ON ROLE postgres",
	"CREATE ROLE streaming_replica",
	"ALTER ROLE streaming_replica",
	"COMMENT ON ROLE streaming_replica",
}

type writerCloserCallback struct {
	io.Writer
	callback func() error
}

func (wcc *writerCloserCallback) Close() error {
	return wcc.callback()
}

type DumpAllOptions struct {
	CleanupTimeout helpers.MaxWaitTime
}

func (lr *LocalRuntime) DumpAll(ctx *contexts.Context, credentials Credentials, outputFilePath string, opts DumpAllOptions) (err error) {
	ctx.Log.With("serverAddress", GetServerAddress(credentials), "username", credentials.GetUsername()).Info("Dumping all databases", "outputFilePath", outputFilePath)
	defer ctx.Log.Info("Database dump complete", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	// This will cause the process to be terminated if the function returns before the process is done.
	commandCtx, ctxCancel := context.WithCancel(ctx.Child())
	defer ctxCancel()

	cmd := lr.wrapCommand(exec.CommandContext(commandCtx, commandName, "--clean", "--if-exists", "--exclude-database=postgres"))
	cmd.Env = credentials.GetVariables().SetDatabaseName("postgres").ToEnvSlice()

	// Capture stderr and also write it to the standard error output stream.
	var stderrCapture strings.Builder
	cmd.Stderr = &writerCloserCallback{
		Writer:   io.MultiWriter(&stderrCapture, lr.errOutputWriter), // TODO integrate this with logging lib at some point
		callback: lr.errOutputWriter.Close,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return trace.Wrap(err, "failed to get stdout pipe for %q process", commandName)
	}

	outputFile, err := os.OpenFile(outputFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return trace.Wrap(err, "failed to open SQL dump output file %q for writing", outputFilePath)
	}
	outputFileWriter := bufio.NewWriter(outputFile) // This is used to avoid writing to the file one (potentially small) line at a time.
	defer cleanup.To(func(ctx *contexts.Context) error {
		flushErr := outputFileWriter.Flush()
		closeErr := outputFile.Close()
		return trace.NewAggregate(
			trace.Wrap(flushErr, "failed to flush all output data to output file at %q", outputFilePath),
			trace.Wrap(closeErr, "failed to close output file at %q", outputFilePath),
		)
	}).WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(30 * time.Second)).
		Run()

	err = cmd.Start()
	if err != nil {
		return trace.Wrap(err, "failed to start %q process", commandName)
	}

	// The map value indicates if the item has been found yet.
	ignoreLineTracker := make(map[string]bool, len(ignoreLines))
	for _, ignoreLine := range ignoreLines {
		ignoreLineTracker[ignoreLine] = false
	}
	foundIgnoreLineCount := 0 // Used to stop checking every line a little early

	// Track the number of lines processed for logging purposes.
	linesProcessed := 0
	modulus := 1

	// This is a pretty naive implementation and probably isn't very efficient. I'll optimize if runtime
	// starts to become a problem.
	sqlReader := bufio.NewReader(stdout)
	for {
		linesProcessed++
		if linesProcessed%modulus == 0 {
			ctx.Log.Debug("Processed lines", "lineCount", linesProcessed)

			// Every 10 messages, increase the modulus by a factor of 10. This helps keep the log output
			// under control, while still providing some feedback.
			newModulus := modulus * 10
			if linesProcessed%newModulus == 0 {
				modulus = newModulus
			}
		}

		hitEOF := false
		sqlLine, err := sqlReader.ReadString('\n')
		if err == io.EOF {
			// Don't break here, because the last line still needs to be processed.
			hitEOF = true
		}
		if !hitEOF && err != nil {
			return trace.Wrap(err, "failed to read from stdout")
		}

		// Find lines that should be ignored, and comment them out.
		// Assumptions made:
		// * All lines end in '\n' (so have a min length of 1)
		// * No leading whitespace
		// * One "entry" per line, where an entry is one of:
		//   * Blank (no chars, just another new line)
		//   * Comment (starts with --)
		//   * A psql command (starts with \)
		//   * A SQL statement (ends with ;)
		// If not all lines to ignore have been found yet, and
		if foundIgnoreLineCount < len(ignoreLineTracker) &&
			// the current line is not blank, and
			len(sqlLine) >= 2 &&
			// the current line is not a comment, and
			!(sqlLine[0] == '-' && sqlLine[1] == '-') &&
			// the current line is not a psql command, and
			sqlLine[0] != '\\' &&
			// the current line is a statement line
			sqlLine[len(sqlLine)-2] == ';' {
			for ignoreLinePrefix, alreadyFound := range ignoreLineTracker {
				if alreadyFound {
					continue
				}

				if strings.HasPrefix(sqlLine, ignoreLinePrefix) {
					ctx.Log.Debug("Ignoring line", "line", sqlLine)
					ignoreLineTracker[ignoreLinePrefix] = true
					foundIgnoreLineCount++

					sqlLine = "-- Ignored statement: " + sqlLine
					break
				}
			}
		}

		_, err = outputFileWriter.WriteString(sqlLine)
		if err != nil {
			return trace.Wrap(err, "failed to write SQL line %q to output file at %q", outputFilePath)
		}

		if hitEOF {
			break
		}
	}

	// TODO document this or parse and wrap the error
	// Common errors:
	// * authentication method requirement "none" failed: server requested SASL authentication: this usually means that the server isn't expecting client cert auth
	cmdErr := trace.Wrap(cmd.Wait(), "process %q failed with stderr: %s", commandName, stderrCapture.String())
	return trace.NewAggregate(cmdErr, err)
}
