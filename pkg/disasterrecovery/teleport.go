package disasterrecovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	teleportBaseMountPath    = string(os.PathSeparator) + "mnt"
	teleportCoreSQLFileName  = "backup-core.sql"
	teleportAuditSQLFileName = "backup-audit.sql"
)

type TeleportOptionsAudit struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Name    string `yaml:"name,omitempty"`
}

type TeleportBackupOptionsAudit struct {
	TeleportOptionsAudit
}

type TeleportBackupOptions struct {
	VolumeSize                   resource.Quantity                                  `yaml:"volumeSize,omitempty"`
	VolumeStorageClass           string                                             `yaml:"volumeStorageClass,omitempty"`
	CloneClusterOptions          clonedcluster.CloneClusterOptions                  `yaml:"clusterCloning,omitempty"`
	AuditCluster                 TeleportBackupOptionsAudit                         `yaml:"auditCluster,omitempty"`
	BackupToolPodCreationTimeout helpers.MaxWaitTime                                `yaml:"backupToolPodCreationTimeout,omitempty"`
	RemoteBackupToolOptions      backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	ClusterServiceSearchDomains  []string                                           `yaml:"clusterServiceSearchDomains,omitempty"`
	BackupSnapshot               OptionsBackupSnapshot                              `yaml:"backupSnapshot,omitempty"`
	CleanupTimeout               helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
}

type Teleport struct {
	kubeClusterClient kubecluster.ClientInterface
	// Testing injection
	newCNPGRestore func() CNPGRestoreInterface
}

func NewTeleport(kubeClusterClient kubecluster.ClientInterface) *Teleport {
	return &Teleport{
		kubeClusterClient: kubeClusterClient,
		newCNPGRestore:    NewCNPGRestore,
	}
}

