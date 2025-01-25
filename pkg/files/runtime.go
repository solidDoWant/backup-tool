package files

import "context"

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	CopyFiles(ctx context.Context, src, dest string) error
	SyncFiles(ctx context.Context, src, dest string) error
}

type LocalRuntime struct{}

func NewLocalRuntime() *LocalRuntime {
	return &LocalRuntime{}
}
