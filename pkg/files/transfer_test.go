package files

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"

	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/require"
)

func testFilePath(dir string) string {
	const testFileName = "testfile"
	if filepath.Base(dir) == testFileName {
		// Assume that "directories" named after the testfile are meant to be the actual test file
		return dir
	}

	return filepath.Join(dir, testFileName)
}

func setupSrcTestFileEmpty(t *testing.T, src, dest string) {
	setupTestFileEmpty(t, src)
}

func setupTestFileEmpty(t *testing.T, dir string) {
	setupTestFileWithContents(t, dir, "")
}

func setupSrcTestFileWithContents(t *testing.T, src, dest string) {
	setupTestFileWithContents(t, src, "test contents")
}

func setupTestFileWithContents(t *testing.T, dir, contents string) {
	oldUmask := syscall.Umask(0)
	f, err := os.OpenFile(testFilePath(dir), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0765)
	syscall.Umask(oldUmask)
	require.NoError(t, err)
	if contents != "" {
		_, err = f.WriteString(contents)
		require.NoError(t, err)
	}
	require.NoError(t, f.Close())
}

func verifyTestFile(t *testing.T, src, dest string) {
	// Check exists
	require.FileExists(t, testFilePath(dest))

	// Check file permissions
	fileInfo, err := os.Lstat(testFilePath(dest))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0765), fileInfo.Mode())

	// Check file ownership
	if linuxStat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		currentUser, err := user.Current()
		require.NoError(t, err)

		require.Equal(t, currentUser.Uid, fmt.Sprintf("%d", linuxStat.Uid))
		require.Equal(t, currentUser.Gid, fmt.Sprintf("%d", linuxStat.Gid))
	}

	// Check contents
	srcContents, err := os.ReadFile(testFilePath(src))
	require.NoError(t, err)

	destContents, err := os.ReadFile(testFilePath(dest))
	require.NoError(t, err)

	require.Equal(t, srcContents, destContents)
}

func testDirPath(dir string) string {
	const testDirName = "subdir"
	if filepath.Base(dir) == testDirName {
		return dir
	}

	return filepath.Join(dir, testDirName)
}

func setupSrcTestDir(t *testing.T, src, dest string) {
	oldUmask := syscall.Umask(0)
	err := os.MkdirAll(testDirPath(src), 0765)
	syscall.Umask(oldUmask)
	require.NoError(t, err)
}

func verityTestDir(t *testing.T, src, dest string) {
	subdir := testDirPath(dest)
	require.DirExists(t, subdir)
	fileInfo, err := os.Lstat(subdir)
	require.NoError(t, err)
	require.Equal(t, os.ModeDir|os.FileMode(0765), fileInfo.Mode())
}

func testSymlinkPath(dir string) string {
	const testSymlinkName = "symlink"
	if filepath.Base(dir) == testSymlinkName {
		return dir
	}

	return filepath.Join(dir, testSymlinkName)
}

func setupSrcTestSymlink(t *testing.T, src string, absolute bool) {
	symlinkPath := testSymlinkPath(src)

	target := testFilePath(src)
	if !absolute {
		var err error
		target, err = filepath.Rel(src, target)
		require.NoError(t, err)
	}

	err := os.Symlink(target, symlinkPath)
	require.NoError(t, err)
}

func verifyTestSymlink(t *testing.T, src, dest string, absolute bool) {
	linkPath := testSymlinkPath(dest)
	fileInfo, err := os.Lstat(linkPath)
	require.NoError(t, err)
	require.NotZero(t, fileInfo.Mode()&os.ModeSymlink)

	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	if absolute {
		require.Equal(t, testFilePath(src), target)
	} else {
		expectedTarget, err := filepath.Rel(src, testFilePath(src))
		require.NoError(t, err)
		require.Equal(t, expectedTarget, target)
	}
}

