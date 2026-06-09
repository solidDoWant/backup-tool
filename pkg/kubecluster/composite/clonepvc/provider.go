package clonepvc

import (
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderInterface clones PVCs from CSI snapshots. ClonePVC clones a single volume (via an
// individual VolumeSnapshot); ClonePVCGroup clones a label-selected set atomically (via a
// VolumeGroupSnapshot). Both share the same dependencies and the same per-snapshot
// create-and-force-bind machinery, so they live on one provider.
type ProviderInterface interface {
	ClonePVC(ctx *contexts.Context, namespace, pvcName string, opts ClonePVCOptions) (clonedPvc *corev1.PersistentVolumeClaim, err error)
	ClonePVCGroup(ctx *contexts.Context, namespace string, selector metav1.LabelSelector, opts ClonePVCGroupOptions) (result *ClonePVCGroupResult, err error)
}

type Provider struct {
	coreClient core.ClientInterface
	esClient   externalsnapshotter.ClientInterface
}

func NewProvider(coreClient core.ClientInterface, esClient externalsnapshotter.ClientInterface) *Provider {
	return &Provider{
		coreClient: coreClient,
		esClient:   esClient,
	}
}
