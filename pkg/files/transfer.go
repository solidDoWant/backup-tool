package files

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"
	cp "github.com/otiai10/copy"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
)

// Copies the filesytem object (file, directory, etc.) to the destination path.
// Special files (such as sockets or device files) are not included.
func (*LocalRuntime) CopyFiles(ctx *contexts.Context, src, dest string) (err error) {
	ctx.Log.With("src", src, "dest", dest).Info("Copying files")
	defer ctx.Log.Info("Finished copying files", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if err := validateSrcDest(src, dest); err != nil {
		return err
	}

	// If the source is not a directory, copy it to <dest>/<file name> or <dest>
	// depending on whether or not dest points to a directory
	fileInfo, err := os.Lstat(src)
	if err != nil {
		return trace.Wrap(err, "failed to get file info for source path %q", src)
	}
	if !fileInfo.IsDir() {
		isDestDir := false
		destFileInfo, err := os.Lstat(dest)
		if err != nil {
			if !os.IsNotExist(err) {
				return trace.Wrap(err, "failed to get file info for destination path %q", dest)
			}

			// If the destination doesn't already exist, assume that it is a directory if it ends
			// in the pathe separator character (i.e. '/')
			isDestDir = dest[len(dest)-1] == filepath.Separator
		} else {
			isDestDir = destFileInfo.IsDir()
		}

		if isDestDir {
			dest = filepath.Join(dest, filepath.Base(src))
		}
	}

	err = cp.Copy(src, dest, cp.Options{
		// Copy the symlink exactly as is
		OnSymlink: func(src string) cp.SymlinkAction {
			return cp.Shallow
		},
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			return cp.Merge
		},
		// Don't copy special files
		Specials:          false,
		Sync:              true,
		PermissionControl: cp.PerservePermission,
		PreserveTimes:     true,
		PreserveOwner:     true,
	})

	return trace.Wrap(err, "failed to copy files from %q to %q", src, dest)
}

// Make the destination path contents match the input directory contents.
// Special files (such as sockets or device files) are not included.
func (lr *LocalRuntime) SyncFiles(ctx *contexts.Context, src, dest string) (err error) {
	ctx.Log.With("src", src, "dest", dest).Info("Syncing files")
	defer ctx.Log.Info("Finished syncing files", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if err := validateSrcDest(src, dest); err != nil {
		return err
	}

	if err := deleteMissingFiles(ctx.Child(), src, dest); err != nil {
		return trace.Wrap(err, "failed to delete missing files from %q in %q", dest, src)
	}

	// Copy all files
	return lr.CopyFiles(ctx.Child(), src, dest)
}

func deleteMissingFiles(ctx *contexts.Context, src, dest string) error {
	ctx.Log.Info("Deleting files in the destination that are missing from the source")
	defer ctx.Log.Info("Finished deleting files")

	// If the source is not a directory, or destination does not exist yet, don't attempt to walk over the destination
	srcFileInfo, err := os.Lstat(src)
	if err != nil {
		return trace.Wrap(err, "failed to get file info for %q", src)
	}

	if !srcFileInfo.IsDir() {
		return nil
	}

	_, err = os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return trace.Wrap(err, "failed to get file info for %q", dest)
	}

	// Delete all files that don't exist in the source
	walkerCtx := ctx.Child()
	err = filepath.WalkDir(dest, func(pathInDest string, d fs.DirEntry, err error) error {
		walkerCtx.Log.Debug("Checking path", "path", pathInDest, "type", d.Type().String()[:1])

		if err != nil {
			return trace.Wrap(err, "failed to walk over path %q", pathInDest)
		}

		relativePath, err := filepath.Rel(dest, pathInDest)
		if err != nil {
			return trace.Wrap(err, "failed to get file path %q relative to %q", pathInDest, dest)
		}

		pathInSrc := filepath.Join(src, relativePath)

		_, err = os.Lstat(pathInSrc)
		if err == nil {
			// File exists in the source, no need to rm it from destination
			walkerCtx.Log.Debug("File exists in source, skipping", "path", pathInSrc)
			return nil
		}
		walkerCtx.Log.Debug("File does not exist in source, removing from destination", "path", pathInDest)

		if !os.IsNotExist(err) {
			// File may or may not exist, but another error was thrown
			return trace.Wrap(err, "failed to lstat %q", pathInSrc)
		}

		// File does not exist in the source, so it should not exist in the destination and must be removed
		err = os.RemoveAll(pathInDest)
		if err != nil {
			return trace.Wrap(err, "failed to remove path %q from the destination path, which does not exist in the source path", pathInDest)
		}

		// If the deleted item was a directory, child items don't need to be (and should not be) processed
		if d.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})
	return trace.Wrap(err, "failed while walking over destination directory %q for files to delete", dest)
}

func validateSrcDest(src, dest string) error {
	src = strings.TrimSpace(src)
	if src == "" {
		return trace.Errorf("no source path provided")
	}

	dest = strings.TrimSpace(dest)
	if dest == "" {
		return trace.Errorf("no destination path provided")
	}

	return nil
}
