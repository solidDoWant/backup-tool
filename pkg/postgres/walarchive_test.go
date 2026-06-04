package postgres

import (
	"strings"
	"testing"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastNonEmptyLine(t *testing.T) {
	assert.Equal(t, "000000010000002B00000000", lastNonEmptyLine("2B/0\n000000010000002B00000000\n"))
	assert.Equal(t, "only", lastNonEmptyLine("only"))
	assert.Equal(t, "", lastNonEmptyLine("\n  \n"))
}

// scriptedRunner returns a PSQLRunner that responds to each SQL statement, recording the statements it
// was asked to run. archiverResult is returned for the pg_stat_archiver poll.
func scriptedRunner(targetSegment, archiverResult string, archiverErr error) (PSQLRunner, *[]string) {
	statements := &[]string{}
	run := func(_ *contexts.Context, sql string) (string, error) {
		*statements = append(*statements, sql)
		switch {
		case strings.Contains(sql, "pg_walfile_name"):
			return targetSegment + "\n", nil
		case strings.Contains(sql, "pg_stat_archiver"):
			return archiverResult, archiverErr
		default:
			return "0/0\n", nil
		}
	}
	return run, statements
}

func TestForceWALArchive(t *testing.T) {
	const targetSegment = "000000010000002B00000000"

	t.Run("writes a commit fence, switches WAL, and waits for archive", func(t *testing.T) {
		run, statements := scriptedRunner(targetSegment, targetSegment+"||f\n", nil)

		err := ForceWALArchive(th.NewTestContext(), run, ForceWALArchiveOptions{})
		require.NoError(t, err)

		// The fence MUST be a committing transaction (txid_current forces an XID so the commit writes a
		// record recovery_target_time can stop at) — never pg_create_restore_point.
		joined := strings.Join(*statements, "\n")
		assert.Contains(t, joined, "txid_current()")
		assert.NotContains(t, joined, "pg_create_restore_point")
		assert.Contains(t, joined, "pg_switch_wal()")
		assert.Contains(t, joined, "pg_stat_archiver")
		// The fence is written before the segment is captured and the switch forced.
		assert.Less(t, strings.Index(joined, "txid_current()"), strings.Index(joined, "pg_switch_wal()"))
	})

	t.Run("fails when archiving is actively failing at the target", func(t *testing.T) {
		run, _ := scriptedRunner(targetSegment, "000000010000002A000000FF|"+targetSegment+"|t\n", nil)

		err := ForceWALArchive(th.NewTestContext(), run, ForceWALArchiveOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WAL archiving is failing")
	})

	t.Run("propagates a psql error", func(t *testing.T) {
		run := func(_ *contexts.Context, _ string) (string, error) { return "", trace.Errorf("boom") }

		err := ForceWALArchive(th.NewTestContext(), run, ForceWALArchiveOptions{})
		require.Error(t, err)
	})
}