// Backup process:
// 1. Create the DR PVC if not exists
// 2. Clone the Core cluster
// 3. Clone the Audit cluster (if enabled) with PITR set to the same time as the Core cluster clone
// 4. Deploy a backup-tool instance with access to both the Core and Audit cloned clusters
// 5. Perform a logical backup of the Core cluster
// 6. Perform a logical backup of the Audit cluster (if enabled)
// 7. Snapshot the backup PVC
func (t *Teleport) Backup(ctx *contexts.Context, namespace, backupName, coreClusterName, servingCertIssuerName, clientCertIssuerName string, opts TeleportBackupOptions) (backup *DREvent, err error) {
	backup = NewDREventNow(backupName)
	ctx.Log.With("backupName", backup.GetFullName(), "namespace", namespace).Info("Starting backup process")
	defer func() {
		backup.Stop()
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Backup process failed", keyvals...)
		} else {
			ctx.Log.Info("Backup process completed", keyvals...)
		}
	}()

	// 1. Create the DR PVC if not exists
	ctx.Log.Step().Info("Ensuring DR PVC exists")
	drVolumeSize := opts.VolumeSize
	// This is a guess as to the resource requirements. The actual size requirement will be much larger than the
	// physical size of the database due to the logical backup format.
	if drVolumeSize.IsZero() {
		lookupCtx := ctx.Child()
		lookupCtx.Log.Info("Calculating the volume size based on the CNPG cluster sizes")

		// Get the sum of the CNPG cluster allocated storage sizes
		lookupCtx.Log.Step().Info("Looking up the core CNPG cluster size")
		coreClusterSize, err := t.getClusterSize(lookupCtx.Child(), namespace, coreClusterName)
		if err != nil {
			return backup, trace.Wrap(err, "failed to get the %q cluster size", helpers.FullNameStr(namespace, coreClusterName))
		}
		drVolumeSize.Add(coreClusterSize)

		if opts.AuditCluster.Enabled {
			lookupCtx.Log.Step().Info("Looking up the audit CNPG cluster size")
			auditClusterSize, err := t.getClusterSize(lookupCtx.Child(), namespace, opts.AuditCluster.Name)
			if err != nil {
				return backup, trace.Wrap(err, "failed to get the %q cluster size", helpers.FullNameStr(namespace, opts.AuditCluster.Name))
			}
			drVolumeSize.Add(auditClusterSize)
		}

		// Default to roughly twice the sum of the CNPG cluster sizes. This may still be too small. If it is, the user
		// should specify the volume size.
		drVolumeSize.Mul(2)
	}

	drPVC, err := t.kubeClusterClient.Core().EnsurePVCExists(ctx.Child(), namespace, backup.Name, drVolumeSize, core.CreatePVCOptions{StorageClassName: opts.VolumeStorageClass})
	if err != nil {
		return backup, trace.Wrap(err, "failed to ensure backup volume exists")
	}

	// 2. Clone the Core cluster
	ctx.Log.Step().Info("Cloning CNPG cluster", "clusterName", coreClusterName)
	clonedCoreCluster, cleanupClonedCoreCluster, err := t.cloneCluster(ctx, namespace, coreClusterName, "core", backup.Name,
		servingCertIssuerName, clientCertIssuerName, backup.StartTime, opts.CleanupTimeout, &err, opts.CloneClusterOptions)
	if err != nil {
		return backup, trace.Wrap(err, "failed to clone the %q cluster", coreClusterName)
	}
	defer cleanupClonedCoreCluster()

	// 3. Clone the Audit cluster (if enabled) with PITR set to the same time as the Core cluster clone
	var clonedAuditCluster clonedcluster.ClonedClusterInterface
	if opts.AuditCluster.Enabled {
		ctx.Log.Step().Info("Cloning CNPG cluster", "clusterName", opts.AuditCluster.Name)
		var cleanupClonedAuditCluster func()
		clonedAuditCluster, cleanupClonedAuditCluster, err = t.cloneCluster(ctx, namespace, opts.AuditCluster.Name, "audit", backup.Name,
			servingCertIssuerName, clientCertIssuerName, backup.StartTime, opts.CleanupTimeout, &err, opts.CloneClusterOptions)
		if err != nil {
			return backup, trace.Wrap(err, "failed to clone the %q cluster", opts.AuditCluster.Name)
		}

		defer cleanupClonedAuditCluster()
	}

	// 4. Deploy a backup-tool instance with access to both the Core and Audit cloned clusters
	ctx.Log.Step().Info("Creating backup tool instance")

	drVolumeMountPath := filepath.Join(teleportBaseMountPath, "dr")
	secretsDirectoryPath := filepath.Join(teleportBaseMountPath, "secrets")

	coreClusterSecretsDirectoryPath := filepath.Join(secretsDirectoryPath, "core")
	coreClusterServingCertVolumeMountPath := filepath.Join(coreClusterSecretsDirectoryPath, "serving-cert")
	coreClusterClientCertVolumeMountPath := filepath.Join(coreClusterSecretsDirectoryPath, "client-cert")

	btOpts := backuptoolinstance.CreateBackupToolInstanceOptions{
		NamePrefix: fmt.Sprintf("%s-%s", constants.ToolName, backup.GetFullName()),
		Volumes: []core.SingleContainerVolume{
			core.NewSingleContainerPVC(drPVC.Name, drVolumeMountPath),
			core.NewSingleContainerSecret(clonedCoreCluster.GetServingCert().Name, coreClusterServingCertVolumeMountPath, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
			core.NewSingleContainerSecret(clonedCoreCluster.GetPostgresUserCert().GetCertificate().Name, coreClusterClientCertVolumeMountPath),
		},
		CleanupTimeout: opts.CleanupTimeout,
	}

	var auditClusterServingCertVolumeMountPath string
	var auditClusterClientCertVolumeMountPath string
	if opts.AuditCluster.Enabled {
		auditClusterSecretsDirectoryPath := filepath.Join(secretsDirectoryPath, "audit")
		auditClusterServingCertVolumeMountPath = filepath.Join(auditClusterSecretsDirectoryPath, "serving-cert")
		auditClusterClientCertVolumeMountPath = filepath.Join(auditClusterSecretsDirectoryPath, "client-cert")

		btOpts.Volumes = append(btOpts.Volumes,
			core.NewSingleContainerSecret(clonedAuditCluster.GetServingCert().Name, auditClusterServingCertVolumeMountPath, corev1.KeyToPath{Key: "tls.crt", Path: "tls.crt"}),
			core.NewSingleContainerSecret(clonedAuditCluster.GetPostgresUserCert().GetCertificate().Name, auditClusterClientCertVolumeMountPath),
		)
	}

	mergo.MergeWithOverwrite(&btOpts, opts.RemoteBackupToolOptions)
	btInstance, err := t.kubeClusterClient.CreateBackupToolInstance(ctx.Child(), namespace, backup.GetFullName(), btOpts)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create %s instance", constants.ToolName)
	}
	defer cleanup.To(btInstance.Delete).WithErrMessage("failed to cleanup backup tool instance %q resources", backup.GetFullName()).
		WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	backupToolClient, err := btInstance.GetGRPCClient(ctx.Child(), opts.ClusterServiceSearchDomains...)
	if err != nil {
		return backup, trace.Wrap(err, "failed to create client for backup tool GRPC server")
	}

	// 5. Perform a logical backup of the Core cluster
	ctx.Log.Step().Info("Performing Core cluster Postgres logical backup")
	podSQLFilePath := filepath.Join(drVolumeMountPath, teleportCoreSQLFileName)
	clusterCredentials := clonedCoreCluster.GetCredentials(coreClusterServingCertVolumeMountPath, coreClusterClientCertVolumeMountPath)
	err = backupToolClient.Postgres().DumpAll(ctx.Child(), clusterCredentials, podSQLFilePath, postgres.DumpAllOptions{CleanupTimeout: opts.CleanupTimeout})
	if err != nil {
		return backup, trace.Wrap(err, "failed to dump logical backup for core postgres server at %q", postgres.GetServerAddress(clusterCredentials))
	}

	// 6. Perform a logical backup of the Audit cluster (if enabled)
	if opts.AuditCluster.Enabled {
		ctx.Log.Step().Info("Performing Audit cluster Postgres logical backup")
		podSQLFilePath = filepath.Join(drVolumeMountPath, teleportAuditSQLFileName)
		clusterCredentials = clonedAuditCluster.GetCredentials(auditClusterServingCertVolumeMountPath, auditClusterClientCertVolumeMountPath)
		err = backupToolClient.Postgres().DumpAll(ctx.Child(), clusterCredentials, podSQLFilePath, postgres.DumpAllOptions{CleanupTimeout: opts.CleanupTimeout})
		if err != nil {
			return backup, trace.Wrap(err, "failed to dump logical backup for audit postgres server at %q", postgres.GetServerAddress(clusterCredentials))
		}
	}

	// 7. Snapshot the backup PVC
	ctx.Log.Step().Info("Snapshotting the DR volume")
	snapshot, err := t.kubeClusterClient.ES().SnapshotVolume(ctx.Child(), namespace, drPVC.Name, externalsnapshotter.SnapshotVolumeOptions{Name: helpers.CleanName(backup.GetFullName()), SnapshotClass: opts.BackupSnapshot.SnapshotClass})
	if err != nil {
		return backup, trace.Wrap(err, "failed to snapshot backup volume %q", helpers.FullName(drPVC))
	}

	_, err = t.kubeClusterClient.ES().WaitForReadySnapshot(ctx.Child(), namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: opts.BackupSnapshot.ReadyTimeout})
	if err != nil {
		return backup, trace.Wrap(err, "failed to wait for backup snapshot %q to become ready", helpers.FullName(snapshot))
	}

	return backup, nil
}

