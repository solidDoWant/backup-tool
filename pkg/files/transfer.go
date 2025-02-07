package files

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"
	cp "github.com/otiai10/copy"
)

// Copies the filesytem object (file, directory, etc.) to the destination path.
// Special files (such as sockets or device files) are not included.
func (*LocalRuntime) CopyFiles(_ context.Context, src, dest string) error {
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
func (lr *LocalRuntime) SyncFiles(ctx context.Context, src, dest string) error {
	if err := validateSrcDest(src, dest); err != nil {
		return err
	}

	// If the source is not a directory, or destination does not exist yet, don't attempt to walk over the destination
	srcFileInfo, err := os.Lstat(src)
	if err != nil {
		return trace.Wrap(err, "failed to get file info for %q", src)
	}

	_, err = os.Lstat(dest)
	if err != nil && !os.IsNotExist(err) {
		return trace.Wrap(err, "failed to get file info for %q", dest)
	}

	if srcFileInfo.IsDir() && err == nil {
		// Delete all files that don't exist in the source
		err := filepath.WalkDir(dest, func(pathInDest string, d fs.DirEntry, err error) error {
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
				return nil
			}

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
		if err != nil {
			return trace.Wrap(err, "failed while walking over destination directory %q for files to delete", dest)
		}
	}

	// Copy all files
	return lr.CopyFiles(ctx, src, dest)
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
