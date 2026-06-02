package s3

import (
	"net/url"
	"strings"

	"github.com/gravitational/trace"
)

// s3Path is a parsed s3:// URL: a bucket and an optional (possibly empty) key prefix with no leading slash.
type s3Path struct {
	bucket string
	prefix string
}

// parseS3Path parses an s3://bucket/prefix URL. It returns isS3=false (and no error) when raw is not an
// s3:// URL, i.e. it is a local filesystem path.
func parseS3Path(raw string) (parsed s3Path, isS3 bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return s3Path{}, false, trace.Wrap(err, "failed to parse %q as a URL", raw)
	}

	if u.Scheme != "s3" {
		return s3Path{}, false, nil
	}

	if u.Host == "" {
		return s3Path{}, false, trace.Errorf("s3 URL %q is missing a bucket name", raw)
	}

	return s3Path{
		bucket: u.Host,
		prefix: strings.TrimPrefix(u.Path, "/"),
	}, true, nil
}
