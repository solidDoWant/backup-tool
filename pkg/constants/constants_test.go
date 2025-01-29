package constants

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
)

// TODO figure out a way to run this on built binaries
func TestImageVersion(t *testing.T) {
	// Some idiot decided to make the official golang semver library require a "v" prefix
	// despite this being explicitly stated as not compliant in the semver spec
	// (https://semver.org/#is-v123-a-semantic-version). Work around it by adding the prefix.
	// If a prefix already exists, then IsValid/similar functions will fail, which is intended here.
	prefixSemver := "v" + ImageTag
	require.True(t, semver.IsValid(prefixSemver))
	require.Equal(t, semver.Canonical(prefixSemver), prefixSemver)
}
