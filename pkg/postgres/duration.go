package postgres

import (
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/trace"
)

// postgresDurationUnits maps PostgreSQL time-value unit suffixes to their Go duration equivalents.
// See https://www.postgresql.org/docs/current/config-setting.html#CONFIG-SETTING-NAMES-VALUES.
// Ordered longest-suffix-first so that e.g. "ms" and "min" are matched before "s"/"m".
var postgresDurationUnits = []struct {
	suffix   string
	duration time.Duration
}{
	{"us", time.Microsecond},
	{"ms", time.Millisecond},
	{"min", time.Minute},
	{"s", time.Second},
	{"h", time.Hour},
	{"d", 24 * time.Hour},
}

// ParseDuration parses a PostgreSQL time-valued configuration parameter (such as archive_timeout)
// into a time.Duration. PostgreSQL accepts an integer optionally followed by a unit suffix
// (us, ms, s, min, h, d). A bare integer with no unit is interpreted in seconds, which is the
// default unit for archive_timeout and the other WAL-timing parameters this is used for.
func ParseDuration(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, trace.Errorf("empty duration value")
	}

	// Default unit is seconds when only a number is provided.
	numberPart := trimmed
	unit := time.Second
	for _, candidate := range postgresDurationUnits {
		if before, found := strings.CutSuffix(trimmed, candidate.suffix); found {
			numberPart = strings.TrimSpace(before)
			unit = candidate.duration
			break
		}
	}

	magnitude, err := strconv.ParseInt(numberPart, 10, 64)
	if err != nil {
		return 0, trace.Wrap(err, "failed to parse %q as a PostgreSQL duration", value)
	}

	return time.Duration(magnitude) * unit, nil
}
