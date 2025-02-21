package disasterrecovery

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clusterusercert"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
)

type OptionsBackupSnapshot struct {
	ReadyTimeout  helpers.MaxWaitTime `yaml:"snapshotReadyTimeout,omitempty"`
	SnapshotClass string              `yaml:"snapshotClass,omitempty"`
}

type OptionsClusterUserCert struct {
	Subject             *certmanagerv1.X509Subject                `yaml:"subject,omitempty"`
	WaitForReadyTimeout helpers.MaxWaitTime                       `yaml:"waitForReadyTimeout,omitempty"`
	CRPOpts             clusterusercert.NewClusterUserCertOptsCRP `yaml:"certificateRequestPolicy,omitempty"`
}
