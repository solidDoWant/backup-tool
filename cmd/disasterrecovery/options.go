package disasterrecovery

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ConfigBackupVolume struct {
	Size         resource.Quantity `yaml:"size, omitempty"`
	StorageClass string            `yaml:"storageClass, omitempty"`
	// SnapshotLabels are added to the backup snapshot, in addition to the labels set by backup-tool.
	SnapshotLabels map[string]string `yaml:"snapshotLabels,omitempty"`
}

// Validate checks the labels that will be applied to the DR volume and its backup snapshot.
func (cbv ConfigBackupVolume) Validate(backupName, drType string) error {
	return trace.Wrap(drvolume.ValidateBackupLabels(backupName, drType, cbv.SnapshotLabels), "invalid backupVolume labels")
}

type ConfigBTI struct {
	CreationOptions backuptoolinstance.CreateBackupToolInstanceOptions `yaml:",inline"`
}
