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
    * Only Postgres backend supported (SQLite not supported)
    * Restoration support pending

## Upcoming support:
* Vaultwarden restoration
* ZFS snapshot to tape drive/library
* Automatic cleanup
