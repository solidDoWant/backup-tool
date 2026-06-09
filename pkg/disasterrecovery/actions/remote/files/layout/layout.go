// Package layout defines the on-disk layout of DR-volume captures shared across the file backup and
// restore actions. These constants are part of the backup format and are restore-compatibility
// load-bearing: changing them breaks restore of existing backups.
package layout

// FileGroupsDirName is the parent directory under the DR volume holding every file-group capture (each
// group gets a FileGroupsDirName/<group>/<pvc> subtree). Its uppercase letter means it can never collide
// with a flat files-slot name, which are lowercase-only (^[a-z0-9]([-a-z0-9]*[a-z0-9])?$).
const FileGroupsDirName = "fileGroups"
