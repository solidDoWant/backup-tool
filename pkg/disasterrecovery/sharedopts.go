package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

type OptionsBackupSnapshot struct {
	ReadyTimeout  helpers.MaxWaitTime `yaml:"snapshotReadyTimeout,omitempty"`
	SnapshotClass string              `yaml:"snapshotClass,omitempty"`
}

// DR types, used as the app's command name and as the value of the dr-type label on backup resources.
// Important: changing these will break label selectors that downstream consumers use to find backups!
const (
	VaultWardenDRType = "vaultwarden"
	TeleportDRType    = "teleport"
	AuthentikDRType   = "authentik"
	GenericDRType     = "generic"
)
