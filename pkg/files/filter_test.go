package files

import (
	"os"
	"path/filepath"
	"testing"

	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/require"
)

func globs(patterns ...string) []FilePattern {
	out := make([]FilePattern, len(patterns))
	for i, p := range patterns {
		out[i] = FilePattern{Glob: p}
	}
	return out
}

func TestFileFilterIsZero(t *testing.T) {
	require.True(t, FileFilter{}.IsZero())
	require.False(t, FileFilter{Include: globs("*")}.IsZero())
	require.False(t, FileFilter{Exclude: globs("*")}.IsZero())
}

func TestFileFilterValidate(t *testing.T) {
	require.NoError(t, FileFilter{}.Validate())
	require.NoError(t, FileFilter{Include: globs("*.db", "data/*"), Exclude: globs("*.tmp")}.Validate())
	require.Error(t, FileFilter{Include: globs("[bad")}.Validate())
	require.Error(t, FileFilter{Exclude: globs("[bad")}.Validate())
	// A pattern object with no matcher set is a misconfiguration.
	require.Error(t, FileFilter{Include: []FilePattern{{}}}.Validate())
}

func TestFileFilterShouldTransfer(t *testing.T) {
	tests := []struct {
		desc    string
		filter  FileFilter
		relPath string
		isDir   bool
		want    bool
	}{
		{desc: "empty filter transfers files", filter: FileFilter{}, relPath: "a.txt", want: true},
		{desc: "empty filter transfers dirs", filter: FileFilter{}, relPath: "sub", isDir: true, want: true},

		// Patterns are anchored at the sync root: "*" stays within a segment.
		{desc: "anchored star matches top-level file", filter: FileFilter{Exclude: globs("*.tmp")}, relPath: "c.tmp", want: false},
		{desc: "anchored star does not cross directories", filter: FileFilter{Exclude: globs("*.tmp")}, relPath: "a/b/c.tmp", want: true},
		{desc: "anchored dir name matches only at root", filter: FileFilter{Exclude: globs("cache")}, relPath: "cache", isDir: true, want: false},
		{desc: "anchored dir name does not match nested", filter: FileFilter{Exclude: globs("cache")}, relPath: "app/cache", isDir: true, want: true},

		// "**" crosses any number of segments (including none).
		{desc: "doublestar matches at any depth", filter: FileFilter{Exclude: globs("**/*.tmp")}, relPath: "a/b/c.tmp", want: false},
		{desc: "doublestar also matches at root", filter: FileFilter{Exclude: globs("**/*.tmp")}, relPath: "c.tmp", want: false},
		{desc: "doublestar dir match at any depth", filter: FileFilter{Exclude: globs("**/cache")}, relPath: "app/cache", isDir: true, want: false},
		{desc: "doublestar leaves non-matching files", filter: FileFilter{Exclude: globs("**/*.tmp")}, relPath: "a/b/c.db", want: true},
		{desc: "trailing doublestar matches subtree contents", filter: FileFilter{Exclude: globs("cache/**")}, relPath: "cache/deep/x", want: false},

		// Include (whitelist) behaviour.
		{desc: "include whitelists matching file", filter: FileFilter{Include: globs("**/*.db")}, relPath: "data/x.db", want: true},
		{desc: "include drops non-matching file", filter: FileFilter{Include: globs("**/*.db")}, relPath: "data/x.txt", want: false},
		{desc: "include always retains directories so descendants stay reachable", filter: FileFilter{Include: globs("**/*.db")}, relPath: "data", isDir: true, want: true},

		// Exclude wins over include.
		{desc: "exclude wins over include for files", filter: FileFilter{Include: globs("**/*.db"), Exclude: globs("**/secret.db")}, relPath: "secret.db", want: false},
		{desc: "exclude wins over include for dirs", filter: FileFilter{Include: globs("**/*.db"), Exclude: globs("cache")}, relPath: "cache", isDir: true, want: false},

		// Single-segment "*" within an anchored path.
		{desc: "anchored path pattern matches direct child", filter: FileFilter{Include: globs("data/*")}, relPath: "data/x.txt", want: true},
		{desc: "anchored path pattern does not match deeper", filter: FileFilter{Include: globs("data/*")}, relPath: "data/nested/x.txt", want: false},
	}

	for _, tC := range tests {
		t.Run(tC.desc, func(t *testing.T) {
			// Exercise both separators to confirm matching is separator-agnostic.
			require.Equal(t, tC.want, tC.filter.shouldTransfer(filepath.FromSlash(tC.relPath), tC.isDir))
		})
	}
}

