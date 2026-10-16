package drvolume

import (
	"maps"
	"strings"

	"github.com/gravitational/trace"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Tool-owned labels set on the DR PVC and on every backup snapshot of it, so that downstream consumers
// (e.g. tape-archiver) can select them with a label selector.
// Important: these are a contract with external consumers. Changing them breaks existing selectors!
const (
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "backup-tool"
	LabelDomain         = "backup-tool.home.arpa"
	LabelBackupName     = LabelDomain + "/backup-name"
	LabelDRType         = LabelDomain + "/dr-type"
)

// ToolOwnedLabels returns the labels that identify the DR PVC and backup snapshots of a backup.
func ToolOwnedLabels(backupName, drType string) map[string]string {
	return map[string]string{
		LabelManagedBy:  LabelManagedByValue,
		LabelBackupName: backupName,
		LabelDRType:     drType,
	}
}

func isToolOwnedLabelKey(key string) bool {
	return key == LabelManagedBy || strings.HasPrefix(key, LabelDomain+"/")
}

// ValidateSnapshotLabels checks user-supplied backup snapshot labels. Keys and values must be valid
// Kubernetes labels, and keys may not collide with the tool-owned labels.
func ValidateSnapshotLabels(labels map[string]string) error {
	if err := metav1validation.ValidateLabels(labels, field.NewPath("snapshotLabels")).ToAggregate(); err != nil {
		return trace.Wrap(err, "invalid snapshot labels")
	}

	for key := range labels {
		if isToolOwnedLabelKey(key) {
			return trace.BadParameter("snapshot label %q is reserved for labels set by backup-tool", key)
		}
	}

	return nil
}

// ValidateBackupLabels checks everything that ends up in the DR PVC and backup snapshot labels, so that
// a bad value is caught before any resources are created.
func ValidateBackupLabels(backupName, drType string, snapshotLabels map[string]string) error {
	if err := metav1validation.ValidateLabels(ToolOwnedLabels(backupName, drType), field.NewPath("labels")).ToAggregate(); err != nil {
		return trace.Wrap(err, "backup name %q and DR type %q must be valid label values", backupName, drType)
	}

	return trace.Wrap(ValidateSnapshotLabels(snapshotLabels))
}

// mergeLabels merges the label sets in order, with later sets taking precedence.
func mergeLabels(labelSets ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, labels := range labelSets {
		maps.Copy(merged, labels)
	}

	return merged
}

// missingLabels returns the subset of want that is absent from, or differs on, have.
func missingLabels(have, want map[string]string) map[string]string {
	missing := make(map[string]string)
	for key, value := range want {
		if existing, ok := have[key]; !ok || existing != value {
			missing[key] = value
		}
	}

	return missing
}
