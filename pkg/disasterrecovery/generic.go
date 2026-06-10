package disasterrecovery

import (
	"fmt"
	"regexp"

	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	cnpgbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/backup"
	cnpgrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/cnpg/restore"
	filesbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/backup"
	filesgroupbackup "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/groupbackup"
	filesgrouprestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/grouprestore"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/layout"
	filesrestore "github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/restore"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/s3sync"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/drvolume"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The generic, config-driven DR "application": rather than a hand-written Go assembler per app
// (vaultwarden.go, teleport.go, authentik.go), a standard app is expressed as a declarative config of
// N volumes / M CNPG clusters / O S3 buckets. The engine (RemoteStage + the existing actions) is reused
// verbatim; only the composition is lifted out of Go.
//
// Sources are grouped by kind (postgres/files/fileGroups/s3) as plain typed slices, which the existing config
// toolchain (goccy strict YAML + go-playground/validator + invopop/jsonschema) handles directly. Backup
// and restore are separate types (and files) because their direction-specific fields differ materially;
// goccy strict mode then rejects a restore-only field in a backup file and vice-versa.

// GenericFilesSource captures (backup) / restores a data-directory PVC into / from a subdirectory of the
// DR volume. Shared by both directions — v1 restores in place (same target as backup).
type GenericFilesSource struct {
	Name string `yaml:"name" jsonschema:"required"` // slot id => DR subdir "<name>"
	PVC  string `yaml:"pvc" jsonschema:"required"`  // sourcePVCName (backup) / targetPVCName (restore)
}

// GenericFilesBackupSource is a files source plus backup-only capture options. SnapshotClass selects the
// VolumeSnapshotClass used when snapshotting the source PVC for a consistent point-in-time clone; when
// empty the cluster default VolumeSnapshotClass is used. Include/Exclude (inlined files.FileFilter)
// optionally whitelist/blacklist which files within the source PVC are captured. These are backup-only
// fields and live on a backup-specific type (mirroring the postgres backup/restore split): the capture is
// already filtered on disk, so restore reads it back verbatim and needs no filter.
type GenericFilesBackupSource struct {
	GenericFilesSource `yaml:",inline"`
	SnapshotClass      string `yaml:"snapshotClass,omitempty"`
	files.FileFilter   `yaml:",inline"`
}

// GenericFileGroupSource captures (backup) / restores a label-selected group of data-directory PVCs into /
// from the DR volume, frozen atomically as a single VolumeGroupSnapshot. The same selector is supplied in
// both directions: at backup it selects the live member PVCs to snapshot together; at restore it re-resolves
// the (already-hydrated) target PVCs so each captured member syncs back onto its identically-named PVC. The
// capture lands under "fileGroups/<name>/<pvc>" on the DR volume (one member subdir per PVC). Shared by both
// directions — v1 restores in place.
type GenericFileGroupSource struct {
	Name     string               `yaml:"name" jsonschema:"required"`     // slot id => DR subdir "fileGroups/<name>"
	Selector metav1.LabelSelector `yaml:"selector" jsonschema:"required"` // member PVC selector (must match >=1 PVC)
}

// GenericFileGroupBackupSource is a file-group source plus backup-only capture options. SnapshotClass
// selects the VolumeGroupSnapshotClass used when snapshotting the member PVCs; when empty the cluster
// default is used. Include/Exclude (inlined files.FileFilter) optionally whitelist/blacklist which files
// are captured, applied identically to every member PVC of the group (membership is selector-resolved, so
// there is no per-member filter). These are backup-only fields and live on a backup-specific type
// (mirroring the files/postgres backup/restore split): the capture is already filtered on disk, so restore
// reads it back verbatim.
type GenericFileGroupBackupSource struct {
	GenericFileGroupSource `yaml:",inline"`
	SnapshotClass          string `yaml:"snapshotClass,omitempty"`
	files.FileFilter       `yaml:",inline"`
}

// GenericS3Source syncs an object-store prefix to (backup) / from (restore) a subdirectory of the DR
// volume. Credentials are an optional inline s3.Credentials (matching the per-app configs); when omitted
// the AWS environment variables are used (s3.NewCredentialsFromEnv).
type GenericS3Source struct {
	Name        string         `yaml:"name" jsonschema:"required"` // slot id => DR subdir "<name>"
	Path        string         `yaml:"path" jsonschema:"required"` // s3://bucket/prefix
	Credentials s3.Credentials `yaml:"credentials,omitempty"`
}

// GenericPostgresBackupSource clones a CNPG cluster and logically dumps it to the DR volume. The clone's
// serving cert and client-CA cert are minted from a self-signed issuer created internally during
// cloning, so no issuer needs to be supplied; clusterCloning carries the remaining cloning options
// (recovery target, snapshot/cert timeouts, and the self-signed issuer's CertificateRequestPolicy).
type GenericPostgresBackupSource struct {
	Name           string                            `yaml:"name" jsonschema:"required"`           // slot id => dump file "<name>.sql"
	Cluster        string                            `yaml:"cluster" jsonschema:"required"`        // clusterName
	ClusterCloning clonedcluster.CloneClusterOptions `yaml:"clusterCloning" jsonschema:"required"` // CNPGBackupOptions.CloningOpts
}

// GenericPostgresRestoreSource logically restores a SQL dump from the DR volume into a live cluster.
type GenericPostgresRestoreSource struct {
	Name             string                             `yaml:"name" jsonschema:"required"`           // slot id => dump file "<name>.sql"
	Cluster          string                             `yaml:"cluster" jsonschema:"required"`        // clusterName (v1: same target as backup)
	ServingCert      string                             `yaml:"servingCert" jsonschema:"required"`    // existing serving cert on the live target cluster
	ClientCAIssuer   cmmeta.IssuerReference             `yaml:"clientCAIssuer" jsonschema:"required"` // issuer that mints the postgres user cert (name + kind + group)
	PostgresUserCert cnpgrestore.CNPGRestoreOptionsCert `yaml:"postgresUserCert,omitempty"`
}

// GenericBackupVolume configures the DR volume and its snapshot for a backup event.
type GenericBackupVolume struct {
	StorageClass         string              `yaml:"storageClass,omitempty"`
	SnapshotClass        string              `yaml:"snapshotClass,omitempty"`
	Size                 resource.Quantity   `yaml:"size,omitempty"`
	SnapshotReadyTimeout helpers.MaxWaitTime `yaml:"snapshotReadyTimeout,omitempty"`
}

// GenericBackupConfig is the declarative backup config for the generic app. A backup produces the event
// named backupName.
type GenericBackupConfig struct {
	Namespace      string                         `yaml:"namespace" jsonschema:"required"`
	BackupName     string                         `yaml:"backupName" jsonschema:"required"`
	BackupVolume   GenericBackupVolume            `yaml:"backupVolume,omitempty"`
	CleanupTimeout helpers.MaxWaitTime            `yaml:"cleanupTimeout,omitempty"`
	Postgres       []GenericPostgresBackupSource  `yaml:"postgres,omitempty"`
	Files          []GenericFilesBackupSource     `yaml:"files,omitempty"`
	FileGroups     []GenericFileGroupBackupSource `yaml:"fileGroups,omitempty"`
	S3             []GenericS3Source              `yaml:"s3,omitempty"`
}

// GenericRestoreConfig is the declarative restore config for the generic app. A restore reads the DR PVC
// named backupName, which must already exist in the namespace (hydrated from a backup snapshot
// out-of-band, as for the per-app restores). v1 restores in place — the targets are the same resources
// the backup captured.
type GenericRestoreConfig struct {
	Namespace      string                         `yaml:"namespace" jsonschema:"required"`
	BackupName     string                         `yaml:"backupName" jsonschema:"required"`
	CleanupTimeout helpers.MaxWaitTime            `yaml:"cleanupTimeout,omitempty"`
	Postgres       []GenericPostgresRestoreSource `yaml:"postgres,omitempty"`
	Files          []GenericFilesSource           `yaml:"files,omitempty"`
	FileGroups     []GenericFileGroupSource       `yaml:"fileGroups,omitempty"`
	S3             []GenericS3Source              `yaml:"s3,omitempty"`
}

// Validation. The shared config-load path (features.ConfigFileCommand.validateConfig) runs go-playground
// tag validation but does not descend into slices of structs, so per-source required fields and all
// cross-field rules are enforced here instead. Validate is the single entrypoint Backup/Restore call
// before touching any resources.

// genericSlotNameRegex enforces a DNS-1123-label-shaped name: it becomes a filename slot and feeds derived
// resource names subject to helpers.CleanName and the 40-char CNPG clone-name cap.
var genericSlotNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func validateSlotName(kind, name string) error {
	if name == "" {
		return trace.BadParameter("a %s source has an empty name", kind)
	}
	if !genericSlotNameRegex.MatchString(name) {
		return trace.BadParameter("%s source name %q is not DNS/path-safe (lowercase alphanumeric and '-', must start and end with an alphanumeric)", kind, name)
	}
	// The flat-slot namespace shares the DR-volume root with the file-groups parent (layout.FileGroupsDirName),
	// so no slot may take its name. The lowercase-only regex above already rules this out, but enforce it
	// explicitly so the invariant survives a future regex change rather than silently allowing a collision.
	if name == layout.FileGroupsDirName {
		return trace.BadParameter("%s source name %q is reserved for the file-groups directory", kind, name)
	}
	return nil
}

func validateFilesSources(files []GenericFilesSource) error {
	seen := make(map[string]struct{}, len(files))
	for _, src := range files {
		if err := validateSlotName("files", src.Name); err != nil {
			return trace.Wrap(err)
		}
		if _, dup := seen[src.Name]; dup {
			return trace.BadParameter("duplicate files slot name %q (collides on the on-disk subdir)", src.Name)
		}
		seen[src.Name] = struct{}{}
		if src.PVC == "" {
			return trace.BadParameter("files source %q: pvc is required", src.Name)
		}
	}
	return nil
}

// isEmptyLabelSelector reports whether a selector constrains nothing. An empty LabelSelector matches every
// PVC in the namespace, which for a file group would silently capture/restore unrelated volumes, so it is
// rejected as a misconfiguration.
func isEmptyLabelSelector(selector metav1.LabelSelector) bool {
	return len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0
}

func validateFileGroupSources(groups []GenericFileGroupSource) error {
	seen := make(map[string]struct{}, len(groups))
	for _, src := range groups {
		if err := validateSlotName("fileGroup", src.Name); err != nil {
			return trace.Wrap(err)
		}
		if _, dup := seen[src.Name]; dup {
			return trace.BadParameter("duplicate fileGroup slot name %q (collides on the on-disk subdir)", src.Name)
		}
		seen[src.Name] = struct{}{}
		if isEmptyLabelSelector(src.Selector) {
			return trace.BadParameter("fileGroup source %q: selector must match on at least one label or expression (an empty selector would match every PVC in the namespace)", src.Name)
		}
	}
	return nil
}

func validateS3Sources(s3Sources []GenericS3Source) error {
	seen := make(map[string]struct{}, len(s3Sources))
	for _, src := range s3Sources {
		if err := validateSlotName("s3", src.Name); err != nil {
			return trace.Wrap(err)
		}
		if _, dup := seen[src.Name]; dup {
			return trace.BadParameter("duplicate s3 slot name %q (collides on the on-disk subdir)", src.Name)
		}
		seen[src.Name] = struct{}{}
		if src.Path == "" {
			return trace.BadParameter("s3 source %q: path is required", src.Name)
		}
		// Credentials are optional (empty => AWS env-var fallback), so nothing to validate beyond the path.
	}
	return nil
}

// Validate enforces the cross-field and per-source rules for a backup config.
func (c GenericBackupConfig) Validate() error {
	if len(c.Postgres)+len(c.Files)+len(c.FileGroups)+len(c.S3) == 0 {
		return trace.BadParameter("at least one source (postgres, files, fileGroups, or s3) must be configured")
	}

	pgNames := make(map[string]struct{}, len(c.Postgres))
	for _, src := range c.Postgres {
		if err := validateSlotName("postgres", src.Name); err != nil {
			return trace.Wrap(err)
		}
		if _, dup := pgNames[src.Name]; dup {
			return trace.BadParameter("duplicate postgres slot name %q (collides on the SQL dump file)", src.Name)
		}
		pgNames[src.Name] = struct{}{}
		if src.Cluster == "" {
			return trace.BadParameter("postgres source %q: cluster is required", src.Name)
		}
		// The clone's serving and client-CA certs are minted from an internally-created self-signed
		// issuer, so there is no issuer to require here.
	}

	filesSources := make([]GenericFilesSource, len(c.Files))
	for i := range c.Files {
		filesSources[i] = c.Files[i].GenericFilesSource
	}
	if err := validateFilesSources(filesSources); err != nil {
		return trace.Wrap(err)
	}
	for _, src := range c.Files {
		if err := src.FileFilter.Validate(); err != nil {
			return trace.Wrap(err, "files source %q has an invalid include/exclude filter", src.Name)
		}
	}

	fileGroupSources := make([]GenericFileGroupSource, len(c.FileGroups))
	for i := range c.FileGroups {
		fileGroupSources[i] = c.FileGroups[i].GenericFileGroupSource
	}
	if err := validateFileGroupSources(fileGroupSources); err != nil {
		return trace.Wrap(err)
	}
	for _, src := range c.FileGroups {
		if err := src.FileFilter.Validate(); err != nil {
			return trace.Wrap(err, "fileGroup source %q has an invalid include/exclude filter", src.Name)
		}
	}
	if err := validateS3Sources(c.S3); err != nil {
		return trace.Wrap(err)
	}

	// A files source contributes its source PVC's requested storage, but postgres, S3, and fileGroup sources
	// have no well-defined size contribution (a fileGroup's membership is selector-resolved and variable), so
	// size must be set explicitly whenever the config has any of them.
	if (len(c.Postgres) > 0 || len(c.S3) > 0 || len(c.FileGroups) > 0) && c.BackupVolume.Size.IsZero() {
		return trace.BadParameter("backupVolume.size is required when the config has postgres, s3, or fileGroup sources (their size cannot be inferred)")
	}

	return nil
}

// Validate enforces the cross-field and per-source rules for a restore config.
func (c GenericRestoreConfig) Validate() error {
	if len(c.Postgres)+len(c.Files)+len(c.FileGroups)+len(c.S3) == 0 {
		return trace.BadParameter("at least one source (postgres, files, fileGroups, or s3) must be configured")
	}

	pgNames := make(map[string]struct{}, len(c.Postgres))
	for _, src := range c.Postgres {
		if err := validateSlotName("postgres", src.Name); err != nil {
			return trace.Wrap(err)
		}
		if _, dup := pgNames[src.Name]; dup {
			return trace.BadParameter("duplicate postgres slot name %q (collides on the SQL dump file)", src.Name)
		}
		pgNames[src.Name] = struct{}{}
		if src.Cluster == "" {
			return trace.BadParameter("postgres source %q: cluster is required", src.Name)
		}
		if src.ClientCAIssuer.Name == "" {
			return trace.BadParameter("postgres source %q: clientCAIssuer.name is required", src.Name)
		}
		if src.ServingCert == "" {
			return trace.BadParameter("postgres source %q: servingCert is required", src.Name)
		}
	}

	if err := validateFilesSources(c.Files); err != nil {
		return trace.Wrap(err)
	}
	if err := validateFileGroupSources(c.FileGroups); err != nil {
		return trace.Wrap(err)
	}
	if err := validateS3Sources(c.S3); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

type GenericApp struct {
	kubeClusterClient kubecluster.ClientInterface
	// Testing injection
	newCNPGBackup        func() cnpgbackup.CNPGBackupInterface
	newCNPGRestore       func() cnpgrestore.CNPGRestoreInterface
	newFilesBackup       func() filesbackup.FilesBackupInterface
	newFilesRestore      func() filesrestore.FilesRestoreInterface
	newFilesGroupBackup  func() filesgroupbackup.FilesGroupBackupInterface
	newFilesGroupRestore func() filesgrouprestore.FilesGroupRestoreInterface
	newS3Sync            func() s3sync.S3SyncInterface
	newRemoteStage       func(kubeClusterClient kubecluster.ClientInterface, namespace, eventName string, opts remote.RemoteStageOptions) remote.RemoteStageInterface
}

func NewGenericApp(client kubecluster.ClientInterface) *GenericApp {
	return &GenericApp{
		kubeClusterClient:    client,
		newCNPGBackup:        cnpgbackup.NewCNPGBackup,
		newCNPGRestore:       cnpgrestore.NewCNPGRestore,
		newFilesBackup:       filesbackup.NewFilesBackup,
		newFilesRestore:      filesrestore.NewFilesRestore,
		newFilesGroupBackup:  filesgroupbackup.NewFilesGroupBackup,
		newFilesGroupRestore: filesgrouprestore.NewFilesGroupRestore,
		newS3Sync:            s3sync.NewS3Sync,
		newRemoteStage:       remote.NewRemoteStage,
	}
}

// dumpFileName is the on-disk SQL dump path for a postgres slot, derived from its slot name. Restore
// re-derives the same path, so this single rule is the backup<->restore contract for postgres dumps.
func dumpFileName(slotName string) string {
	return slotName + ".sql"
}

// resolveS3Credentials uses the inline credentials when supplied, otherwise the AWS environment variables.
func resolveS3Credentials(creds s3.Credentials) s3.CredentialsInterface {
	if creds == (s3.Credentials{}) {
		return s3.NewCredentialsFromEnv()
	}
	return &creds
}

// Backup captures every configured source into the DR volume and snapshots it. Sources are registered in
// a fixed kind order — postgres, then files, then fileGroups, then s3 — independent of their order in the
// config. This is consistency-load-bearing: the postgres base backups must precede the filesystem freezes
// (both files and fileGroups) that define the event's consistency point (see CLAUDE.md, RemoteStage
// consistency-point protocol).
func (g *GenericApp) Backup(ctx *contexts.Context, config GenericBackupConfig) (backup *DREvent, err error) {
	if err := config.Validate(); err != nil {
		return nil, trace.Wrap(err, "invalid backup configuration")
	}

	backup = NewDREventNow(config.BackupName)
	ctx.Log.With("backupName", backup.GetFullName(), "namespace", config.Namespace).Info("Starting backup process")
	defer func() {
		backup.Stop()
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Backup process failed", keyvals...)
		} else {
			ctx.Log.Info("Backup process completed", keyvals...)
		}
	}()

	ctx.Log.Step().Info("Ensuring DR volume exists")
	drVolumeSize, err := g.backupVolumeSize(ctx.Child(), config)
	if err != nil {
		return backup, trace.Wrap(err, "failed to determine the DR volume size")
	}

	clusterNames := make([]string, 0, len(config.Postgres))
	for _, src := range config.Postgres {
		clusterNames = append(clusterNames, src.Cluster)
	}

	drv, err := g.kubeClusterClient.NewDRVolume(ctx.Child(), config.Namespace, backup.Name, drVolumeSize, drvolume.DRVolumeCreateOptions{
		VolumeStorageClass: config.BackupVolume.StorageClass,
		CNPGClusterNames:   clusterNames,
	})
	if err != nil {
		return backup, trace.Wrap(err, "failed to create the DR volume")
	}

	ctx.Log.Step().Info("Configuring backup actions")
	stage := g.newRemoteStage(g.kubeClusterClient, config.Namespace, backup.GetFullName(), remote.RemoteStageOptions{
		CleanupTimeout: config.CleanupTimeout,
	})

	for _, src := range config.Postgres {
		action := g.newCNPGBackup()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, src.Cluster, backup.Name, dumpFileName(src.Name), cnpgbackup.CNPGBackupOptions{
			CloningOpts:    src.ClusterCloning,
			CleanupTimeout: config.CleanupTimeout,
		}); err != nil {
			return backup, trace.Wrap(err, "failed to configure postgres source %q backup", src.Name)
		}
		stage.WithAction(fmt.Sprintf("postgres %q backup", src.Name), action)
	}

	for _, src := range config.Files {
		action := g.newFilesBackup()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, src.PVC, backup.Name, src.Name, filesbackup.FilesBackupOptions{
			SnapshotClass:  src.SnapshotClass,
			Filter:         src.FileFilter,
			CleanupTimeout: config.CleanupTimeout,
		}); err != nil {
			return backup, trace.Wrap(err, "failed to configure files source %q backup", src.Name)
		}
		stage.WithAction(fmt.Sprintf("files %q backup", src.Name), action)
	}

	for _, src := range config.FileGroups {
		action := g.newFilesGroupBackup()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, src.Selector, backup.Name, src.Name, filesgroupbackup.FilesGroupBackupOptions{
			SnapshotClass:  src.SnapshotClass,
			Filter:         src.FileFilter,
			CleanupTimeout: config.CleanupTimeout,
		}); err != nil {
			return backup, trace.Wrap(err, "failed to configure fileGroup source %q backup", src.Name)
		}
		stage.WithAction(fmt.Sprintf("fileGroup %q backup", src.Name), action)
	}

	for _, src := range config.S3 {
		action := g.newS3Sync()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, backup.Name, src.Name, src.Path, resolveS3Credentials(src.Credentials), s3sync.DirectionDownload, s3sync.S3SyncOptions{}); err != nil {
			return backup, trace.Wrap(err, "failed to configure s3 source %q backup", src.Name)
		}
		stage.WithAction(fmt.Sprintf("s3 %q sync", src.Name), action)
	}

	ctx.Log.Step().Info("Running backup actions")
	if err := stage.Run(ctx.Child()); err != nil {
		return backup, trace.Wrap(err, "failed to run backup actions")
	}

	ctx.Log.Step()
	if err := drv.SnapshotAndWaitReady(ctx.Child(), backup.GetFullName(), drvolume.DRVolumeSnapshotAndWaitOptions{
		SnapshotClass: config.BackupVolume.SnapshotClass,
		ReadyTimeout:  config.BackupVolume.SnapshotReadyTimeout,
	}); err != nil {
		return backup, trace.Wrap(err, "failed to snapshot the backup volume")
	}

	return backup, nil
}

