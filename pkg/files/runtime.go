package files

import "github.com/solidDoWant/backup-tool/pkg/contexts"

// SyncFilesOptions are the optional parameters for a file sync.
type SyncFilesOptions struct {
	// Filter selects which files are transferred (a whitelist/blacklist). The zero value transfers
	// everything.
	Filter FileFilter
}

// Represents a place (i.e. local or remote) where commands can run.
type Runtime interface {
	CopyFiles(ctx *contexts.Context, src, dest string) error
	SyncFiles(ctx *contexts.Context, src, dest string, opts SyncFilesOptions) error
	ListDirectory(ctx *contexts.Context, path string) ([]string, error)
}

type LocalRuntime struct{}

func NewLocalRuntime() *LocalRuntime {
	return &LocalRuntime{}
}