func (t *Teleport) getClusterSize(ctx *contexts.Context, namespace, clusterName string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity

	ctx.Log.With("clusterName", clusterName).Debug("Getting the cluster size")
	cluster, err := t.kubeClusterClient.CNPG().GetCluster(ctx.Child(), namespace, clusterName)
	if err != nil {
		return defaultQuantityVal, trace.Wrap(err, "failed to get the %q cluster", helpers.FullNameStr(namespace, clusterName))
	}

	ctx.Log.With("clusterSize", cluster.Spec.StorageConfiguration.Size).Debug("Parsing the cluster size")
	clusterSize, err := resource.ParseQuantity(cluster.Spec.StorageConfiguration.Size)
	if err != nil {
		return defaultQuantityVal, trace.Wrap(err, "failed to parse the %q cluster size %q", helpers.FullName(cluster), cluster.Spec.StorageConfiguration.Size)
	}

	return clusterSize, nil
}

func (t *Teleport) cloneCluster(ctx *contexts.Context, namespace, clusterName, shortClusterName, backupName, servingCertIssuerName, clientCertIssuerName string,
	recoveryTime time.Time, finalCleanupTimeout helpers.MaxWaitTime, outerErr *error, cloningOpts clonedcluster.CloneClusterOptions) (clonedcluster.ClonedClusterInterface, func(), error) {
	clonedClusterName := helpers.CleanName(fmt.Sprintf("%s-%s", clusterName, backupName))
	if len(clonedClusterName) > 50 {
		clonedClusterName = helpers.CleanName(helpers.TruncateString(fmt.Sprintf("%s-%s", shortClusterName, backupName), 50, ""))
	}

	if cloningOpts.CleanupTimeout == 0 {
		cloningOpts.CleanupTimeout = finalCleanupTimeout
	}

	if cloningOpts.RecoveryTargetTime == "" {
		cloningOpts.RecoveryTargetTime = recoveryTime.Format(time.RFC3339)
	}

	clonedCluster, err := t.kubeClusterClient.CloneCluster(ctx.Child(), namespace, clusterName,
		clonedClusterName, servingCertIssuerName, clientCertIssuerName, cloningOpts)
	if err != nil {
		return nil, nil, trace.Wrap(err, "failed to clone cluster %q", clusterName)
	}

	cleanupFunc := func() {
		// This is just the normal cleanup func with the error discarded
		_ = cleanup.To(clonedCluster.Delete).
			WithErrMessage("failed to cleanup cloned cluster %q resources", clonedClusterName).WithOriginalErr(outerErr).
			WithParentCtx(ctx).WithTimeout(finalCleanupTimeout.MaxWait(10 * time.Minute)).
			Run()
	}

	return clonedCluster, cleanupFunc, nil
}

