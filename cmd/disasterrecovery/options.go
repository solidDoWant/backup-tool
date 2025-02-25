package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ConfigBackupVolume struct {
	Size         resource.Quantity `yaml:"size, omitempty"`
	StorageClass string            `yaml:"storageClass, omitempty"`
}

type ConfigBTI struct {
	CreationOptions      backuptoolinstance.CreateBackupToolInstanceOptions `yaml:",inline"`
	ServiceSearchDomains []string                                           `yaml:"serviceSearchDomains,omitempty"`
}
