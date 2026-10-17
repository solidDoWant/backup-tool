package disasterrecovery

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/retention"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	// Timezones are needed to evaluate retention schedules, and the container image may not have tzdata
	_ "time/tzdata"
)

// BackupCleanupConfig configures the deletion of old backup snapshots. Backups are cleaned up across every DR
// type, and the retention policy is applied to each backup (namespace + backup name) independently.
type BackupCleanupConfig struct {
	// Namespace limits the cleanup to backups in a single namespace. When empty, backups in all namespaces are
	// cleaned up.
	Namespace string `yaml:"namespace,omitempty"`
	// Selector limits the cleanup to backups matching it. Labeled backup snapshots are matched against their
	// own labels, and backup snapshots taken before snapshots were labeled are matched against their DR
	// volume's labels. Selecting on the tool-owned backup-name and dr-type labels works for both.
	Selector metav1.LabelSelector `yaml:"selector,omitempty"`
	// Retention is a list of rules that decide which backup snapshots are kept. All others are deleted.
	Retention retention.Policy `yaml:"retention" jsonschema:"required"`
	// Timezone is the IANA timezone (e.g. "America/Chicago") that retention rule schedules are evaluated in.
	// Defaults to UTC.
	Timezone string `yaml:"timezone,omitempty"`
	// DryRun logs which backup snapshots would be deleted, without deleting them.
	DryRun bool `yaml:"dryRun,omitempty"`
}

func (c BackupCleanupConfig) Validate() error {
	if err := c.Retention.Validate(); err != nil {
		return trace.Wrap(err, "invalid retention policy")
	}

	if _, err := c.location(); err != nil {
		return trace.Wrap(err)
	}

	if _, err := metav1.LabelSelectorAsSelector(&c.Selector); err != nil {
		return trace.Wrap(err, "invalid selector")
	}

	return nil
}

func (c BackupCleanupConfig) location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.UTC, nil
	}

	loc, err := time.LoadLocation(c.Timezone)
	return loc, trace.Wrap(err, "invalid timezone %q", c.Timezone)
}

type BackupCleanup struct {
	kubeClusterClient kubecluster.ClientInterface
}

func NewBackupCleanup(client kubecluster.ClientInterface) *BackupCleanup {
	return &BackupCleanup{
		kubeClusterClient: client,
	}
}

// Run deletes the backup snapshots that the retention policy doesn't keep. Snapshots that are not ready
// (such as one being taken by an in-progress backup) are neither kept nor deleted, so they don't count
// towards the policy. A failure to delete a snapshot doesn't stop the cleanup; all failures are returned.
func (bc *BackupCleanup) Run(ctx *contexts.Context, config BackupCleanupConfig) (err error) {
	ctx.Log.Info("Cleaning up backups", "namespace", config.Namespace, "dryRun", config.DryRun)
	defer ctx.Log.Info("Finished cleaning up backups", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	loc, err := config.location()
	if err != nil {
		return trace.Wrap(err)
	}

	ctx.Log.Step()
	backupSnapshots, err := bc.kubeClusterClient.ListBackupSnapshots(ctx.Child(), config.Namespace, drvolume.ListBackupSnapshotsOptions{Selector: config.Selector})
	if err != nil {
		return trace.Wrap(err, "failed to list backup snapshots")
	}

	backups := make(map[types.NamespacedName][]drvolume.BackupSnapshot)
	for _, backupSnapshot := range backupSnapshots {
		backup := types.NamespacedName{Namespace: backupSnapshot.Snapshot.Namespace, Name: backupSnapshot.BackupName}
		backups[backup] = append(backups[backup], backupSnapshot)
	}

	sortedBackups := slices.SortedFunc(maps.Keys(backups), func(a, b types.NamespacedName) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})

	var errs []error
	for _, backup := range sortedBackups {
		ctx.Log.Step()
		if err := bc.cleanupBackup(ctx.Child(), backup, backups[backup], config, loc); err != nil {
			errs = append(errs, trace.Wrap(err, "failed to clean up backup %q", backup.String()))
		}
	}

	return trace.NewAggregate(errs...)
}

func (bc *BackupCleanup) cleanupBackup(ctx *contexts.Context, backup types.NamespacedName, backupSnapshots []drvolume.BackupSnapshot, config BackupCleanupConfig, loc *time.Location) error {
	// ctx is this backup's own child context, so these keyvals are scoped to this backup
	ctx.Log.With("namespace", backup.Namespace, "backupName", backup.Name)

	candidates := make([]drvolume.BackupSnapshot, 0, len(backupSnapshots))
	skipped := 0
	for _, backupSnapshot := range backupSnapshots {
		switch {
		case backupSnapshot.IsDeleting():
			ctx.Log.Debug("Skipping snapshot that is already being deleted", "snapshot", backupSnapshot.Snapshot.Name)
			skipped++
		case !backupSnapshot.IsReady():
			ctx.Log.Info("Skipping snapshot that is not ready", "snapshot", backupSnapshot.Snapshot.Name)
			skipped++
		default:
			candidates = append(candidates, backupSnapshot)
		}
	}

	decisions, err := retention.Apply(config.Retention, candidates, drvolume.BackupSnapshot.Time, loc)
	if err != nil {
		return trace.Wrap(err, "failed to apply the retention policy")
	}

	var toDelete []retention.Decision[drvolume.BackupSnapshot]
	for _, decision := range decisions {
		if decision.Keep {
			ctx.Log.Debug("Keeping snapshot", "snapshot", decision.Item.Snapshot.Name, "snapshotTime", decision.Time, "reasons", decision.Reasons)
			continue
		}

		toDelete = append(toDelete, decision)
	}

	ctx.Log.Info("Applied retention policy", "kept", len(decisions)-len(toDelete), "deleting", len(toDelete), "skipped", skipped)

	// Delete oldest first
	slices.Reverse(toDelete)

	var errs []error
	for _, decision := range toDelete {
		snapshot := decision.Item.Snapshot
		keyvals := []any{"snapshot", snapshot.Name, "snapshotTime", decision.Time, "legacy", decision.Item.Legacy}

		if config.DryRun {
			ctx.Log.Info("Would delete snapshot (dry run)", keyvals...)
			continue
		}

		ctx.Log.Info("Deleting snapshot", keyvals...)
		err := bc.kubeClusterClient.ES().DeleteSnapshot(ctx.Child(), snapshot.Namespace, snapshot.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, trace.Wrap(err, "failed to delete snapshot %q", snapshot.Name))
		}
	}

	return trace.NewAggregate(errs...)
}
