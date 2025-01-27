package common

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
)

type KubeClusterCommand struct {
	KubernetesCommand
}

func (kc *KubernetesCommand) NewKubeClusterClient() (kubecluster.ClientInterface, error) {
	config, err := kc.GetClusterConfig()
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

	externalsnapshotterClient, err := externalsnapshotter.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create external-snapshotter client")
	}

	coreClient, err := core.NewClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create core client")
	}

	return kubecluster.NewClient(
		cmClient,
		cnpgClient,
		externalsnapshotterClient,
		coreClient,
	), nil
}