// writeFile creates a file (and any parent directories) with the given relative path under root.
func writeFile(t *testing.T, root, relPath string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
	require.NoError(t, os.WriteFile(full, []byte("contents"), 0644))
}

func TestSyncFilesFilter(t *testing.T) {
	tests := []struct {
		desc        string
		srcFiles    []string
		destFiles   []string // pre-existing destination entries, to test pruning
		filter      FileFilter
		wantPresent []string
		wantAbsent  []string
	}{
		{
			desc:        "doublestar exclude omits matching files at any depth",
			srcFiles:    []string{"keep.db", "drop.tmp", "nested/also.tmp", "nested/keep.txt"},
			filter:      FileFilter{Exclude: globs("**/*.tmp")},
			wantPresent: []string{"keep.db", "nested/keep.txt"},
			wantAbsent:  []string{"drop.tmp", "nested/also.tmp"},
		},
		{
			desc:        "anchored exclude only omits top-level match",
			srcFiles:    []string{"drop.tmp", "nested/keep.tmp"},
			filter:      FileFilter{Exclude: globs("*.tmp")},
			wantPresent: []string{"nested/keep.tmp"},
			wantAbsent:  []string{"drop.tmp"},
		},
		{
			desc:        "include keeps only whitelisted files",
			srcFiles:    []string{"data/a.db", "data/b.db", "data/c.txt", "other.txt"},
			filter:      FileFilter{Include: globs("**/*.db")},
			wantPresent: []string{"data/a.db", "data/b.db"},
			wantAbsent:  []string{"data/c.txt", "other.txt"},
		},
		{
			desc:        "exclude prunes whole directory subtree",
			srcFiles:    []string{"keep/a.txt", "cache/b.txt", "cache/deep/c.txt"},
			filter:      FileFilter{Exclude: globs("cache")},
			wantPresent: []string{"keep/a.txt"},
			wantAbsent:  []string{"cache/b.txt", "cache/deep/c.txt"},
		},
		{
			desc:        "newly-excluded file already in destination is pruned",
			srcFiles:    []string{"keep.db", "stale.tmp"},
			destFiles:   []string{"keep.db", "stale.tmp"},
			filter:      FileFilter{Exclude: globs("*.tmp")},
			wantPresent: []string{"keep.db"},
			wantAbsent:  []string{"stale.tmp"},
		},
	}

	for _, tC := range tests {
		t.Run(tC.desc, func(t *testing.T) {
			src := t.TempDir()
			dest := t.TempDir()
			for _, f := range tC.srcFiles {
				writeFile(t, src, f)
			}
			for _, f := range tC.destFiles {
				writeFile(t, dest, f)
			}

			runtime := NewLocalRuntime()
			require.NoError(t, runtime.SyncFiles(th.NewTestContext(), src, dest, SyncFilesOptions{Filter: tC.filter}))

			for _, f := range tC.wantPresent {
				require.FileExists(t, filepath.Join(dest, filepath.FromSlash(f)), "expected %q to be present", f)
			}
			for _, f := range tC.wantAbsent {
				require.NoFileExists(t, filepath.Join(dest, filepath.FromSlash(f)), "expected %q to be absent", f)
			}
		})
	}
}
