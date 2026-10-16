package drvolume

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolOwnedLabels(t *testing.T) {
	assert.Equal(t, map[string]string{
		"app.kubernetes.io/managed-by":      "backup-tool",
		"backup-tool.home.arpa/backup-name": "some-backup",
		"backup-tool.home.arpa/dr-type":     "vaultwarden",
	}, ToolOwnedLabels("some-backup", "vaultwarden"))
}

func TestValidateBackupLabels(t *testing.T) {
	tests := []struct {
		desc           string
		backupName     string
		drType         string
		snapshotLabels map[string]string
		wantErr        bool
	}{
		{
			desc:       "no snapshot labels",
			backupName: "backup",
			drType:     "generic",
		},
		{
			desc:           "valid snapshot labels",
			backupName:     "backup",
			drType:         "generic",
			snapshotLabels: map[string]string{"tape.home.arpa/archive": "true", "simple": ""},
		},
		{
			desc:       "backup name too long",
			backupName: strings.Repeat("a", 64),
			drType:     "generic",
			wantErr:    true,
		},
		{
			desc:           "invalid key",
			backupName:     "backup",
			drType:         "generic",
			snapshotLabels: map[string]string{"not a valid/key/at all": "true"},
			wantErr:        true,
		},
		{
			desc:           "invalid value",
			backupName:     "backup",
			drType:         "generic",
			snapshotLabels: map[string]string{"key": "-starts-with-dash"},
			wantErr:        true,
		},
		{
			desc:           "reserved managed-by key",
			backupName:     "backup",
			drType:         "generic",
			snapshotLabels: map[string]string{LabelManagedBy: "someone-else"},
			wantErr:        true,
		},
		{
			desc:           "reserved tool domain key",
			backupName:     "backup",
			drType:         "generic",
			snapshotLabels: map[string]string{LabelDomain + "/anything": "value"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := ValidateBackupLabels(tt.backupName, tt.drType, tt.snapshotLabels)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestMergeLabels(t *testing.T) {
	assert.Equal(t, map[string]string{"a": "3", "b": "2", "c": "3"}, mergeLabels(
		map[string]string{"a": "1", "b": "2"},
		nil,
		map[string]string{"a": "3", "c": "3"},
	))
	assert.Empty(t, mergeLabels())
}

func TestMissingLabels(t *testing.T) {
	have := map[string]string{"same": "value", "different": "old", "extra": "value"}
	want := map[string]string{"same": "value", "different": "new", "absent": "value"}

	assert.Equal(t, map[string]string{"different": "new", "absent": "value"}, missingLabels(have, want))
	assert.Empty(t, missingLabels(want, want))
	assert.Equal(t, want, missingLabels(nil, want))
}
