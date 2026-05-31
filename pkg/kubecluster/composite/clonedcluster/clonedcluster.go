package clonedcluster

import (
	"fmt"
	"path/filepath"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	postgres "github.com/solidDoWant/backup-tool/pkg/postgres"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ClonedClusterInterface interface {
	GetCredentials(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials
	Delete(ctx *contexts.Context) error
	setServingCert(cert *certmanagerv1.Certificate)
	GetServingCert() *certmanagerv1.Certificate
	setClientCACert(cert *certmanagerv1.Certificate)
	GetClientCACert() *certmanagerv1.Certificate
	setClientCAIssuer(issuer *certmanagerv1.Issuer)
	GetClientCAIssuer() *certmanagerv1.Issuer
	setPostgresUserCert(cuc clusterusercert.ClusterUserCertInterface)
	GetPostgresUserCert() clusterusercert.ClusterUserCertInterface
	setStreamingReplicaUserCert(cuc clusterusercert.ClusterUserCertInterface)
	GetStreamingReplicaUserCert() clusterusercert.ClusterUserCertInterface
	setCluster(cluster *apiv1.Cluster)
	GetCluster() *apiv1.Cluster
}

type ClonedCluster struct {
	p                               providerInterfaceInternal
	cluster                         *apiv1.Cluster
	servingCertificate              *certmanagerv1.Certificate
	clientCACertificate             *certmanagerv1.Certificate
	clientCAIssuer                  *certmanagerv1.Issuer
	postgresUserCertificate         clusterusercert.ClusterUserCertInterface
	streamingReplicaUserCertificate clusterusercert.ClusterUserCertInterface
}

type CloneClusterOptionsCertificate struct {
	Subject             *certmanagerv1.X509Subject `yaml:"subject,omitempty"`
	WaitForReadyTimeout helpers.MaxWaitTime        `yaml:"waitForReadyTimeout,omitempty"`
}

// Describes options for a certificate that is issued by an issuer created by the backup tool.
type CloneClusterOptionsInternallyIssuedCertificate struct {
	CloneClusterOptionsCertificate `yaml:",inline"`
	CRPOpts                        clusterusercert.NewClusterUserCertOptsCRP `yaml:"certificateRequestPolicy,omitempty"`
}

// Describes options for a certificate that is issued by an issuer that was not created by the backup tool.
type CloneClusterOptionsExternallyIssuedCertificate struct {
	CloneClusterOptionsCertificate `yaml:",inline"`
	IssuerKind                     string `yaml:"issuerKind,omitempty"`
}

type CloneClusterOptionsCertificates struct {
	ServingCert              CloneClusterOptionsExternallyIssuedCertificate `yaml:"servingCert,omitempty"`
	ClientCACert             CloneClusterOptionsExternallyIssuedCertificate `yaml:"clientCACert,omitempty"`
	PostgresUserCert         CloneClusterOptionsInternallyIssuedCertificate `yaml:"postgresUserCert,omitempty"`
	StreamingReplicaUserCert CloneClusterOptionsInternallyIssuedCertificate `yaml:"streamingReplicaUserCert,omitempty"`
}

type CloneClusterOptionsCAIssuer struct {
	WaitForReadyTimeout helpers.MaxWaitTime `yaml:"waitForReadyTimeout,omitempty"`
}

type CloneClusterOptions struct {
	WaitForBackupTimeout  helpers.MaxWaitTime             `yaml:"waitForBackupTimeout,omitempty"`
	Certificates          CloneClusterOptionsCertificates `yaml:"certificates,omitempty"`
	ClientCAIssuer        CloneClusterOptionsCAIssuer     `yaml:"clientCAIssuer,omitempty"`
	RecoveryTargetTime    string                          `yaml:"recoveryTargetTime,omitempty" jsonschema:"description=The time to roll back to in RFC3339 format"`
	WaitForClusterTimeout helpers.MaxWaitTime             `yaml:"waitForClusterTimeout,omitempty"`
	CleanupTimeout        helpers.MaxWaitTime             `yaml:"cleanupTimeout,omitempty"`
	// TODO maybe provide an option for additional client auth CAs?
}

func newClonedCluster(p providerInterfaceInternal) ClonedClusterInterface {
	return &ClonedCluster{p: p}
}

// Clone an existing CNPG cluster, with separate certificates for authentication.
// It is assumed that all required resources for approving certificats (such as Certificate Request Policies) are already in place.
func (p *Provider) CloneCluster(ctx *contexts.Context, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCACertIssuerName string, opts CloneClusterOptions) (cluster ClonedClusterInterface, err error) {
	if len(newClusterName) > 40 { // Max length that CNPG allows for cloned cluster names, see https://github.com/cloudnative-pg/cloudnative-pg/pull/6755
		return nil, trace.Errorf("newClusterName must be 40 characters or less")
	}

	ctx.Log.With("existingCluster", existingClusterName, "newCluster", newClusterName).Info("Cloning CNPG cluster")
	defer ctx.Log.Info("Finished cloning CNPG cluster", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	cluster = p.newClonedCluster()

	// Prepare to handle resource cleanup in the event of an error
	errHandler := func(originalErr error, args ...interface{}) (*ClonedCluster, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.To(cluster.Delete).
			WithErrMessage("failed to cleanup cloned cluster %q in namespace %q", newClusterName, namespace).
			WithOriginalErr(&originalErr).
			WithParentCtx(ctx).
			WithTimeout(opts.CleanupTimeout.MaxWait(10 * time.Minute)).
			Run()
	}

	// Perform as many read-only operations as possible now to reduce the number of changes that need to be reverted in case of a failure
	ctx.Log.Info("Collecting information about the existing cluster")
	existingCluster, err := p.cnpgClient.GetCluster(ctx.Child(), namespace, existingClusterName)
	if err != nil {
		return errHandler(err, "failed to get existing cluster %q", helpers.FullNameStr(namespace, existingClusterName))
	}

	clusterVolumeSize, err := resource.ParseQuantity(existingCluster.Spec.StorageConfiguration.Size)
	if err != nil {
		return errHandler(err, "failed to parse the existing cluster %q storage volume size %q", helpers.FullName(existingCluster), existingCluster.Spec.StorageConfiguration.Size)
	}

	// 1. Create a backup of the current cluster
	backupNamePrefix := existingClusterName + "-cloned"
	ctx.Log.Step().Info("Creating a backup of the existing cluster", "backupName", backupNamePrefix)
	backup, err := p.cnpgClient.CreateBackup(ctx.Child(), namespace, backupNamePrefix, existingClusterName, cnpg.CreateBackupOptions{GenerateName: true})
	if err != nil {
		return errHandler(err, "failed to create backup of existing cluster %q", helpers.FullNameStr(namespace, existingClusterName))
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		// A child context is not passed here because this is mostly just a wrapper for DeleteBackup.
		cleanupErr := p.cnpgClient.DeleteBackup(ctx, namespace, backup.Name)
		if cleanupErr == nil {
			return nil
		}

		// If backup deletion failed, treat the entire operation as a failure. This includes deleting the cluster, and setting the return value to nil.
		cluster, cleanupErr = errHandler(cleanupErr, "failed to delete backup %q", helpers.FullName(backup))
		return cleanupErr
	}).WithErrMessage("cleanup failed").WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	readyBackup, err := p.cnpgClient.WaitForReadyBackup(ctx.Child(), namespace, backup.Name, cnpg.WaitForReadyBackupOpts{MaxWaitTime: opts.WaitForBackupTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for backup %q to be ready", helpers.FullName(backup))
	}

	// 2. Create the serving certificate (short lived)
	servingCertNameSuffix := "-serving-cert"
	servingCertName := helpers.CleanName(helpers.TruncateStringEllipsis(newClusterName, 64-len(servingCertNameSuffix)) + servingCertNameSuffix)
	ctx.Log.Step().Info("Creating serving certificate for the new cluster", "certificateName", servingCertName)
	certOptions := certmanager.CreateCertificateOptions{
		CommonName: servingCertName,
		Subject:    opts.Certificates.ServingCert.Subject,
		Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth},
		SecretLabels: map[string]string{
			utils.WatchedLabelName: "true",
		},
		DNSNames:   getClusterDomainNames(ctx.Child(), newClusterName, namespace),
		IssuerKind: opts.Certificates.ServingCert.IssuerKind,
	}

	servingCert, err := p.cmClient.CreateCertificate(ctx.Child(), namespace, servingCertName, servingCertIssuerName, certOptions)
	if err != nil {
		return errHandler(err, "failed to create cluster serving cert %q", helpers.FullNameStr(namespace, servingCertName))
	}
	cluster.setServingCert(servingCert)

	readyServingCert, err := p.cmClient.WaitForReadyCertificate(ctx.Child(), namespace, servingCertName, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: opts.Certificates.ServingCert.WaitForReadyTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for serving certificate %q to be ready", helpers.FullName(servingCert))
	}
	cluster.setServingCert(readyServingCert)

	// 3. Create the client CA certificate (short lived) and issuer
	clientCACertName := helpers.CleanName(newClusterName + "-client-ca")
	clientCAIssuerName := helpers.CleanName(clientCACertName + "-issuer")
	ctx.Log.Step().Info("Creating PKI for client certificates for the new cluster", "certificateName", clientCACertName, "issuerName", clientCAIssuerName)

	// 3.1 Client CA certificate
	clientCACNSuffix := " CNPC CA"
	certOptions = certmanager.CreateCertificateOptions{
		IsCA: true,
		// Permit nothing. The certs should only be authoritive for the common name, which stores the postgres username.
		CAConstraints: &certmanagerv1.NameConstraints{
			Critical: true,
			Excluded: &certmanagerv1.NameConstraintItem{
				DNSDomains:     []string{},
				IPRanges:       []string{},
				EmailAddresses: []string{},
				URIDomains:     []string{},
			},
		},
		CommonName: helpers.TruncateStringEllipsis(newClusterName, 64-len(clientCACNSuffix)) + clientCACNSuffix,
		Subject:    opts.Certificates.ClientCACert.Subject,
		Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageCertSign},
		SecretLabels: map[string]string{
			utils.WatchedLabelName: "true",
		},
		IssuerKind: opts.Certificates.ClientCACert.IssuerKind,
	}

	clientCACert, err := p.cmClient.CreateCertificate(ctx.Child(), namespace, clientCACertName, clientCACertIssuerName, certOptions)
	if err != nil {
		return errHandler(err, "failed to create client CA cert %q", helpers.FullNameStr(namespace, clientCACertName))
	}
	cluster.setClientCACert(clientCACert)

	readyClientCACert, err := p.cmClient.WaitForReadyCertificate(ctx.Child(), namespace, clientCACertName, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: opts.Certificates.ClientCACert.WaitForReadyTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for client CA cert %q to be ready", helpers.FullName(readyServingCert))
	}
	cluster.setClientCACert(readyClientCACert)

	// 3.2 Client CA issuer
	clientCAIssuer, err := p.cmClient.CreateIssuer(ctx.Child(), namespace, clientCAIssuerName, readyClientCACert.Name, certmanager.CreateIssuerOptions{})
	if err != nil {
		return errHandler(err, "failed to create client CA issuer %q", helpers.FullNameStr(namespace, clientCAIssuerName))
	}
	cluster.setClientCAIssuer(clientCAIssuer)

	readyClientCAIssuer, err := p.cmClient.WaitForReadyIssuer(ctx.Child(), namespace, clientCAIssuerName, certmanager.WaitForReadyIssuerOpts{MaxWaitTime: opts.ClientCAIssuer.WaitForReadyTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for client CA issuer %q to be ready", helpers.FullName(clientCAIssuer))
	}
	cluster.setClientCAIssuer(readyClientCAIssuer)

	// 4. Create the user certificates
	ctx.Log.Step().Info("Creating user certificates for the new cluster")

	// 4.1 Create the postgres user certificate
	cucOptions := clusterusercert.NewClusterUserCertOpts{
		Subject:            opts.Certificates.PostgresUserCert.Subject,
		CRPOpts:            opts.Certificates.PostgresUserCert.CRPOpts,
		WaitForCertTimeout: opts.Certificates.PostgresUserCert.WaitForReadyTimeout,
		CleanupTimeout:     opts.CleanupTimeout,
	}
	postgresUserCert, err := p.cucp.NewClusterUserCert(ctx.Child(), namespace, "postgres", clientCAIssuerName, newClusterName, cucOptions)
	if err != nil {
		return errHandler(err, "failed to create postgres user cert resources")
	}
	cluster.setPostgresUserCert(postgresUserCert)

	// 4.2 Create the streaming_replica user certificate
	cucOptions = clusterusercert.NewClusterUserCertOpts{
		Subject:            opts.Certificates.StreamingReplicaUserCert.Subject,
		CRPOpts:            opts.Certificates.StreamingReplicaUserCert.CRPOpts,
		WaitForCertTimeout: opts.Certificates.StreamingReplicaUserCert.WaitForReadyTimeout,
		CleanupTimeout:     opts.CleanupTimeout,
	}
	replicationUserCert, err := p.cucp.NewClusterUserCert(ctx.Child(), namespace, "streaming_replica", clientCAIssuerName, newClusterName, cucOptions)
	if err != nil {
		return errHandler(err, "failed to create streaming_replica user cert resources")
	}
	cluster.setStreamingReplicaUserCert(replicationUserCert)

	// 5. Create a new cluster from the backup
	ctx.Log.Step().Info("Creating new cluster from backup")
	clusterOpts := cnpg.CreateClusterOptions{
		ImageName: existingCluster.Spec.ImageName,
	}

	if existingCluster.Spec.StorageConfiguration.StorageClass != nil {
		clusterOpts.StorageClass = *existingCluster.Spec.StorageConfiguration.StorageClass
	}

	if opts.RecoveryTargetTime != "" {
		clusterOpts.RecoveryTarget = &apiv1.RecoveryTarget{
			TargetTime: opts.RecoveryTargetTime,
		}
	}

	if existingCluster.Spec.Bootstrap != nil {
		if existingCluster.Spec.Bootstrap.InitDB != nil {
			clusterOpts.DatabaseName = existingCluster.Spec.Bootstrap.InitDB.Database
			clusterOpts.OwnerName = existingCluster.Spec.Bootstrap.InitDB.Owner
		} else if existingCluster.Spec.Bootstrap.PgBaseBackup != nil {
			clusterOpts.DatabaseName = existingCluster.Spec.Bootstrap.PgBaseBackup.Database
			clusterOpts.OwnerName = existingCluster.Spec.Bootstrap.PgBaseBackup.Owner
		} else if existingCluster.Spec.Bootstrap.Recovery != nil {
			clusterOpts.DatabaseName = existingCluster.Spec.Bootstrap.Recovery.Database
			clusterOpts.OwnerName = existingCluster.Spec.Bootstrap.Recovery.Owner
		}
	}

	// Configure how the new cluster sources WAL during recovery, matching the WAL archiving method
	// used by the source cluster (the barman-cloud plugin, or the deprecated in-tree barman support).
	if err := p.configureWALRecovery(ctx.Child(), namespace, existingCluster, readyBackup, &clusterOpts); err != nil {
		return errHandler(err, "failed to configure WAL recovery for new cluster %q", helpers.FullNameStr(namespace, newClusterName))
	}

	newCluster, err := p.cnpgClient.CreateCluster(ctx.Child(), namespace, newClusterName, clusterVolumeSize, readyServingCert.Name, readyClientCACert.Name, replicationUserCert.GetCertificate().Name, clusterOpts)
	if err != nil {
		return errHandler(err, "failed to create new cluster %q from backup %q with serving certificate %q and client certificate %q",
			helpers.FullNameStr(namespace, newClusterName), helpers.FullName(readyBackup), helpers.FullName(readyServingCert), helpers.FullName(readyClientCACert))
	}
	cluster.setCluster(newCluster)

	readyCluster, err := p.cnpgClient.WaitForReadyCluster(ctx.Child(), namespace, newClusterName, cnpg.WaitForReadyClusterOpts{MaxWaitTime: opts.WaitForClusterTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for new cluster %q to become ready", helpers.FullNameStr(namespace, newClusterName))
	}
	cluster.setCluster(readyCluster)

	return cluster, nil
}

// configureWALRecovery sets the recovery source on clusterOpts based on how the source cluster
// archives write-ahead logs. Source clusters using the barman-cloud plugin recover from the
// backup's volume snapshots, fetching WAL via an external cluster that references the same object
// store. Source clusters using the deprecated in-tree barman support recover from the Backup
// object directly, which carries the WAL archive location.
func (p *Provider) configureWALRecovery(ctx *contexts.Context, namespace string, sourceCluster *apiv1.Cluster, backup *apiv1.Backup, clusterOpts *cnpg.CreateClusterOptions) error {
	walArchiverPlugin := findBarmanCloudWALArchiver(sourceCluster)
	if walArchiverPlugin == nil {
		// No barman-cloud plugin: the source uses the deprecated in-tree barman WAL archiving, which
		// records the WAL archive location on the Backup object itself, so recover straight from it.
		ctx.Log.Debug("Source cluster uses in-tree barman WAL archiving; recovering from backup object", "backup", backup.Name)
		clusterOpts.BackupName = backup.Name
		return nil
	}

	ctx.Log.Debug("Source cluster uses the barman-cloud plugin; recovering from volume snapshots", "plugin", walArchiverPlugin.Name)

	objectStoreName := walArchiverPlugin.Parameters["barmanObjectName"]
	if objectStoreName == "" {
		return trace.Errorf("barman-cloud plugin on cluster %q is missing the %q parameter", helpers.FullName(sourceCluster), "barmanObjectName")
	}

	serverName, err := p.resolveBarmanServerName(ctx, namespace, sourceCluster.Name, walArchiverPlugin, objectStoreName)
	if err != nil {
		return trace.Wrap(err, "failed to resolve barman server name for cluster %q", helpers.FullName(sourceCluster))
	}

	dataSnapshotName, walSnapshotName := getBackupSnapshotNames(backup)
	if dataSnapshotName == "" {
		return trace.Errorf("backup %q did not report a PG_DATA volume snapshot to recover from", helpers.FullName(backup))
	}

	clusterOpts.VolumeSnapshotRecovery = &cnpg.VolumeSnapshotRecovery{
		DataSnapshotName: dataSnapshotName,
		WALSnapshotName:  walSnapshotName,
		WALSource: apiv1.ExternalCluster{
			Name: serverName,
			PluginConfiguration: &apiv1.PluginConfiguration{
				Name: cnpg.BarmanCloudPluginName,
				Parameters: map[string]string{
					"barmanObjectName": objectStoreName,
					"serverName":       serverName,
				},
			},
		},
	}

	// Recover to the backup's consistency point (targetImmediate) rather than the caller's wall-clock
	// RecoveryTargetTime. This matches the deprecated in-tree path: CNPG's volume-snapshot recovery
	// from a Backup object (recovery.backup) ignores recoveryTarget.targetTime and stops at the
	// consistency point, so honoring a wall-clock target here would both diverge from in-tree and, on
	// a quiescent source, fail outright ("recovery ended before configured recovery target was
	// reached", because no transaction commits at/after the target). WAL is still replayed from the
	// source's object store, up to the consistency point, via the plugin external cluster configured
	// above; it just isn't carried past the backup.
	//
	// Known limitation: the consistency point is the instant this CNPG base backup completed, which is
	// not aligned with the filesystem/S3 captures elsewhere in the DR event, so a restore is not
	// guaranteed to be point-in-time consistent across resources. Aligning them (true PITR on the
	// plugin path) is a separate work item; see docs/cross-resource-pitr-consistency.md.
	clusterOpts.RecoveryTarget = &apiv1.RecoveryTarget{TargetImmediate: new(true)}
	return nil
}

// resolveBarmanServerName determines the server name (the folder under which the source cluster's
// backups are stored in the object store) that the recovering cluster must point at. The barman-cloud
// plugin resolves this from the plugin's serverName parameter, then the ObjectStore's configured
// server name, and finally defaults to the cluster name.
func (p *Provider) resolveBarmanServerName(ctx *contexts.Context, namespace, clusterName string, plugin *apiv1.PluginConfiguration, objectStoreName string) (string, error) {
	if serverName := plugin.Parameters["serverName"]; serverName != "" {
		return serverName, nil
	}

	objectStore, err := p.barmanCloudClient.GetObjectStore(ctx, namespace, objectStoreName)
	if err != nil {
		return "", trace.Wrap(err, "failed to get barman-cloud object store %q", helpers.FullNameStr(namespace, objectStoreName))
	}

	if serverName := objectStore.Spec.Configuration.ServerName; serverName != "" {
		return serverName, nil
	}

	return clusterName, nil
}

// findBarmanCloudWALArchiver returns the source cluster's barman-cloud WAL archiver plugin
// configuration, or nil if the cluster does not use the plugin (i.e. it uses in-tree barman, or no
// WAL archiving at all). It prefers the plugin explicitly marked as the WAL archiver.
func findBarmanCloudWALArchiver(cluster *apiv1.Cluster) *apiv1.PluginConfiguration {
	var barmanCloudPlugin *apiv1.PluginConfiguration
	for i := range cluster.Spec.Plugins {
		plugin := &cluster.Spec.Plugins[i]
		if plugin.Name != cnpg.BarmanCloudPluginName {
			continue
		}

		if plugin.IsWALArchiver != nil && *plugin.IsWALArchiver {
			return plugin
		}

		if barmanCloudPlugin == nil {
			barmanCloudPlugin = plugin
		}
	}
	return barmanCloudPlugin
}

// getBackupSnapshotNames extracts the PGDATA and (optional) PG_WAL volume snapshot names from a
// completed volumeSnapshot-method backup.
func getBackupSnapshotNames(backup *apiv1.Backup) (dataSnapshotName, walSnapshotName string) {
	for _, element := range backup.Status.BackupSnapshotStatus.Elements {
		switch element.Type {
		case string(utils.PVCRolePgData):
			dataSnapshotName = element.Name
		case string(utils.PVCRolePgWal):
			walSnapshotName = element.Name
		}
	}
	return dataSnapshotName, walSnapshotName
}

func getClusterDomainNames(ctx *contexts.Context, clusterName, namespace string) []string {
	endpointTypes := []string{"r", "ro", "rw"}
	// Pending https://github.com/kubernetes/kubernetes/issues/44954 there is no way to get the cluster domain name (i.e. cluster.local)
	// from the k8s API
	parentDomainComponents := []string{"", "." + namespace, ".svc"} // ".cluster.local"

	domainNames := make([]string, 0, len(endpointTypes)*len(parentDomainComponents))
	for _, endpointType := range endpointTypes {
		// Example: my-cluster-r, my-cluster-ro, my-cluster-rw
		subdomain := fmt.Sprintf("%s-%s", clusterName, endpointType)
		parentDomain := ""
		for _, parentDomainComponent := range parentDomainComponents {
			// Example: "", ".my-namespace", ".my-namespace.svc"
			parentDomain += parentDomainComponent
			domainNames = append(domainNames, subdomain+parentDomain)
		}
	}

	ctx.Log.Debug("Generated domain names", "domainNames", domainNames)
	return domainNames
}

func (cc *ClonedCluster) setServingCert(cert *certmanagerv1.Certificate) {
	cc.servingCertificate = cert
}

func (cc *ClonedCluster) GetServingCert() *certmanagerv1.Certificate {
	return cc.servingCertificate
}

func (cc *ClonedCluster) setClientCACert(cert *certmanagerv1.Certificate) {
	cc.clientCACertificate = cert
}

func (cc *ClonedCluster) GetClientCACert() *certmanagerv1.Certificate {
	return cc.clientCACertificate
}

func (cc *ClonedCluster) setClientCAIssuer(issuer *certmanagerv1.Issuer) {
	cc.clientCAIssuer = issuer
}

func (cc *ClonedCluster) GetClientCAIssuer() *certmanagerv1.Issuer {
	return cc.clientCAIssuer
}

func (cc *ClonedCluster) setPostgresUserCert(postgresUserCertificate clusterusercert.ClusterUserCertInterface) {
	cc.postgresUserCertificate = postgresUserCertificate
}

func (cc *ClonedCluster) GetPostgresUserCert() clusterusercert.ClusterUserCertInterface {
	return cc.postgresUserCertificate
}

func (cc *ClonedCluster) setStreamingReplicaUserCert(streamingReplicaUserCertificate clusterusercert.ClusterUserCertInterface) {
	cc.streamingReplicaUserCertificate = streamingReplicaUserCertificate
}

func (cc *ClonedCluster) GetStreamingReplicaUserCert() clusterusercert.ClusterUserCertInterface {
	return cc.streamingReplicaUserCertificate
}

func (cc *ClonedCluster) setCluster(cluster *apiv1.Cluster) {
	cc.cluster = cluster
}

func (cc *ClonedCluster) GetCluster() *apiv1.Cluster {
	return cc.cluster
}

func (cc *ClonedCluster) GetCredentials(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials {
	return &cnpg.KubernetesSecretCredentials{
		Host:                         fmt.Sprintf("%s.%s.svc", cc.cluster.GetServiceReadWriteName(), cc.cluster.Namespace),
		ServingCertificateCAFilePath: filepath.Join(servingCertMountDirectory, "tls.crt"),
		ClientCertificateFilePath:    filepath.Join(clientCertMountDirectory, "tls.crt"),
		ClientPrivateKeyFilePath:     filepath.Join(clientCertMountDirectory, "tls.key"),
	}
}

func (cc *ClonedCluster) Delete(ctx *contexts.Context) (err error) {
	ctx.Log.Info("Cleaning up cloned cluster resources")
	defer ctx.Log.Info("Finished cleaning up cloned cluster resources", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	cleanupErrs := make([]error, 0, 6)

	if cc.cluster != nil {
		err := cc.p.cnpg().DeleteCluster(ctx.Child(), cc.cluster.Namespace, cc.cluster.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster CNPG cluster %q", helpers.FullName(cc.cluster)))
		}
	}

	if cc.streamingReplicaUserCertificate != nil {
		err := cc.streamingReplicaUserCertificate.Delete(ctx.Child())
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster streaming_replica user cert resources"))
		}
	}

	if cc.postgresUserCertificate != nil {
		err := cc.postgresUserCertificate.Delete(ctx.Child())
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster postgres user cert resources"))
		}
	}

	if cc.clientCAIssuer != nil {
		err := cc.p.cm().DeleteIssuer(ctx.Child(), cc.clientCAIssuer.Namespace, cc.clientCAIssuer.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster client CA issuer %q", helpers.FullName(cc.clientCAIssuer)))
		}
	}

	if cc.clientCACertificate != nil {
		err := cc.p.cm().DeleteCertificate(ctx.Child(), cc.clientCACertificate.Namespace, cc.clientCACertificate.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster client CA cert %q", helpers.FullName(cc.clientCACertificate)))
		}
	}

	if cc.servingCertificate != nil {
		err := cc.p.cm().DeleteCertificate(ctx.Child(), cc.servingCertificate.Namespace, cc.servingCertificate.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster serving cert %q", helpers.FullName(cc.servingCertificate)))
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed while cleaning up cloned cluster")
}
