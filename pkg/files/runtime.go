package files

import "github.com/solidDoWant/backup-tool/pkg/contexts"

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	CopyFiles(ctx *contexts.Context, src, dest string) error
	SyncFiles(ctx *contexts.Context, src, dest string) error
}

type LocalRuntime struct{}

func NewLocalRuntime() *LocalRuntime {
	return &LocalRuntime{}
}
