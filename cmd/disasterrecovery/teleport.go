package disasterrecovery

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpgrestore"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	"k8s.io/apimachinery/pkg/api/resource"
)

type TeleportConfigBTI struct {
	CreationOptions backuptoolinstance.CreateBackupToolInstanceOptions `yaml:",inline"`
}

type TeleportConfigAuditSessionLogs struct {
	S3Path      string         `yaml:"s3Path,omitempty"`
	Credentials s3.Credentials `yaml:"credentials,omitempty"`
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
	Audit TeleportBackupConfigClusterConfig `yaml:"audit,omitempty"`
}

type TeleportBackupConfig struct {
	Namespace              string                                 `yaml:"namespace" jsonschema:"required"`
	BackupName             string                                 `yaml:"backupName" jsonschema:"required"`
	CNPGClusters           TeleportBackupConfigClustersConfig     `yaml:"cnpgClusters" jsonschema:"required"`
	ServingCertIssuerName  string                                 `yaml:"servingCertIssuerName" jsonschema:"required"`
	ClientCACertIssuerName string                                 `yaml:"clientCACertIssuerName" jsonschema:"required"`
	AuditSessionLogs       TeleportConfigAuditSessionLogs         `yaml:"auditSessionLogs,omitempty"`
	BackupVolume           TeleportBackupConfigBackupVolume       `yaml:"backupVolume" jsonschema:"omitempty"`
	BackupSnapshot         disasterrecovery.OptionsBackupSnapshot `yaml:"backupSnapshot" jsonschema:"omitempty"`
	CloneClusterOptions    clonedcluster.CloneClusterOptions      `yaml:"clusterCloning,omitempty"`
	BackupToolInstance     TeleportConfigBTI                      `yaml:"backupToolInstance,omitempty"`
	ServiceSearchDomains   []string                               `yaml:"serviceSearchDomains,omitempty"`
	CleanupTimeout         helpers.MaxWaitTime                    `yaml:"cleanupTimeout,omitempty"`
}

type TeleportRestoreClusterConfig struct {
	Name                 string                             `yaml:"name" jsonschema:"required"`
	ServingCertName      string                             `yaml:"servingCertName" jsonschema:"required"`
	ClientCertIssuerName string                             `yaml:"clientCertIssuerName" jsonschema:"required"`
	ClusterUserCert      cnpgrestore.CNPGRestoreOptionsCert `yaml:"clusterUserCert,omitempty"`
}

type TeleportRestoreClustersConfig struct {
	Core  TeleportRestoreClusterConfig `yaml:"core" jsonschema:"required"`
	Audit TeleportRestoreClusterConfig `yaml:"audit,omitempty"`
}

type TeleportRestoreConfig struct {
	Namespace            string                         `yaml:"namespace" jsonschema:"required"`
	BackupName           string                         `yaml:"backupName" jsonschema:"required"`
	CNPGClusters         TeleportRestoreClustersConfig  `yaml:"cnpgClusters" jsonschema:"required"`
	AuditSessionLogs     TeleportConfigAuditSessionLogs `yaml:"auditSessionLogs,omitempty"`
	BackupToolInstance   TeleportConfigBTI              `yaml:"backupToolInstance,omitempty"`
	ServiceSearchDomains []string                       `yaml:"serviceSearchDomains,omitempty"`
	CleanupTimeout       helpers.MaxWaitTime            `yaml:"cleanupTimeout,omitempty"`
}

type TeleportDRCommand struct {
	*ClusterDRCommand[TeleportBackupConfig, TeleportRestoreConfig]
}

func NewTeleportDRCommand() *TeleportDRCommand {
	tBackup := func(ctx *contexts.Context, config TeleportBackupConfig, kubeCluster kubecluster.ClientInterface) error {
		t := disasterrecovery.NewTeleport(kubeCluster)

		opts := disasterrecovery.TeleportBackupOptions{
			VolumeSize:          config.BackupVolume.Size,
			VolumeStorageClass:  config.BackupVolume.StorageClass,
			CloneClusterOptions: config.CloneClusterOptions,
			AuditCluster: disasterrecovery.TeleportBackupOptionsAudit{
				TeleportOptionsAudit: disasterrecovery.TeleportOptionsAudit{

					Enabled: config.CNPGClusters.Audit.CNPGClusterName != "",
					Name:    config.CNPGClusters.Audit.CNPGClusterName,
				},
			},
			AuditSessionLogs: disasterrecovery.TeleportOptionsS3Sync{
				Enabled:     config.AuditSessionLogs.S3Path != "",
				S3Path:      config.AuditSessionLogs.S3Path,
				Credentials: config.AuditSessionLogs.Credentials,
			},
			RemoteBackupToolOptions:     config.BackupToolInstance.CreationOptions,
			ClusterServiceSearchDomains: config.ServiceSearchDomains,
			BackupSnapshot:              config.BackupSnapshot,
			CleanupTimeout:              config.CleanupTimeout,
		}

		_, err := t.Backup(ctx, config.Namespace, config.BackupName, config.CNPGClusters.Core.CNPGClusterName,
			config.ServingCertIssuerName, config.ClientCACertIssuerName, opts)

		return err
	}

	tRestore := func(ctx *contexts.Context, config TeleportRestoreConfig, kubeCluster kubecluster.ClientInterface) error {
		t := disasterrecovery.NewTeleport(kubeCluster)

		_, err := t.Restore(ctx, config.Namespace, config.BackupName, config.CNPGClusters.Core.Name, config.CNPGClusters.Core.ServingCertName,
			config.CNPGClusters.Core.ClientCertIssuerName, disasterrecovery.TeleportRestoreOptions{
				AuditCluster: disasterrecovery.TeleportRestoreOptionsAudit{
					TeleportOptionsAudit: disasterrecovery.TeleportOptionsAudit{
						Enabled: config.CNPGClusters.Audit.Name != "",
						Name:    config.CNPGClusters.Audit.Name,
					},
					ServingCertName:      config.CNPGClusters.Audit.ServingCertName,
					ClientCertIssuerName: config.CNPGClusters.Audit.ClientCertIssuerName,
					PostgresUserCert:     config.CNPGClusters.Audit.ClusterUserCert,
				},
				AuditSessionLogs: disasterrecovery.TeleportOptionsS3Sync{
					Enabled:     config.AuditSessionLogs.S3Path != "",
					S3Path:      config.AuditSessionLogs.S3Path,
					Credentials: config.AuditSessionLogs.Credentials,
				},
				PostgresUserCert:            config.CNPGClusters.Core.ClusterUserCert,
				RemoteBackupToolOptions:     config.BackupToolInstance.CreationOptions,
				ClusterServiceSearchDomains: config.ServiceSearchDomains,
				CleanupTimeout:              config.CleanupTimeout,
			})

		return err
	}

	return &TeleportDRCommand{
		ClusterDRCommand: NewClusterDRCommand("teleport", tBackup, tRestore),
	}
}
