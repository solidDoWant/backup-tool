package postgres

import (
	"io"
	"os/exec"
)

// Wrapper around exec.Cmd that allows for easier testing.
// Callbacks can write to the command's stdin, stdout, and stderr, and can be used to simulate
// the command being started, canceled, and waited on.
// TODO move this to another package when/if other packages need to run commands.
type cmdWrapper struct {
	*exec.Cmd
	stdoutPipeCallback     func(*cmdWrapper) (io.ReadCloser, error)
	startCallback          func(*cmdWrapper) error
	waitCallback           func(*cmdWrapper) error
	combinedOutputCallback func(*cmdWrapper) ([]byte, error)
}

func NewCmdWrapper(cmd *exec.Cmd) *cmdWrapper {
	return &cmdWrapper{Cmd: cmd}
}

func (cw *cmdWrapper) StdoutPipe() (io.ReadCloser, error) {
	if cw.stdoutPipeCallback != nil {
		return cw.stdoutPipeCallback(cw)
	}
	return cw.Cmd.StdoutPipe()
}

func (cw *cmdWrapper) Start() error {
	if cw.startCallback != nil {
		return cw.startCallback(cw)
	}
	return cw.Cmd.Start()
}

func (cw *cmdWrapper) Wait() error {
	if cw.waitCallback != nil {
		return cw.waitCallback(cw)
	}
	return cw.Cmd.Wait()
}

func (cw *cmdWrapper) CombinedOutput() ([]byte, error) {
	if cw.combinedOutputCallback != nil {
		return cw.combinedOutputCallback(cw)
	}
	return cw.Cmd.CombinedOutput()
}
