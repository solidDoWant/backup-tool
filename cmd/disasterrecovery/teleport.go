package disasterrecovery

import (
	"fmt"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"k8s.io/apimachinery/pkg/api/resource"
)

type TeleportBackupConfigBTI struct {
	CreationTimeout helpers.MaxWaitTime                                `yaml:"creationTimeout,omitempty"`
	CreationOptions backuptoolinstance.CreateBackupToolInstanceOptions `yaml:",inline"`
}

type TeleportBackupConfigBackupVolume struct {
	Size         resource.Quantity `yaml:"size, omitempty"`
	StorageClass string            `yaml:"storageClass, omitempty"`
}

type TeleportBackupConfigClusterConfig struct {
	CNPGClusterName string `yaml:"name" jsonschema:"required"`
}

type TeleportBackupConfigClustersConfig struct {
	Core  TeleportBackupConfigClusterConfig `yaml:"core" jsonschema:"required"`
	Audit TeleportBackupConfigClusterConfig `yaml:"audit" jsonschema:"required"`
}

type TeleportBackupConfig struct {
	Namespace              string                                                  `yaml:"namespace" jsonschema:"required"`
	BackupName             string                                                  `yaml:"backupName" jsonschema:"required"`
	CNPGClusters           TeleportBackupConfigClustersConfig                      `yaml:"cnpgClusters" jsonschema:"required"`
	ServingCertIssuerName  string                                                  `yaml:"servingCertIssuerName" jsonschema:"required"`
	ClientCACertIssuerName string                                                  `yaml:"clientCACertIssuerName" jsonschema:"required"`
	BackupVolume           TeleportBackupConfigBackupVolume                        `yaml:"backupVolume" jsonschema:"omitempty"`
	BackupSnapshot         disasterrecovery.VaultWardenBackupOptionsBackupSnapshot `yaml:"backupSnapshot" jsonschema:"omitempty"`
	CloneClusterOptions    clonedcluster.CloneClusterOptions                       `yaml:"clusterCloning,omitempty"`
	BackupToolInstance     TeleportBackupConfigBTI                                 `yaml:"backupToolInstance,omitempty"`
	ServiceSearchDomains   []string                                                `yaml:"serviceSearchDomains,omitempty"`
	CleanupTimeout         helpers.MaxWaitTime                                     `yaml:"cleanupTimeout,omitempty"`
}

type TeleportRestoreConfig struct {
	// TODO
}

type TeleportDRCommand struct {
	*ClusterDRCommand[TeleportBackupConfig, TeleportRestoreConfig]
}

func NewTeleportDRCommand() *TeleportDRCommand {
	tBackup := func(ctx *contexts.Context, config TeleportBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		t := disasterrecovery.NewTeleport(kubeCluster)

		auditClusterEnabled := false
		if config.CNPGClusters.Audit.CNPGClusterName != "" {
			auditClusterEnabled = true
		}

		opts := disasterrecovery.TeleportBackupOptions{
			VolumeSize:          config.BackupVolume.Size,
			VolumeStorageClass:  config.BackupVolume.StorageClass,
			CloneClusterOptions: config.CloneClusterOptions,
			AuditCluster: disasterrecovery.TeleportBackupOptionsAudit{
				Name:    config.CNPGClusters.Audit.CNPGClusterName,
				Enabled: auditClusterEnabled,
			},
			BackupToolPodCreationTimeout: config.BackupToolInstance.CreationTimeout,
			RemoteBackupToolOptions:      config.BackupToolInstance.CreationOptions,
			ClusterServiceSearchDomains:  config.ServiceSearchDomains,
			BackupSnapshot:               config.BackupSnapshot,
			CleanupTimeout:               config.CleanupTimeout,
		}

		_, err := t.Backup(ctx, config.Namespace, config.BackupName, config.CNPGClusters.Core.CNPGClusterName,
			config.ServingCertIssuerName, config.ClientCACertIssuerName, opts)

		return err
	}

	tRestore := func(ctx *contexts.Context, config TeleportRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		return fmt.Errorf("not implemented")
	}

	return &TeleportDRCommand{
		ClusterDRCommand: NewClusterDRCommand("teleport", tBackup, tRestore),
	}
}
