package common

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	"k8s.io/client-go/kubernetes"
)

type KubeClusterCommand struct {
	KubernetesCommand
}

func (kc *KubernetesCommand) NewKubeClusterClient() (kubecluster.ClientInterface, error) {
	config, err := kc.GetClusterConfig()
	if err != nil {
		return nil, trace.Wrap(err, "failed to get kubernetes config")
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create new kubernetes client")
	}
	restClient := k8sClient.RESTClient()

	return kubecluster.NewClient(
		certmanager.NewClient(restClient),
		cnpg.NewClient(restClient),
		externalsnapshotter.NewClient(restClient),
		core.NewClient(restClient),
	), nil
}
