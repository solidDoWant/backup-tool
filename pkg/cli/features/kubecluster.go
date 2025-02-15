package features

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
)

type KubeClusterCommandInterface interface {
	KubernetesCommandInterface
	NewKubeClusterClient() (kubecluster.ClientInterface, error)
}

// Gives a command the ability to interact with Kubernetes clusters (all supported APIs).
type KubeClusterCommand struct {
	KubernetesCommand
}

func NewKubeClusterCommand() *KubeClusterCommand {
	return &KubeClusterCommand{}
}

func (kcc *KubernetesCommand) NewKubeClusterClient() (kubecluster.ClientInterface, error) {
	config, err := kcc.GetClusterConfig()
	if err != nil {
		return nil, trace.Wrap(err, "failed to get kubernetes config")
	}

	cmClient, err := certmanager.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create cert-manager client")
	}

	cnpgClient, err := cnpg.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create cloudnative-pg client")
	}

	esClient, err := externalsnapshotter.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create external-snapshotter client")
	}

	coreClient, err := core.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create core client")
	}

	apClient, err := approverpolicy.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create approver-policy client")
	}

	return kubecluster.NewClient(
		cmClient,
		cnpgClient,
		esClient,
		coreClient,
		apClient,
	), nil
}
