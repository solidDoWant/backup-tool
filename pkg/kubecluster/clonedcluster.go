package kubecluster

import (
	context "context"
	"fmt"
	"path/filepath"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	postgres "github.com/solidDoWant/backup-tool/pkg/postgres"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ClonedClusterInterface interface {
	GetCredentials(servingCertMountDirectory, clientCertMountDirectory string) postgres.Credentials
	Delete(ctx context.Context) error
	setServingCert(cert *certmanagerv1.Certificate)
	GetServingCert() *certmanagerv1.Certificate
	setClientCert(cert *certmanagerv1.Certificate)
	GetClientCert() *certmanagerv1.Certificate
	setCluster(cluster *apiv1.Cluster)
	GetCluster() *apiv1.Cluster
}

type ClonedCluster struct {
	c                  ClientInterface
	cluster            *apiv1.Cluster
	servingCertificate *certmanagerv1.Certificate
	clientCertificate  *certmanagerv1.Certificate
}

type CloneClusterOptions struct {
	WaitForBackupTimeout      helpers.MaxWaitTime        `yaml:"waitForBackupTimeout,omitempty"`
	ServingCertSubject        *certmanagerv1.X509Subject `yaml:"servingCertSubject,omitempty"`
	ServingCertIssuerKind     string                     `yaml:"servingCertIssuerKind,omitempty"`
	WaitForServingCertTimeout helpers.MaxWaitTime        `yaml:"waitForServingCertTimeout,omitempty"`
	ClientCertSubject         *certmanagerv1.X509Subject `yaml:"clientCertSubject,omitempty"`
	ClientCertIssuerKind      string                     `yaml:"clientCertIssuerKind,omitempty"`
	WaitForClientCertTimeout  helpers.MaxWaitTime        `yaml:"waitForClientCertTimeout,omitempty"`
	RecoveryTargetTime        string                     `yaml:"recoveryTargetTime,omitempty"`
	WaitForClusterTimeout     helpers.MaxWaitTime        `yaml:"waitForClusterTimeout,omitempty"`
	CleanupTimeout            helpers.MaxWaitTime        `yaml:"cleanupTimeout,omitempty"`
	// TODO maybe provide an option for additional client auth CAs?
	// TODO maybe provide an option for CRPs?
}

func newClonedCluster(c ClientInterface) ClonedClusterInterface {
	return &ClonedCluster{c: c}
}

// Clone an existing CNPG cluster, with separate certificates for authentication.
// It is assumed that all required resources for approving certificats (such as Certificate Request Policies) are already in place.
func (c *Client) CloneCluster(ctx context.Context, namespace, existingClusterName, newClusterName, servingCertIssuerName, clientCertIssuerName string, opts CloneClusterOptions) (cluster ClonedClusterInterface, err error) {
	cluster = c.newClonedCluster()

	// Prepare to handle resource cleanup in the event of an error
	errHandler := func(originalErr error, args ...interface{}) (*ClonedCluster, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(10*time.Minute), cluster.Delete).
			WithErrMessage("failed to cleanup cloned cluster %q in namespace %q", newClusterName, namespace).
			WithOriginalErr(&originalErr).
			Run()
	}

	// Perform as many read-only operations as possible now to reduce the number of changes that need to be reverted in case of a failure
	existingCluster, err := c.CNPG().GetCluster(ctx, namespace, existingClusterName)
	if err != nil {
		return errHandler(err, "failed to get existing cluster %q", helpers.FullNameStr(namespace, existingClusterName))
	}

	clusterVolumeSize, err := resource.ParseQuantity(existingCluster.Spec.StorageConfiguration.Size)
	if err != nil {
		return errHandler(err, "failed to parse the existing cluster %q storage volume size %q", helpers.FullName(existingCluster), existingCluster.Spec.StorageConfiguration.Size)
	}

	// 1. Create a backup of the current cluster
	backupNamePrefix := existingClusterName + "-cloned"
	backup, err := c.CNPG().CreateBackup(ctx, namespace, backupNamePrefix, existingClusterName, cnpg.CreateBackupOptions{GenerateName: true})
	if err != nil {
		return errHandler(err, "failed to create backup of existing cluster %q", helpers.FullNameStr(namespace, existingClusterName))
	}
	defer cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(time.Minute), func(ctx context.Context) error {
		cleanupErr := c.CNPG().DeleteBackup(ctx, namespace, backup.Name)
		if cleanupErr == nil {
			return nil
		}

		// If backup deletion failed, treat the entire operation as a failure. This includes deleting the cluster, and setting the return value to nil.
		cluster, cleanupErr = errHandler(cleanupErr, "failed to delete backup %q", helpers.FullName(backup))
		return cleanupErr
	}).WithErrMessage("cleanup failed").WithOriginalErr(&err).Run()

	err = c.CNPG().WaitForReadyBackup(ctx, namespace, backup.Name, cnpg.WaitForReadyBackupOpts{MaxWaitTime: opts.WaitForBackupTimeout})
	if err != nil {
		return errHandler(err, "failed to wait forbackup %q to be ready", helpers.FullName(backup))
	}

	// 2. Create the serving certificate (short lived)
	servingCertName := newClusterName + "-serving-cert"
	certOptions := certmanager.CreateCertificateOptions{
		CommonName: servingCertName,
		Subject:    opts.ServingCertSubject,
		Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageServerAuth},
		SecretLabels: map[string]string{
			utils.WatchedLabelName: "true",
		},
		DNSNames: getClusterDomainNames(newClusterName, namespace),
	}

	if opts.ServingCertIssuerKind != "" {
		certOptions.IssuerKind = opts.ServingCertIssuerKind
	}

	servingCert, err := c.CM().CreateCertificate(ctx, servingCertName, namespace, servingCertIssuerName, certOptions)
	if err != nil {
		return errHandler(err, "failed to create cluster serving cert %q", helpers.FullNameStr(namespace, servingCertName))
	}
	cluster.setServingCert(servingCert)

	err = c.CM().WaitForReadyCertificate(ctx, namespace, servingCertName, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: opts.WaitForServingCertTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for serving certificate %q to be ready", helpers.FullName(servingCert))
	}

	// 3. Create the client certificate (short lived). This is the only certificate that the cluster will trust for client auth.
	clientUserName := "postgres" // Postgres superuser, which has access to all databases
	clientCertName := fmt.Sprintf("%s-%s-user", newClusterName, clientUserName)
	certOptions = certmanager.CreateCertificateOptions{
		CommonName: clientUserName,
		Subject:    opts.ClientCertSubject,
		Usages:     []certmanagerv1.KeyUsage{certmanagerv1.UsageClientAuth},
		SecretLabels: map[string]string{
			utils.WatchedLabelName: "true",
		},
	}

	if opts.ClientCertIssuerKind != "" {
		certOptions.IssuerKind = opts.ClientCertIssuerKind
	}

	clientCert, err := c.CM().CreateCertificate(ctx, clientCertName, namespace, clientCertIssuerName, certOptions)
	if err != nil {
		return errHandler(err, "failed to create %q user cert %q", clientUserName, helpers.FullNameStr(namespace, servingCertName))
	}
	cluster.setClientCert(clientCert)

	err = c.CM().WaitForReadyCertificate(ctx, namespace, clientCertName, certmanager.WaitForReadyCertificateOpts{MaxWaitTime: opts.WaitForClientCertTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for %q user certificate %q to be ready", clientUserName, helpers.FullName(servingCert))
	}

	// 4. Create a new cluster from the backup
	clusterOpts := cnpg.CreateClusterOptions{
		BackupName: backup.Name,
	}

	if opts.RecoveryTargetTime != "" {
		clusterOpts.RecoveryTarget = &apiv1.RecoveryTarget{
			TargetTime: opts.RecoveryTargetTime,
		}
	}

	newCluster, err := c.CNPG().CreateCluster(ctx, namespace, newClusterName, clusterVolumeSize, servingCertName, clientCertName, clusterOpts)
	if err != nil {
		return errHandler(err, "failed to create new cluster %q from backup %q with serving certificate %q and client certificate %q",
			helpers.FullNameStr(namespace, newClusterName), helpers.FullName(backup), helpers.FullName(servingCert), helpers.FullName(clientCert))
	}
	cluster.setCluster(newCluster)

	err = c.CNPG().WaitForReadyCluster(ctx, namespace, newClusterName, cnpg.WaitForReadyClusterOpts{MaxWaitTime: opts.WaitForClusterTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for new cluster %q to become ready", helpers.FullNameStr(namespace, newClusterName))
	}

	return cluster, nil
}

func getClusterDomainNames(clusterName, namespace string) []string {
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

	return domainNames
}

func (cc *ClonedCluster) setServingCert(cert *certmanagerv1.Certificate) {
	cc.servingCertificate = cert
}

func (cc *ClonedCluster) GetServingCert() *certmanagerv1.Certificate {
	return cc.servingCertificate
}

func (cc *ClonedCluster) setClientCert(cert *certmanagerv1.Certificate) {
	cc.clientCertificate = cert
}

func (cc *ClonedCluster) GetClientCert() *certmanagerv1.Certificate {
	return cc.clientCertificate
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
		ServingCertificateCAFilePath: filepath.Join(servingCertMountDirectory, "ca.crt"),
		ClientCertificateFilePath:    filepath.Join(clientCertMountDirectory, "tls.crt"),
		ClientPrivateKeyFilePath:     filepath.Join(clientCertMountDirectory, "tls.key"),
	}
}

func (cc *ClonedCluster) Delete(ctx context.Context) error {
	cleanupErrs := make([]error, 0, 3)

	if cc.cluster != nil {
		err := cc.c.CNPG().DeleteCluster(ctx, cc.cluster.Namespace, cc.cluster.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster CNPG cluster %q", helpers.FullName(cc.cluster)))
		}
	}

	if cc.clientCertificate != nil {
		err := cc.c.CM().DeleteCertificate(ctx, cc.clientCertificate.Name, cc.clientCertificate.Namespace)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster client cert %q", helpers.FullName(cc.clientCertificate)))
		}
	}

	if cc.servingCertificate != nil {
		err := cc.c.CM().DeleteCertificate(ctx, cc.servingCertificate.Name, cc.servingCertificate.Namespace)
		if err != nil {
			cleanupErrs = append(cleanupErrs, trace.Wrap(err, "failed to delete cloned cluster serving cert %q", helpers.FullName(cc.servingCertificate)))
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed while cleaning up cloned cluster")
}