func verifyNotExist(t *testing.T, path string) {
	_, err := os.Lstat(path)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

type TransferTestCase struct {
	desc    string
	src     string
	dest    string
	errFunc require.ErrorAssertionFunc
	setup   func(t *testing.T, src, dest string)
	verify  func(t *testing.T, src, dest string)
}

func transferTestCases(t *testing.T) []TransferTestCase {
	return []TransferTestCase{
		{
			desc:    "empty source path",
			dest:    t.TempDir(),
			errFunc: require.Error,
		},
		{
			desc:    "empty dest path",
			src:     t.TempDir(),
			errFunc: require.Error,
		},
		{
			desc:    "empty src and dest path",
			errFunc: require.Error,
		},
		{
			desc:    "whitespace source path",
			src:     " ",
			dest:    t.TempDir(),
			errFunc: require.Error,
		},
		{
			desc:    "whitespace dest path",
			src:     t.TempDir(),
			dest:    "  ",
			errFunc: require.Error,
		},
		{
			desc:    "whitespace src and dest path",
			src:     "  ",
			dest:    " ",
			errFunc: require.Error,
		},
		{
			desc:   "single file in dir, no contents",
			src:    t.TempDir(),
			dest:   t.TempDir(),
			setup:  setupSrcTestFileEmpty,
			verify: verifyTestFile,
		},
		{
			desc:   "single file in dir, with contents",
			src:    t.TempDir(),
			dest:   t.TempDir(),
			setup:  setupSrcTestFileWithContents,
			verify: verifyTestFile,
		},
		{
			desc:   "single file directly, no contents",
			src:    testFilePath(t.TempDir()),
			dest:   t.TempDir(),
			setup:  setupSrcTestFileEmpty,
			verify: verifyTestFile,
		},
		{
			desc:   "single file directly, with contents",
			src:    testFilePath(t.TempDir()),
			dest:   t.TempDir(),
			setup:  setupSrcTestFileWithContents,
			verify: verifyTestFile,
		},
		{
			desc: "single file directly, dest already exists",
			src:  testFilePath(t.TempDir()),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestFileEmpty(t, src, dest)
				setupTestFileEmpty(t, dest)
			},
			verify: verifyTestFile,
		},
		{
			desc: "single file directly, dest already exists, different contents",
			src:  testFilePath(t.TempDir()),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestFileEmpty(t, src, dest)
				setupSrcTestFileWithContents(t, dest, "different contents")
			},
			verify: verifyTestFile,
		},
		{
			desc:   "source with subdirectory",
			src:    t.TempDir(),
			dest:   t.TempDir(),
			setup:  setupSrcTestDir,
			verify: verityTestDir,
		},
		{
			desc: "source with subdirectory with contents",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestDir(t, src, dest)
				setupSrcTestFileWithContents(t, testDirPath(src), testDirPath(dest))
				setupSrcTestFileWithContents(t, src, dest)
			},
			verify: func(t *testing.T, src, dest string) {
				verityTestDir(t, src, dest)
				verifyTestFile(t, testDirPath(src), testDirPath(dest))
				verifyTestFile(t, src, dest)
			},
		},
		{
			desc: "source with symlink to exisitng sibling file, absolute path",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestFileEmpty(t, src, dest)
				setupSrcTestSymlink(t, src, true)
			},
			verify: func(t *testing.T, src, dest string) {
				verifyTestFile(t, src, dest)
				verifyTestSymlink(t, src, dest, true)
			},
		},
		{
			desc: "source with symlink to exisitng sibling file, relative path",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestFileEmpty(t, src, dest)
				setupSrcTestSymlink(t, src, false)
			},
			verify: func(t *testing.T, src, dest string) {
				verifyTestFile(t, src, dest)
				verifyTestSymlink(t, src, dest, false)
			},
		},
		{
			desc: "source with symlink to nonexistent sibling file",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestSymlink(t, src, false)
			},
			verify: func(t *testing.T, src, dest string) {
				verifyTestSymlink(t, src, dest, false)
			},
		},
	}
}

func TestCopyFiles(t *testing.T) {
	runtime := NewLocalRuntime()

	for _, tC := range transferTestCases(t) {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.errFunc == nil {
				tC.errFunc = require.NoError
			}

			if tC.setup != nil {
				tC.setup(t, tC.src, tC.dest)
			}

			err := runtime.CopyFiles(th.NewTestContext(), tC.src, tC.dest)
			tC.errFunc(t, err)

			if tC.verify != nil {
				tC.verify(t, tC.src, tC.dest)
			}
		})
	}
}

func TestSyncFiles(t *testing.T) {
	runtime := NewLocalRuntime()

	extraTestCases := []TransferTestCase{
		{
			desc: "file in destination but not in source",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestFileWithContents(t, dest, src)
			},
			verify: func(t *testing.T, src, dest string) {
				verifyNotExist(t, testFilePath(dest))
			},
		},
		{
			desc: "dir in destination but not in source",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestDir(t, dest, src)
			},
			verify: func(t *testing.T, src, dest string) {
				verifyNotExist(t, testDirPath(dest))
			},
		},
		{
			desc: "file in subdir in destination but not in source",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestDir(t, src, dest)
				setupSrcTestDir(t, dest, src)
				setupSrcTestFileWithContents(t, dest, src)
			},
			verify: func(t *testing.T, src, dest string) {
				verityTestDir(t, src, dest)
				verifyNotExist(t, testFilePath(dest))
			},
		},
		{
			desc: "symlink in destination but not in source",
			src:  t.TempDir(),
			dest: t.TempDir(),
			setup: func(t *testing.T, src, dest string) {
				setupSrcTestFileWithContents(t, dest, src)
				setupSrcTestSymlink(t, dest, true)
			},
			verify: func(t *testing.T, src, dest string) {
				verifyNotExist(t, testSymlinkPath(dest))
			},
		},
	}

	for _, tC := range append(transferTestCases(t), extraTestCases...) {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.errFunc == nil {
				tC.errFunc = require.NoError
			}

			if tC.setup != nil {
				tC.setup(t, tC.src, tC.dest)
			}

			err := runtime.SyncFiles(th.NewTestContext(), tC.src, tC.dest)
			tC.errFunc(t, err)

			if tC.verify != nil {
				tC.verify(t, tC.src, tC.dest)
			}
		})
	}
}