// backupVolumeSize returns the explicit backupVolume.size when set. Otherwise (only reachable for a
// files-only config — Validate requires an explicit size when postgres/s3/fileGroup sources are present) it
// sums the files sources' PVC requests and doubles the total, the same sizing the per-app Vaultwarden backup
// uses.
func (g *GenericApp) backupVolumeSize(ctx *contexts.Context, config GenericBackupConfig) (resource.Quantity, error) {
	if !config.BackupVolume.Size.IsZero() {
		return config.BackupVolume.Size, nil
	}

	total := resource.NewQuantity(0, resource.BinarySI)
	for _, src := range config.Files {
		pvc, err := g.kubeClusterClient.Core().GetPVC(ctx.Child(), config.Namespace, src.PVC)
		if err != nil {
			return resource.Quantity{}, trace.Wrap(err, "failed to get files source PVC %q to size the DR volume", src.PVC)
		}

		size, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			return resource.Quantity{}, trace.Errorf("files source PVC %q has no storage request to size the DR volume from", src.PVC)
		}
		total.Add(size)
	}

	total.Mul(2)
	return *total, nil
}

// Restore restores every configured source from the DR volume. The DR PVC named backupName must already
// exist in the namespace. Sources are registered in the same fixed kind order as Backup; restore actions
// are independent (no consistency point is established), so the order is purely for symmetry.
func (g *GenericApp) Restore(ctx *contexts.Context, config GenericRestoreConfig) (restore *DREvent, err error) {
	if err := config.Validate(); err != nil {
		return nil, trace.Wrap(err, "invalid restore configuration")
	}

	restore = NewDREventNow(config.BackupName)
	ctx.Log.With("restoreName", restore.GetFullName(), "namespace", config.Namespace).Info("Starting restore process")
	defer func() {
		restore.Stop()
		keyvals := []interface{}{ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err)}
		if err != nil {
			ctx.Log.Warn("Restore process failed", keyvals...)
		} else {
			ctx.Log.Info("Restore process completed", keyvals...)
		}
	}()

	ctx.Log.Step().Info("Configuring restoration actions")
	stage := g.newRemoteStage(g.kubeClusterClient, config.Namespace, restore.GetFullName(), remote.RemoteStageOptions{
		CleanupTimeout: config.CleanupTimeout,
	})

	for _, src := range config.Postgres {
		action := g.newCNPGRestore()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, src.Cluster, src.ServingCert, src.ClientCAIssuer, restore.Name, dumpFileName(src.Name), cnpgrestore.CNPGRestoreOptions{
			PostgresUserCert: src.PostgresUserCert,
			CleanupTimeout:   config.CleanupTimeout,
		}); err != nil {
			return restore, trace.Wrap(err, "failed to configure postgres source %q restoration", src.Name)
		}
		stage.WithAction(fmt.Sprintf("postgres %q restore", src.Name), action)
	}

	for _, src := range config.Files {
		action := g.newFilesRestore()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, src.PVC, restore.Name, src.Name, filesrestore.FilesRestoreOptions{}); err != nil {
			return restore, trace.Wrap(err, "failed to configure files source %q restoration", src.Name)
		}
		stage.WithAction(fmt.Sprintf("files %q restore", src.Name), action)
	}

	for _, src := range config.FileGroups {
		action := g.newFilesGroupRestore()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, src.Selector, restore.Name, src.Name, filesgrouprestore.FilesGroupRestoreOptions{}); err != nil {
			return restore, trace.Wrap(err, "failed to configure fileGroup source %q restoration", src.Name)
		}
		stage.WithAction(fmt.Sprintf("fileGroup %q restore", src.Name), action)
	}

	for _, src := range config.S3 {
		action := g.newS3Sync()
		if err := action.Configure(g.kubeClusterClient, config.Namespace, restore.Name, src.Name, src.Path, resolveS3Credentials(src.Credentials), s3sync.DirectionUpload, s3sync.S3SyncOptions{}); err != nil {
			return restore, trace.Wrap(err, "failed to configure s3 source %q restoration", src.Name)
		}
		stage.WithAction(fmt.Sprintf("s3 %q sync", src.Name), action)
	}

	ctx.Log.Step().Info("Running restoration actions")
	err = stage.Run(ctx.Child())
	return restore, trace.Wrap(err, "failed to run restoration actions")
}
