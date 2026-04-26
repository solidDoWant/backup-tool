package testhelpers

import "os"

// The fake clientset does not honor the WatchList content-type negotiation that
// client-go's reflector uses when this feature is enabled, so the reflector
// hangs waiting for a synthetic Bookmark event that the fake never emits. Force
// the legacy List+Watch path for any test binary that pulls in this package.
func init() {
	if _, set := os.LookupEnv("KUBE_FEATURE_WatchListClient"); !set {
		_ = os.Setenv("KUBE_FEATURE_WatchListClient", "false")
	}
}
