package postgres

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

// How often to poll pg_stat_archiver while waiting for the forced WAL switch to archive.
const archivePollInterval = 2 * time.Second

type ForceWALArchiveOptions struct {
	WaitForArchiveTimeout helpers.MaxWaitTime
}

// PSQLRunner runs a single SQL statement against a Postgres server and returns psql's stdout. The
// caller decides where and how psql runs (e.g. exec'd inside the cluster's primary pod, connecting as
// the local superuser). Implementations must run psql so the result is the bare value(s) — tuples
// only, unaligned (psql -tA) — and must fail the call if the statement errors.
type PSQLRunner func(ctx *contexts.Context, sql string) (string, error)

// ForceWALArchive guarantees two things a clone's forward PITR recovery depends on: that a clone
// recovering to any wall-clock time at or before now can confirm it reached its target, and that the
// WAL segment holding the base backup's consistency point is archived. It does this by:
//
//  1. Writing a recovery fence — a no-op transaction forced to commit. SELECT txid_current() assigns
//     the transaction a real XID, so committing it writes an XLOG_XACT_COMMIT record (a transaction
//     that never touches data and is never assigned an XID writes no such record). recovery_target_time
//     only ever stops at a transaction commit, so this commit — timestamped after the recovery target —
//     is the marker that lets recovery confirm it replayed past the target and stop cleanly. Without
//     it, a source that committed nothing after the target (an idle database) makes a forward recovery
//     run out of WAL and fail with "recovery ended before configured recovery target was reached".
//  2. Forcing a WAL switch so the segment holding the fence — and, on an idle source, the consistency
//     point that shares that segment — is completed and archived.
//  3. Waiting for that segment to reach the object store before any recovery can need it.
//
// The fence MUST be a committing transaction. Do not "simplify" it to pg_create_restore_point or a
// bare pg_switch_wal: recovery_target_time is blind to restore points, and a non-committing switch
// leaves a source that is idle past the target with no record to recover to.
func ForceWALArchive(ctx *contexts.Context, run PSQLRunner, opts ForceWALArchiveOptions) (err error) {
	ctx.Log.Info("Writing WAL recovery fence and forcing WAL archive")
	defer ctx.Log.Info("Finished forcing WAL archive", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	// 1. Write the recovery fence.
	// This query was chosen specifically because it is the absolute minimum needed to cause a commit record to be written.
	if _, err := run(ctx.Child(), "SELECT txid_current()"); err != nil {
		return trace.Wrap(err, "failed to write WAL recovery fence")
	}

	// 2. Capture the WAL segment the fence landed in.
	segmentOutput, err := run(ctx.Child(), "SELECT pg_walfile_name(pg_current_wal_lsn())")
	if err != nil {
		return trace.Wrap(err, "failed to determine the WAL segment for the recovery fence")
	}

	targetSegment := lastNonEmptyLine(segmentOutput)
	if targetSegment == "" {
		return trace.Errorf("could not determine the WAL segment for the recovery fence from psql output %q", segmentOutput)
	}
	ctx.Log.Debug("Recovery fence written", "walSegment", targetSegment)

	// 3. Force a WAL switch so the target segment is completed and archived.
	if _, err := run(ctx.Child(), "SELECT pg_switch_wal()"); err != nil {
		return trace.Wrap(err, "failed to switch WAL after writing the recovery fence")
	}

	// 4. Wait for the target segment to reach the object store before any recovery can need it.
	return waitForWALArchive(ctx, run, targetSegment, opts.WaitForArchiveTimeout.MaxWait(2*time.Minute))
}

// waitForWALArchive polls pg_stat_archiver until the last archived WAL segment is at or past
// targetSegment (WAL filenames sort lexicographically in archive order within a timeline), bailing
// out early if archiving is actively failing at or past the target.
func waitForWALArchive(ctx *contexts.Context, run PSQLRunner, targetSegment string, timeout time.Duration) error {
	ctx.Log.Debug("Waiting for WAL segment to be archived", "walSegment", targetSegment, "timeout", timeout)

	deadlineCtx, cancel := context.WithTimeout(ctx.Child(), timeout)
	defer cancel()

	for {
		archived, failed, failing, err := queryArchiverStatus(ctx, run)
		if err != nil {
			return trace.Wrap(err, "failed to query WAL archiver status")
		}

		if archived != "" && archived >= targetSegment {
			ctx.Log.Debug("WAL segment archived", "walSegment", targetSegment, "lastArchivedWAL", archived)
			return nil
		}

		if failing && failed != "" && failed >= targetSegment {
			return trace.Errorf("WAL archiving is failing at segment %q (target %q); the source cluster cannot archive the consistency-point WAL", failed, targetSegment)
		}

		select {
		case <-deadlineCtx.Done():
			return trace.Wrap(deadlineCtx.Err(), "timed out after %s waiting for WAL segment %q to be archived (last archived %q)", timeout, targetSegment, archived)
		case <-time.After(archivePollInterval):
		}
	}
}

// queryArchiverStatus returns the last archived WAL segment, the last failed WAL segment, and whether
// the last archive attempt failed more recently than it succeeded.
func queryArchiverStatus(ctx *contexts.Context, run PSQLRunner) (archived, failed string, failing bool, err error) {
	output, err := run(ctx.Child(), "SELECT coalesce(last_archived_wal, ''), coalesce(last_failed_wal, ''), coalesce(last_failed_time > last_archived_time, false) FROM pg_stat_archiver")
	if err != nil {
		return "", "", false, err
	}

	fields := strings.Split(lastNonEmptyLine(output), "|")
	if len(fields) != 3 {
		return "", "", false, trace.Errorf("unexpected pg_stat_archiver output %q", output)
	}

	return fields[0], fields[1], fields[2] == "t", nil
}

// lastNonEmptyLine returns the last non-blank line of psql output, trimmed.
func lastNonEmptyLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range slices.Backward(lines) {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}

	return ""
}
