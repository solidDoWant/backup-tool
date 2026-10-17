# backup-tool

Tool for backing up [my home infrastructure](https://github.com/solidDoWant/infra-mk3). Might be helpful for other folks as well.

## Features:
* Consistent backups for all resources (i.e. application databases and filesystem states will match)
* Backups stored as files, in a human-readable format where possible
* Little to no impact on running applications
    * No need to take databases offline for a consistent backup
    * No performance impact on running applications where possible
* Backup jobs run on demand, with no long-lived processes (or containers) needed
* Takes advantage of underlying filesystem features where possible, rather than reimplementing in-band
    * ZFS-backed storage reduces backup size via Copy on Write and compression (if enabled)
* Can run locally or in-cluster
* No passwords wherever possible - short-lived x509 certificates instead

See [the design decision doc](docs/design%20decisions.md) for additional details.

## Supported applications:
* Vaultwarden
    * Backup to an in-cluster PVC, which is then snapshotted
    * Restore from an in-cluster PVC
    * Only Postgres backend supported (SQLite not supported)
* Teleport
    * Backup to an in-cluster PVC, which is then snapshotted
    * Restore from an in-cluster PVC
    * Only Postgres supported for "core" backend and audit events
    * Only S3-compatible object storage supported for audit session logs
* Authentik
* Generic
    * Backup to an in-cluster PVC, which is then snapshotted
    * Restore from an in-cluster PVC
    * Specify multiple CNPG clusters, S3 buckets, and up to one volume and get a consistent backup
    * Multiple PVCs result in an inconsistent backup due to tool limitation (VolumeGroupSnapshot is not supported yet)

## Backup cleanup:
Old backup snapshots can be deleted with a retention policy made of cron schedules, typically by a nightly CronJob
(`dr backups cleanup run`). The policy is applied to each backup (namespace + backup name) independently, for every DR
type. Backup snapshots are found by their `backup-tool.home.arpa` labels, and snapshots taken before backups were
labeled are found by being a snapshot of a DR volume with a backup snapshot name. The DR volume is labeled by
the first backup taken with a version that labels backups, so older snapshots are only cleaned up after that backup has
run. Older snapshots of a DR volume that no longer exists are never deleted.

With the dr-job chart:
```yaml
jobConfig:
  drType: backups
  drAction: cleanup
  configFile:
    namespace: "" # All namespaces. Defaults to the release namespace.
    retention:
      - schedule: "@daily"   # The backup from each of the last 14 days
        keep: 14
      - schedule: "@weekly"  # ... each of the last 4 Sundays
        keep: 4
      - schedule: "@monthly" # ... the 1st of each of the last 12 months
        keep: 12
      - schedule: "@yearly"  # ... Jan 1 of every year (-1 = unlimited)
        keep: -1
    # timezone: America/Chicago # The timezone schedules are evaluated in. Defaults to UTC.
    # dryRun: true # Log what would be deleted without deleting anything
```

Each rule is a cron schedule (five fields, or `@yearly`/`@monthly`/`@weekly`/`@daily`/`@hourly`) and a count. Each
scheduled time keeps the first backup taken at or after it (until the next scheduled time), so if Sunday's backup
failed, Monday's stands in. A backup is kept if any rule keeps it. A field prefixed with `!` matches every value the
rest of the field doesn't, e.g. `0 0 * * !sun` is every day but Sunday and `0 0 1 !jan,jul *` is the 1st of every month
but January and July. Unlike cron, when both day of month and day of week are set a day must match both: `0 0 1 * !sun`
is the 1st of the month unless it's a Sunday, not "the 1st, or any day but Sunday".

Rules count periods that have a backup, not calendar periods, so if backups stop being taken the existing ones are kept
rather than aged out. Snapshots that aren't ready yet (such as one being taken by an in-progress backup) are ignored.
Deleting a `VolumeSnapshot` only deletes the underlying storage snapshot if its `VolumeSnapshotClass` has
`deletionPolicy: Delete`. See `schemas/backups-cleanup.schema.json` for all options.

## Upcoming support:
* ZFS snapshot to tape drive/library
