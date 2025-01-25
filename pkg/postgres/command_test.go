package postgres

import (
	"io"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func absPathCommand() *exec.Cmd {
	return exec.Command("/some/program", "/some/filepath")
}

func instantReturnCommand() *exec.Cmd {
	// Run a command that should exist on all Linux systems,
	// does effectively nothing, doesn't return an error,
	// and is unlikely to be a shell built-in.
	return exec.Command("cat", "/dev/null")
}

func runShortlyCommand() *exec.Cmd {
	return exec.Command("sleep", "0.01")
}

func TestNewCmdWrapper(t *testing.T) {
	cmd := absPathCommand()
	wrapper := NewCmdWrapper(cmd)

	require.NotNil(t, wrapper)
	require.Equal(t, cmd, wrapper.Cmd)
}

func TestCmdWrapperStdoutPipe(t *testing.T) {
	tests := []struct {
		desc               string
		stdoutPipeCallback func(*cmdWrapper) (io.ReadCloser, error)
		errFunc            require.ErrorAssertionFunc
	}{
		{
			desc: "callback is nil",
		},
		{
			desc: "callback is not nil",
			stdoutPipeCallback: func(cw *cmdWrapper) (io.ReadCloser, error) {
				return nil, nil
			},
		},
		{
			desc: "callback returns error",
			stdoutPipeCallback: func(cw *cmdWrapper) (io.ReadCloser, error) {
				return nil, exec.ErrNotFound
			},
			errFunc: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = require.NoError
			}

			cmd := instantReturnCommand()
			wrapper := NewCmdWrapper(cmd)
			wrapper.stdoutPipeCallback = tt.stdoutPipeCallback

			_, err := wrapper.StdoutPipe()

			tt.errFunc(t, err)
		})
	}
}

func TestCmdWrapperStart(t *testing.T) {
	tests := []struct {
		desc          string
		startCallback func(*cmdWrapper) error
		errFunc       require.ErrorAssertionFunc
	}{
		{
			desc: "callback is nil",
		},
		{
			desc: "callback is not nil",
			startCallback: func(cw *cmdWrapper) error {
				return nil
			},
		},
		{
			desc: "callback returns error",
			startCallback: func(cw *cmdWrapper) error {
				return exec.ErrNotFound
			},
			errFunc: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = require.NoError
			}

			cmd := instantReturnCommand()
			wrapper := NewCmdWrapper(cmd)
			wrapper.startCallback = tt.startCallback

			err := wrapper.Start()

			tt.errFunc(t, err)
		})
	}
}

func TestCmdWrapperWait(t *testing.T) {
	tests := []struct {
		desc         string
		waitCallback func(*cmdWrapper) error
		errFunc      require.ErrorAssertionFunc
	}{
		{
			desc: "callback is nil",
		},
		{
			desc: "callback is not nil",
			waitCallback: func(cw *cmdWrapper) error {
				return nil
			},
		},
		{
			desc: "callback returns error",
			waitCallback: func(cw *cmdWrapper) error {
				return exec.ErrNotFound
			},
			errFunc: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = require.NoError
			}

			cmd := runShortlyCommand()
			wrapper := NewCmdWrapper(cmd)
			wrapper.waitCallback = tt.waitCallback

			err := wrapper.Start()
			require.NoError(t, err)

			t.Cleanup(func() {
				// Kill the process and wait for it to exit, if it hasn't already
				// been killed.
				if wrapper.ProcessState != nil && !wrapper.ProcessState.Exited() {
					err := wrapper.Process.Kill()
					require.NoError(t, err)
					_, err = wrapper.Process.Wait()
					require.NoError(t, err)
				}
			})

			err = wrapper.Wait()

			tt.errFunc(t, err)
		})
	}
}
