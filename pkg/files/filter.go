package files

import (
	"path/filepath"
	"slices"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/gravitational/trace"
)

// FilePattern matches file paths within a sync. Each pattern selects its matcher by which field is
// set; currently the only matcher is Glob. Modelling a pattern as an object (rather than a bare string)
// leaves room to add alternative matchers later (e.g. a Regex field) without changing the config shape.
type FilePattern struct {
	// Glob is a doublestar glob (https://pkg.go.dev/github.com/bmatcuk/doublestar/v4#Match), matched
	// against the entry's slash-separated path relative to the sync root and anchored there. "*" matches
	// within a single path segment; "**" matches across segments (any depth, including none). So "logs"
	// matches only a top-level "logs", "**/logs" matches a "logs" at any depth, "**/*.tmp" matches ".tmp"
	// files anywhere, and "cache/**" matches everything under a top-level "cache".
	Glob string `yaml:"glob,omitempty"`
}

// Validate reports whether the pattern is well-formed: exactly one matcher must be set, and it must be
// syntactically valid.
func (p FilePattern) Validate() error {
	if p.Glob == "" {
		return trace.BadParameter("a file filter pattern must specify a glob")
	}

	if !doublestar.ValidatePattern(p.Glob) {
		return trace.BadParameter("invalid glob pattern %q", p.Glob)
	}

	return nil
}

// matches reports whether the pattern matches slashPath (a slash-separated path). The pattern is tested
// against both the full relative path and the base name. A pattern with no matcher set never matches.
func (p FilePattern) matches(slashPath string) bool {
	if p.Glob == "" {
		return false
	}

	// doublestar.Match errors are pattern-syntax errors, surfaced eagerly by Validate; treat a malformed
	// pattern as a non-match here so a bad pattern never silently drops files. The pattern is anchored at
	// the sync root: "*" matches within a path segment, "**" matches across segments (any depth).
	matched, _ := doublestar.Match(p.Glob, slashPath)
	return matched
}

// FileFilter selects which files within a sync are transferred (a whitelist/blacklist).
//
// Include is a whitelist: when non-empty, only files matching at least one Include pattern are
// transferred. Exclude is a blacklist: any entry matching an Exclude pattern is omitted, and for a
// directory its whole subtree is pruned. Exclude takes precedence over Include.
//
// Directories are always traversed when only Include patterns are set, so a whitelist of "data/**/*.db"
// still reaches the nested files (intermediate directories may be left empty). A zero FileFilter (no
// patterns) transfers everything.
type FileFilter struct {
	Include []FilePattern `yaml:"include,omitempty"`
	Exclude []FilePattern `yaml:"exclude,omitempty"`
}

// IsZero reports whether the filter constrains nothing (transfers everything).
func (f FileFilter) IsZero() bool {
	return len(f.Include) == 0 && len(f.Exclude) == 0
}

// Validate reports whether every Include/Exclude pattern is well-formed.
func (f FileFilter) Validate() error {
	patterns := append(append([]FilePattern(nil), f.Include...), f.Exclude...)

	errors := make([]error, 0, len(patterns))
	for _, pattern := range patterns {
		if err := pattern.Validate(); err != nil {
			errors = append(errors, trace.Wrap(err))
		}
	}

	return trace.Wrap(trace.NewAggregate(errors...), "failed to validate file filter patterns")
}

// shouldTransfer reports whether the entry at relPath (relative to the sync root, OS-separated)
// should be transferred. Directories are always retained when only Include patterns are set so that
// matching descendants stay reachable; an Exclude match still prunes a directory subtree.
func (f FileFilter) shouldTransfer(relPath string, isDir bool) bool {
	slashPath := filepath.ToSlash(relPath)
	if matchesAnyPattern(f.Exclude, slashPath) {
		return false
	}

	if len(f.Include) == 0 || isDir {
		return true
	}

	return matchesAnyPattern(f.Include, slashPath)
}

// matchesAnyPattern reports whether slashPath matches any of the patterns.
func matchesAnyPattern(patterns []FilePattern, slashPath string) bool {
	return slices.ContainsFunc(patterns, func(pattern FilePattern) bool {
		return pattern.matches(slashPath)
	})
}
