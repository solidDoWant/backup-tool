package clonedcluster

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/createcrpforcertificate"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	postgres "github.com/solidDoWant/backup-tool/pkg/postgres"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ErrRecoveryTargetNotReached indicates a targetTime recovery's Job exhausted its retries without
// reaching the target, so the source had no WAL at or after the target (an idle database) and the
// target is unreachable from archived WAL. The clone should instead be recovered to the backup's
// consistency point (targetImmediate).
var ErrRecoveryTargetNotReached = errors.New("recovery target not reachable from archived WAL")

type ClonedClusterInterface interface {
	GetCredentials(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials
	Delete(ctx *contexts.Context) error
	setSelfSignedIssuer(issuer *certmanagerv1.Issuer)
	GetSelfSignedIssuer() *certmanagerv1.Issuer
	addCertificateRequestPolicy(crp *policyv1alpha1.CertificateRequestPolicy)
	GetCertificateRequestPolicies() []*policyv1alpha1.CertificateRequestPolicy
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
	selfSignedIssuer                *certmanagerv1.Issuer
	servingCertificate              *certmanagerv1.Certificate
	clientCACertificate             *certmanagerv1.Certificate
	clientCAIssuer                  *certmanagerv1.Issuer
	postgresUserCertificate         clusterusercert.ClusterUserCertInterface
	streamingReplicaUserCertificate clusterusercert.ClusterUserCertInterface
	// certificateRequestPolicies are the CRPs created for the serving and client-CA certs (the user
	// certs own their own CRPs via clusterusercert). Tracked here so Delete can tear them down.
	certificateRequestPolicies []*policyv1alpha1.CertificateRequestPolicy
}

// CloneClusterOptionsCertificate describes options for one of the clone's certificates. Every cert the
// clone creates is issued by an issuer the backup tool creates (the serving and client-CA certs from an
// internal self-signed issuer; the user certs from the client-CA issuer), so each gets a tight
// CertificateRequestPolicy when (and only when) approver-policy is detected as enforcing on the cluster.
type CloneClusterOptionsCertificate struct {
	Subject             *certmanagerv1.X509Subject                `yaml:"subject,omitempty"`
	CRPOpts             clusterusercert.NewClusterUserCertOptsCRP `yaml:"certificateRequestPolicy,omitempty"`
	WaitForReadyTimeout helpers.MaxWaitTime                       `yaml:"waitForReadyTimeout,omitempty"`
}

type CloneClusterOptionsCertificates struct {
	ServingCert              CloneClusterOptionsCertificate `yaml:"servingCert,omitempty"`
	ClientCACert             CloneClusterOptionsCertificate `yaml:"clientCACert,omitempty"`
	PostgresUserCert         CloneClusterOptionsCertificate `yaml:"postgresUserCert,omitempty"`
	StreamingReplicaUserCert CloneClusterOptionsCertificate `yaml:"streamingReplicaUserCert,omitempty"`
}

type CloneClusterOptionsCAIssuer struct {
	WaitForReadyTimeout helpers.MaxWaitTime `yaml:"waitForReadyTimeout,omitempty"`
}

type CloneClusterOptions struct {
	WaitForBackupTimeout  helpers.MaxWaitTime             `yaml:"waitForBackupTimeout,omitempty"`
	SelfSignedIssuer      CloneClusterOptionsCAIssuer     `yaml:"selfSignedIssuer,omitempty"`
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

// CreateClusterBackup takes a volume-snapshot backup of an existing cluster and waits for it to
// complete. The returned backup marks the consistency point. The caller owns the backup's lifecycle
// and must DeleteBackup it once the recovering clone has been created — the recovery volume snapshots
// are owned by the backup, so deleting it earlier would remove them.
func (p *Provider) CreateClusterBackup(ctx *contexts.Context, namespace, existingClusterName string, opts CloneClusterOptions) (backup *apiv1.Backup, err error) {
	backupNamePrefix := existingClusterName + "-cloned"
	ctx.Log.With("existingCluster", existingClusterName).Info("Creating a backup of the existing cluster", "backupName", backupNamePrefix)
	defer ctx.Log.Info("Finished backing up the existing cluster", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	backup, err = p.cnpgClient.CreateBackup(ctx.Child(), namespace, backupNamePrefix, existingClusterName, cnpg.CreateBackupOptions{GenerateName: true})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create backup of existing cluster %q", helpers.FullNameStr(namespace, existingClusterName))
	}

	// If waiting for the backup fails, delete the backup we just created so it isn't leaked (on
	// success the caller owns it).
	createdBackupName := backup.Name
	defer func() {
		if err == nil {
			return
		}
		cleanup.To(func(ctx *contexts.Context) error {
			return p.cnpgClient.DeleteBackup(ctx, namespace, createdBackupName)
		}).WithErrMessage("failed to delete backup %q", helpers.FullNameStr(namespace, createdBackupName)).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()
	}()

	readyBackup, err := p.cnpgClient.WaitForReadyBackup(ctx.Child(), namespace, backup.Name, cnpg.WaitForReadyBackupOpts{MaxWaitTime: opts.WaitForBackupTimeout})
	if err != nil {
		return nil, trace.Wrap(err, "failed to wait for backup %q to be ready", helpers.FullNameStr(namespace, createdBackupName))
	}

	return readyBackup, nil
}

// CloneClusterFromBackup creates a new cluster that recovers from a previously-created (ready) backup,
// with its own short-lived certificates. When a RecoveryTargetTime is supplied, the clone recovers
// forward to that wall-clock instant (recoveryTarget.targetTime); if the source had no WAL at/after the
// target (an idle database), it falls back to the backup's consistency point. Otherwise it recovers to
// the consistency point directly.
func (p *Provider) CloneClusterFromBackup(ctx *contexts.Context, namespace, existingClusterName, newClusterName string, readyBackup *apiv1.Backup, opts CloneClusterOptions) (cluster ClonedClusterInterface, err error) {
	if len(newClusterName) > 40 { // Max length that CNPG allows for cloned cluster names, see https://github.com/cloudnative-pg/cloudnative-pg/pull/6755
		return nil, trace.Errorf("newClusterName must be 40 characters or less")
	}

	ctx.Log.With("existingCluster", existingClusterName, "newCluster", newClusterName).Info("Creating cloned cluster from backup")
	defer ctx.Log.Info("Finished creating cloned cluster from backup", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	cluster = p.newClonedCluster()

	// Prepare to handle resource cleanup in the event of an error
	errHandler := func(originalErr error, args ...any) (*ClonedCluster, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.To(cluster.Delete).
			WithErrMessage("failed to cleanup cloned cluster %q in namespace %q", newClusterName, namespace).
			WithOriginalErr(&originalErr).
			WithParentCtx(ctx).
			WithTimeout(opts.CleanupTimeout.MaxWait(10 * time.Minute)).
			RunError()
	}

	ctx.Log.Info("Collecting information about the existing cluster")
	existingCluster, err := p.cnpgClient.GetCluster(ctx.Child(), namespace, existingClusterName)
	if err != nil {
		return errHandler(err, "failed to get existing cluster %q", helpers.FullNameStr(namespace, existingClusterName))
	}

	clusterVolumeSize, err := resource.ParseQuantity(existingCluster.Spec.StorageConfiguration.Size)
	if err != nil {
		return errHandler(err, "failed to parse the existing cluster %q storage volume size %q", helpers.FullName(existingCluster), existingCluster.Spec.StorageConfiguration.Size)
	}

	// 1. Create the self-signed issuer that mints the clone's short-lived root certificates: its serving
	// cert and the client-auth CA cert. A self-signed cert-manager issuer holds no key of its own (it
	// signs each request with that certificate's own key), so a single issuer backs both certs with no
	// downside. It is internal to this clone — created here and torn down with it — so no issuer needs
	// to be supplied.
	selfSignedIssuerName := helpers.CleanName(newClusterName + "-self-signed-issuer")
	ctx.Log.Step().Info("Creating self-signed issuer for the new cluster", "issuerName", selfSignedIssuerName)
	selfSignedIssuer, err := p.cmClient.CreateIssuer(ctx.Child(), namespace, selfSignedIssuerName, certmanagerv1.IssuerConfig{SelfSigned: &certmanagerv1.SelfSignedIssuer{}}, certmanager.CreateIssuerOptions{})
	if err != nil {
		return errHandler(err, "failed to create self-signed issuer %q", helpers.FullNameStr(namespace, selfSignedIssuerName))
	}
	cluster.setSelfSignedIssuer(selfSignedIssuer)

	readySelfSignedIssuer, err := p.cmClient.WaitForReadyIssuer(ctx.Child(), namespace, selfSignedIssuerName, certmanager.WaitForReadyIssuerOpts{MaxWaitTime: opts.SelfSignedIssuer.WaitForReadyTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for self-signed issuer %q to be ready", helpers.FullName(selfSignedIssuer))
	}
	cluster.setSelfSignedIssuer(readySelfSignedIssuer)
	selfSignedIssuerRef := cmmeta.IssuerReference{Name: readySelfSignedIssuer.Name}

	// 2. Create the serving certificate (short lived), minted from the self-signed issuer.
	servingCertNameSuffix := "-serving-cert"
	servingCertName := helpers.CleanName(helpers.TruncateStringEllipsis(newClusterName, 64-len(servingCertNameSuffix)) + servingCertNameSuffix)
	ctx.Log.Step().Info("Creating serving certificate for the new cluster", "certificateName", servingCertName)
	servingCertOptions := certmanager.CreateCertificateOptions{
		CommonName: servingCertName,
		Subject:    opts.Certificates.ServingCert.Subject,
		Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth},
		SecretLabels: map[string]string{
			utils.WatchedLabelName: "true",
		},
		DNSNames: getClusterDomainNames(ctx.Child(), newClusterName, namespace),
	}

	readyServingCert, err := p.createClonedClusterCertificate(ctx.Child(), namespace, servingCertName, selfSignedIssuerRef, servingCertOptions, opts.Certificates.ServingCert, cluster, cluster.setServingCert)
	if err != nil {
		return errHandler(err, "failed to create cluster serving cert %q", helpers.FullNameStr(namespace, servingCertName))
	}

	// 3. Create the client CA certificate (short lived) and issuer
	clientCACertName := helpers.CleanName(newClusterName + "-client-ca")
	clientCAIssuerName := helpers.CleanName(clientCACertName + "-issuer")
	ctx.Log.Step().Info("Creating PKI for client certificates for the new cluster", "certificateName", clientCACertName, "issuerName", clientCAIssuerName)

	// 3.1 Client CA certificate, minted from the self-signed issuer.
	clientCACNSuffix := " CNPC CA"
	clientCACertOptions := certmanager.CreateCertificateOptions{
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
	}

	readyClientCACert, err := p.createClonedClusterCertificate(ctx.Child(), namespace, clientCACertName, selfSignedIssuerRef, clientCACertOptions, opts.Certificates.ClientCACert, cluster, cluster.setClientCACert)
	if err != nil {
		return errHandler(err, "failed to create client CA cert %q", helpers.FullNameStr(namespace, clientCACertName))
	}

	// 3.2 Client CA issuer
	clientCAIssuer, err := p.cmClient.CreateIssuer(ctx.Child(), namespace, clientCAIssuerName, certmanagerv1.IssuerConfig{CA: &certmanagerv1.CAIssuer{SecretName: readyClientCACert.Name}}, certmanager.CreateIssuerOptions{})
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
	postgresUserCert, err := p.cucp.NewClusterUserCert(ctx.Child(), namespace, "postgres", cmmeta.IssuerReference{Name: clientCAIssuerName}, newClusterName, cucOptions)
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
	replicationUserCert, err := p.cucp.NewClusterUserCert(ctx.Child(), namespace, "streaming_replica", cmmeta.IssuerReference{Name: clientCAIssuerName}, newClusterName, cucOptions)
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

	// Configure where the new cluster sources WAL during recovery from the source cluster's
	// barman-cloud plugin object store.
	if err := p.configureWALRecovery(ctx.Child(), namespace, existingCluster, readyBackup, &clusterOpts); err != nil {
		return errHandler(err, "failed to configure WAL recovery for new cluster %q", helpers.FullNameStr(namespace, newClusterName))
	}

	// configureWALRecovery has already populated VolumeSnapshotRecovery, so this deref is safe.
	clusterOpts.VolumeSnapshotRecovery.RecoveryTargetTime = opts.RecoveryTargetTime

	createClone := func(ctx *contexts.Context) error {
		newCluster, createErr := p.cnpgClient.CreateCluster(ctx.Child(), namespace, newClusterName, clusterVolumeSize, readyServingCert.Name, readyClientCACert.Name, replicationUserCert.GetCertificate().Name, clusterOpts)
		if createErr != nil {
			return trace.Wrap(createErr, "failed to create new cluster %q from backup %q with serving certificate %q and client certificate %q",
				helpers.FullNameStr(namespace, newClusterName), helpers.FullName(readyBackup), helpers.FullName(readyServingCert), helpers.FullName(readyClientCACert))
		}

		cluster.setCluster(newCluster)
		return nil
	}

	if err := createClone(ctx.Child()); err != nil {
		return errHandler(err)
	}

	readyCluster, err := p.waitForCloneRecovery(ctx.Child(), namespace, newClusterName, opts)
	if err != nil {
		if !errors.Is(err, ErrRecoveryTargetNotReached) {
			return errHandler(err, "failed to wait for new PITR cluster %q to become ready", helpers.FullNameStr(namespace, newClusterName))
		}

		// The source had no WAL at or after the target (an idle database), so recovering forward to
		// the wall-clock target is impossible. Recovering to the consistency point is data-identical
		// (nothing changed after it), so tear the clone down and recreate it with targetImmediate.
		ctx.Log.Info("Recovery target unreachable from archived WAL (idle source); falling back to the consistency point", "recoveryTargetTime", opts.RecoveryTargetTime)
		if deleteErr := p.cnpgClient.DeleteCluster(ctx.Child(), namespace, newClusterName); deleteErr != nil {
			return errHandler(deleteErr, "failed to delete cluster %q before recovery fallback", newClusterName)
		}

		if deleteErr := p.cnpgClient.WaitForClusterDeleted(ctx.Child(), namespace, newClusterName, cnpg.WaitForClusterDeletedOpts{MaxWaitTime: opts.WaitForClusterTimeout}); deleteErr != nil {
			return errHandler(deleteErr, "failed waiting for cluster %q deletion before recovery fallback", newClusterName)
		}

		// An empty target time recovers to the consistency point (targetImmediate).
		clusterOpts.VolumeSnapshotRecovery.RecoveryTargetTime = ""
		if createErr := createClone(ctx.Child()); createErr != nil {
			return errHandler(createErr)
		}

		readyCluster, err = p.cnpgClient.WaitForReadyCluster(ctx.Child(), namespace, newClusterName, cnpg.WaitForReadyClusterOpts{MaxWaitTime: opts.WaitForClusterTimeout})
		if err != nil {
			return errHandler(err, "failed to wait for new cluster %q to become ready", helpers.FullNameStr(namespace, newClusterName))
		}
	}
	cluster.setCluster(readyCluster)

	return cluster, nil
}

// createClonedClusterCertificate creates one of the clone's certificates and, when approver-policy is
// enforcing approval on the cluster, a tight policy derived from the cert (via createcrpforcertificate)
// so approver-policy approves it. This mirrors the clusterusercert flow: because the policy is derived
// from the cert it must be created after the cert, so the first issuance is denied and the cert is
// reissued once the policy exists. The cert and any policy are recorded on the cluster (via setCert and
// addCertificateRequestPolicy) as they are created, so a failure mid-way is cleaned up by the caller's
// cluster.Delete. Returns the ready certificate.
func (p *Provider) createClonedClusterCertificate(ctx *contexts.Context, namespace, certName string, issuerRef cmmeta.IssuerReference, certOpts certmanager.CreateCertificateOptions, clusterCertOpts CloneClusterOptionsCertificate, cluster ClonedClusterInterface, setCert func(*certmanagerv1.Certificate)) (*certmanagerv1.Certificate, error) {
	cert, err := p.cmClient.CreateCertificate(ctx.Child(), namespace, certName, issuerRef, certOpts)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create certificate %q", helpers.FullNameStr(namespace, certName))
	}
	setCert(cert)

	// Only deploy a CertificateRequestPolicy when approver-policy is enforcing approval (see IsAvailable).
	shouldDeployCRP, err := p.apClient.IsAvailable(ctx.Child())
	if err != nil {
		return nil, trace.Wrap(err, "failed to determine whether approver-policy is enforcing CertificateRequest approval")
	}

	if shouldDeployCRP {
		crp, err := p.ccfp.CreateCRPForCertificate(ctx.Child(), cert, createcrpforcertificate.CreateCRPForCertificateOpts{MaxWaitTime: clusterCertOpts.CRPOpts.WaitForCRPTimeout})
		if err != nil {
			return nil, trace.Wrap(err, "failed to create certificate request policy for certificate %q", helpers.FullName(cert))
		}
		cluster.addCertificateRequestPolicy(crp)

		// The initial request was likely denied (the policy didn't exist yet), so reissue now that it does.
		reissuedCert, err := p.cmClient.ReissueCertificate(ctx.Child(), cert.Namespace, cert.Name)
		if err != nil {
			return nil, trace.Wrap(err, "failed to re-issue certificate %q", helpers.FullName(cert))
		}
		cert = reissuedCert
		setCert(cert)
	}

	readyCert, err := p.cmClient.WaitForReadyCertificate(ctx.Child(), cert.Namespace, cert.Name, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: clusterCertOpts.WaitForReadyTimeout})
	if err != nil {
		return nil, trace.Wrap(err, "failed to wait for certificate %q to be ready", helpers.FullName(cert))
	}
	setCert(readyCert)

	return readyCert, nil
}

// waitForCloneRecovery waits for the cloned cluster to become ready, watching the API server rather
// than polling. When recovering to a wall-clock target, recovery may not reach it (an idle source),
// so it watches the recovery Job to its terminal outcome:
//   - The Job retries (its backoffLimit window) long enough to wait out the source's archive_timeout,
//     so it completes once an attempt reaches the target; the instance is then promoted and the
//     cluster becomes ready.
//   - It fails once it has exhausted its retries without reaching the target, meaning the target is
//     unreachable from archived WAL (an idle source), so this returns ErrRecoveryTargetNotReached and
//     the caller falls back to the consistency point.
//
// Watching the Job's terminal condition is unambiguous — a completed Job promoted, a failed Job gave
// up — unlike the cluster status (which doesn't surface the reason) or a one-shot pod check (which
// can't tell an idle retry from a slow replay). The terminal state is observed even if CNPG garbage
// collects the Job afterwards, since the delete watch event still carries the Job's final state.
func (p *Provider) waitForCloneRecovery(ctx *contexts.Context, namespace, newClusterName string, opts CloneClusterOptions) (*apiv1.Cluster, error) {
	if opts.RecoveryTargetTime != "" {
		// The recovery Job's name isn't known ahead of time, so select it by the cluster + jobRole labels.
		jobSelector := fmt.Sprintf("%s=%s,%s", utils.ClusterLabelName, newClusterName, utils.JobRoleLabelName)
		_, err := p.coreClient.WaitForJobCompletion(ctx.Child(), namespace, "", core.WaitForJobCompletionOpts{MaxWaitTime: opts.WaitForClusterTimeout, LabelSelector: jobSelector})
		if err != nil {
			// A failed recovery Job means the target wasn't reachable from archived WAL.
			if errors.Is(err, core.ErrJobFailed) {
				return nil, ErrRecoveryTargetNotReached
			}

			return nil, trace.Wrap(err, "failed waiting for the recovery job of cluster %q", helpers.FullNameStr(namespace, newClusterName))
		}
	}

	return p.cnpgClient.WaitForReadyCluster(ctx.Child(), namespace, newClusterName, cnpg.WaitForReadyClusterOpts{MaxWaitTime: opts.WaitForClusterTimeout})
}

// configureWALRecovery sets the recovery source on clusterOpts for a source cluster that archives
// write-ahead logs with the barman-cloud plugin: the new cluster recovers from the backup's volume
// snapshots, fetching WAL via an external cluster that references the same object store. This honors a
// wall-clock recoveryTarget.targetTime. The recovery target itself is chosen by the caller; this only
// configures the source. It is an error for the source cluster not to use the barman-cloud plugin.
func (p *Provider) configureWALRecovery(ctx *contexts.Context, namespace string, sourceCluster *apiv1.Cluster, backup *apiv1.Backup, clusterOpts *cnpg.CreateClusterOptions) (err error) {
	walArchiverPlugin := findBarmanCloudWALArchiver(sourceCluster)
	if walArchiverPlugin == nil {
		return trace.Errorf("source cluster %q does not archive WAL via the barman-cloud plugin", helpers.FullName(sourceCluster))
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
// configuration, or nil if the cluster does not use the plugin (i.e. it does no WAL archiving at
// all). It prefers the plugin explicitly marked as the WAL archiver.
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

func (cc *ClonedCluster) setSelfSignedIssuer(issuer *certmanagerv1.Issuer) {
	cc.selfSignedIssuer = issuer
}

func (cc *ClonedCluster) GetSelfSignedIssuer() *certmanagerv1.Issuer {
	return cc.selfSignedIssuer
}

func (cc *ClonedCluster) addCertificateRequestPolicy(crp *policyv1alpha1.CertificateRequestPolicy) {
	cc.certificateRequestPolicies = append(cc.certificateRequestPolicies, crp)
}

func (cc *ClonedCluster) GetCertificateRequestPolicies() []*policyv1alpha1.CertificateRequestPolicy {
	return cc.certificateRequestPolicies
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

	cleanupErrs := make([]error, 0, 7+len(cc.certificateRequestPolicies))

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

	// Delete the CertificateRequestPolicies created for the serving and client-CA certs (the user certs
	// own their own CRPs, deleted above via their Delete).
	for _, crp := range cc.certificateRequestPolicies {
		if err := cc.p.ap().DeleteCertificateRequestPolicy(ctx.Child(), crp.Name); err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster certificate request policy %q", helpers.FullName(crp)))
		}
	}

	// Delete the self-signed issuer last: it backs the serving and client CA certs, so it outlives them.
	if cc.selfSignedIssuer != nil {
		if err := cc.p.cm().DeleteIssuer(ctx.Child(), cc.selfSignedIssuer.Namespace, cc.selfSignedIssuer.Name); err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster self-signed issuer %q", helpers.FullName(cc.selfSignedIssuer)))
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed while cleaning up cloned cluster")
}
