package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

type OptionsBackupSnapshot struct {
	ReadyTimeout  helpers.MaxWaitTime `yaml:"snapshotReadyTimeout,omitempty"`
	SnapshotClass string              `yaml:"snapshotClass,omitempty"`
}
