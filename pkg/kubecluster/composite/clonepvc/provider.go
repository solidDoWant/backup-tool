package clonepvc

import (
	context "context"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
)

type ProviderInterface interface {
	ClonePVC(ctx context.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error)
}

type Provider struct {
	esClient   externalsnapshotter.ClientInterface
	coreClient core.ClientInterface
}

func NewProvider(coreClient core.ClientInterface, esClient externalsnapshotter.ClientInterface) *Provider {
	return &Provider{
		coreClient: coreClient,
		esClient:   esClient,
	}
}