type TeleportRestoreOptionsAudit struct {
	TeleportOptionsAudit
	ServingCertName      string                 `yaml:"servingCertName,omitempty"`
	ClientCertIssuerName string                 `yaml:"clientCertIssuerName,omitempty"`
	PostgresUserCert     OptionsClusterUserCert `yaml:"postgresUserCert,omitempty"`
}

type TeleportRestoreOptions struct {
	AuditCluster            TeleportRestoreOptionsAudit                        `yaml:"auditCluster,omitempty"`
	PostgresUserCert        OptionsClusterUserCert                             `yaml:"postgresUserCert,omitempty"`
	RemoteBackupToolOptions backuptoolinstance.CreateBackupToolInstanceOptions `yaml:"remoteBackupToolOptions,omitempty"`
	CleanupTimeout          helpers.MaxWaitTime                                `yaml:"cleanupTimeout,omitempty"`
}

// Restore requirements:
// * The DR PVC must exist
// * Replacement clusters must be already deployed
// * The enabled CNPG cluster must already exist, but not be in use
// * The enabled  CNPG client CA issuer must already exist
// * The enabled  CNPG cluster must support TLS auth for the postgres user
// * The enabled  CNPG cluster serving cert must already exist
// Restore process:
// 1. Ensure that the provided resources exist and are ready
// 2. Restore the core CNPG cluster
// 2. 1. Create postgres user cert
// 2. 2. Spawn a new backup-tool pod with postgres auth and serving certs, and DR mount attached
// 2. 3. Perform a Postgres logical recovery of the cluster
// 3. Restore the audit CNPG cluster (if enabled)
// 3. 1. Create postgres user cert
// 3. 2. Spawn a new backup-tool pod with postgres auth and serving certs, and DR mount attached
// 3. 3. Perform a Postgres logical recovery of the cluster
func (t *Teleport) Restore(ctx *contexts.Context, namespace, restoreName, coreClusterName, coreServingCertName, coreClientCertIssuerName string, opts TeleportRestoreOptions) (restore *DREvent, err error) {
	restore = NewDREventNow(restoreName)
	ctx.Log.With("restoreName", restore.GetFullName(), "namespace", namespace).Info("Starting restore process")
	defer func() {
		restore.Stop()
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Restore process failed", keyvals...)
		} else {
			ctx.Log.Info("Restore process completed", keyvals...)
		}
	}()

	coreRestore := t.newCNPGRestore()
	coreRestore.Configure(t.kubeClusterClient, namespace, coreClusterName, coreServingCertName, coreClientCertIssuerName, restoreName, restore.GetFullName(), teleportCoreSQLFileName, CNPGRestoreOpts{
		PostgresUserCert:        opts.PostgresUserCert,
		RemoteBackupToolOptions: opts.RemoteBackupToolOptions,
		CleanupTimeout:          opts.CleanupTimeout,
	})

	auditRestore := t.newCNPGRestore()
	if opts.AuditCluster.Enabled {
		auditRestore.Configure(t.kubeClusterClient, namespace, opts.AuditCluster.Name, opts.AuditCluster.ServingCertName, opts.AuditCluster.ClientCertIssuerName, restoreName, restore.GetFullName(), teleportAuditSQLFileName, CNPGRestoreOpts{
			PostgresUserCert:        opts.AuditCluster.PostgresUserCert,
			RemoteBackupToolOptions: opts.RemoteBackupToolOptions,
			CleanupTimeout:          opts.CleanupTimeout,
		})
	}

	// 1. Ensure the require resources already exist
	ctx.Log.Step().Info("Ensuring required resources exist")
	_, err = t.kubeClusterClient.Core().GetPVC(ctx.Child(), namespace, restoreName)
	if err != nil {
		return restore, trace.Wrap(err, "failed to get DR PVC %q", restoreName)
	}

	err = coreRestore.CheckResourcesReady(ctx.Child())
	if err != nil {
		return restore, trace.Wrap(err, "failed to verify that resources for core cluster restoration are ready")
	}

	if opts.AuditCluster.Enabled {
		err = auditRestore.CheckResourcesReady(ctx.Child())
		if err != nil {
			return restore, trace.Wrap(err, "failed to verify that resources for audit cluster restoration are ready")
		}
	}

	// 2. Restore the core CNPG cluster
	ctx.Log.Step().Info("Restoring the core cluster")
	err = coreRestore.Restore(ctx.Child())
	if err != nil {
		return restore, trace.Wrap(err, "failed to restore the core cluster")
	}

	// 3. Restore the audit CNPG cluster (if enabled)
	if !opts.AuditCluster.Enabled {
		return restore, nil
	}

	ctx.Log.Step().Info("Restoring the audit cluster")
	err = auditRestore.Restore(ctx.Child())
	if err != nil {
		return restore, trace.Wrap(err, "failed to restore audit cluster")
	}

	return restore, nil
}
