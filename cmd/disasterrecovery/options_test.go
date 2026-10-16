package disasterrecovery

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/stretchr/testify/assert"
)

func TestConfigBackupVolumeValidate(t *testing.T) {
	assert.NoError(t, ConfigBackupVolume{}.Validate("backup", "vaultwarden"))
	assert.NoError(t, ConfigBackupVolume{SnapshotLabels: map[string]string{"tape.home.arpa/archive": "true"}}.Validate("backup", "vaultwarden"))
	assert.Error(t, ConfigBackupVolume{SnapshotLabels: map[string]string{"bad key!": "true"}}.Validate("backup", "vaultwarden"))
	assert.Error(t, ConfigBackupVolume{SnapshotLabels: map[string]string{"key": "bad value!"}}.Validate("backup", "vaultwarden"))
}

// Backup configs must be validated at load time so that bad labels fail the run before any resources are created.
func TestBackupConfigsAreValidatable(t *testing.T) {
	invalidVolume := ConfigBackupVolume{SnapshotLabels: map[string]string{"bad key!": "true"}}

	configs := map[string]features.ValidatableConfig{
		"vaultwarden": VaultWardenBackupConfig{BackupName: "backup", BackupVolume: invalidVolume},
		"teleport":    TeleportBackupConfig{BackupName: "backup", BackupVolume: invalidVolume},
		"authentik":   AuthentikBackupConfig{BackupName: "backup", BackupVolume: invalidVolume},
		"generic": disasterrecovery.GenericBackupConfig{
			BackupName:   "backup",
			BackupVolume: disasterrecovery.GenericBackupVolume{SnapshotLabels: invalidVolume.SnapshotLabels},
			Files:        []disasterrecovery.GenericFilesBackupSource{{GenericFilesSource: disasterrecovery.GenericFilesSource{Name: "data", PVC: "data"}}},
		},
	}

	for name, config := range configs {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, config.Validate())
		})
	}
}
