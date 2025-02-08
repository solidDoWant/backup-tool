package postgres

import (
	"io"
	"os"
	"os/exec"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	DumpAll(*contexts.Context, Credentials, string, DumpAllOptions) error
}

type LocalRuntime struct {
	// Used for tests to mock running commands, and for dep injection
	wrapCommand     func(cmd *exec.Cmd) *cmdWrapper
	errOutputWriter io.WriteCloser
}

func NewLocalRuntime() *LocalRuntime {
	return &LocalRuntime{
		wrapCommand:     NewCmdWrapper,
		errOutputWriter: os.Stderr,
	}
}
