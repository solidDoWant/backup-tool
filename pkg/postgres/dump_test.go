package postgres

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIgnoreLines(t *testing.T) {
	// Require no duplicates
	require.Equal(t, lo.Uniq(ignoreLines), ignoreLines)
}

// Why doesn't Go include this in the io package?
type nopWriterCloser struct {
	io.Writer
}

func (*nopWriterCloser) Close() error { return nil }

func TestDumpAll(t *testing.T) {
	tests := []struct {
		name                   string
		stdout                 string
		stderr                 string
		expectedOutputContents string
		loadStdoutFromFile     bool
		shouldStdoutPipeError  bool
		shouldStartError       bool
	}{
		{
			name: "command succeeds, but provides no output",
		},
		{
			name:                   "command succeeds, and provides some output to stdout",
			stdout:                 "some pgdump stdout data",
			expectedOutputContents: "some pgdump stdout data",
		},
		{
			name:   "command succeeds, and provides some output to stderr",
			stderr: "some pgdump stderr data",
		},
		{
			name:                   "command succeeds, and provides some output to both stdout and stderr",
			stdout:                 "some pgdump stdout data",
			stderr:                 "some pgdump stderr data",
			expectedOutputContents: "some pgdump stdout data",
		},
		{
			name:                   "command succeeds with the output ending in a newline",
			stdout:                 "some pgdump stdout data\n",
			expectedOutputContents: "some pgdump stdout data\n",
		},
		{
			name:                   "command succeeds with the output ending in multiple newlines",
			stdout:                 "some pgdump stdout data\n\n\n",
			expectedOutputContents: "some pgdump stdout data\n\n\n",
		},
		{
			name:                   "command succeeds with a single long output line",
			stdout:                 strings.Repeat("some long string", 10000),
			expectedOutputContents: strings.Repeat("some long string", 10000),
		},
		{
			name:                   "command succeeds, with the multiple output lines",
			stdout:                 "some pgdump stdout data\nsome pgdump stdout data\nsome pgdump stdout data\n",
			expectedOutputContents: "some pgdump stdout data\nsome pgdump stdout data\nsome pgdump stdout data\n",
		},
		{
			name:                   "command succeeds with the real-world testdata",
			stdout:                 "pgdump_stdout",
			expectedOutputContents: "pgdump_stdout_expected",
			loadStdoutFromFile:     true,
		},
		{
			name:                  "fail to get stdout stream",
			shouldStdoutPipeError: true,
		},
		{
			name:             "fail to start command",
			shouldStartError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a context for signaling when the process should pretend to be complete
			ctx := context.Background()
			processCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Capture values to verify later
			var capturedStderr bytes.Buffer
			var funcCmd *exec.Cmd

			// Override the wrapCommand function to inject custom stdout/stderr behavior,
			// and to capture the command itself
			lr := &LocalRuntime{
				errOutputWriter: &nopWriterCloser{&capturedStderr}, // Capture stderr
				wrapCommand: func(cmd *exec.Cmd) *cmdWrapper {
					funcCmd = cmd

					// Overridden to inject custom stdout/stderr
					cw := NewCmdWrapper(cmd)
					cw.stdoutPipeCallback = func(cw *cmdWrapper) (io.ReadCloser, error) {
						if tt.shouldStdoutPipeError {
							return nil, assert.AnError
						}
						return cw.Cmd.StdoutPipe()
					}
					cw.startCallback = func(cw *cmdWrapper) error {
						if tt.shouldStartError {
							return assert.AnError
						}

						// Stream the stdout and stderr data for the test case to the command's stdout
						testDataToWriter(processCtx, t, tt.stdout, tt.loadStdoutFromFile, cw.Stdout)
						testDataToWriter(processCtx, t, tt.stderr, false, cw.Stderr)

						go func() {
							// Wait long enough for the data to be written and read, then cancel the context
							// which signals that the process has completed.
							time.Sleep(100 * time.Millisecond)
							cancel()
						}()
						return nil
					}
					cw.waitCallback = func(cw *cmdWrapper) error {
						cancel() // Signals that the process has completed
						return nil
					}
					return cw
				},
			}

			creds := EnvironmentCredentials{
				HostVarName: "fakehost",
				UserVarName: "fakeuser",
			}

			outputFilePath := filepath.Join(t.TempDir(), "test_output.sql")

			funcErr := lr.DumpAll(ctx, creds, outputFilePath, DumpAllOptions{})

			// Verify that the set environment variables match the provided variables even if an error occurs
			require.Subset(t, funcCmd.Env, []string{"PGHOST=fakehost", "PGUSER=fakeuser", "PGDATABASE=postgres"})

			// Verify the error output
			// This test doesn't do much right now, but if/when the stderr logic is replaced with a logging library,
			// this should help catch any regressions.
			if !tt.loadStdoutFromFile {
				require.Equal(t, capturedStderr.String(), tt.stderr)
			}

			if tt.shouldStdoutPipeError || tt.shouldStartError {
				require.Error(t, funcErr)
				return
			}

			require.NoError(t, funcErr)

			// Verify the contents of the output file
			expectedOutputContents := tt.expectedOutputContents
			if tt.loadStdoutFromFile {
				expectedOutputContentsRaw, err := os.ReadFile(filepath.Join("testdata", tt.expectedOutputContents))
				require.NoError(t, err)
				expectedOutputContents = string(expectedOutputContentsRaw)
			}
			outputFileContents, err := os.ReadFile(outputFilePath)
			require.NoError(t, err)
			require.Equal(t, expectedOutputContents, string(outputFileContents))
		})
	}
}

// The underlying implementation of stdout/stderr uses separate readers and writers attached to different ends of an os.Pipe.
// To mock the actual exec.Cmd logic, both the reader and writer must be closed when the process is complete. But first, all
// data must be written to the writer. This function handles that process.
func testDataToWriter(ctx context.Context, t *testing.T, contents string, contentsAreFilePath bool, writer io.Writer) {
	require.Implements(t, (*io.WriteCloser)(nil), writer)

	var contentsReader io.ReadCloser
	if contentsAreFilePath {
		contentsFile, err := os.Open(filepath.Join("testdata", contents))
		require.NoError(t, err)
		contentsReader = contentsFile
	} else {
		contentsReader = io.NopCloser(strings.NewReader(contents))
	}

	go func() {
		_, err := io.Copy(writer, contentsReader)
		require.NoError(t, err)

		// Wait for the close signal to close the reader.
		// This is required to synchronize the closing of the reader.
		<-ctx.Done()
		err = writer.(io.WriteCloser).Close()
		require.NoError(t, err)

		defer contentsReader.Close()
	}()
}
