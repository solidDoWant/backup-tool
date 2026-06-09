package externalsnapshotter

import (
	"github.com/gravitational/trace"
	volumegroupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// ClientInterface wraps the external-snapshotter clientset, which serves both the
// snapshot.storage.k8s.io group (VolumeSnapshot...) and the groupsnapshot.storage.k8s.io group
// (VolumeGroupSnapshot...). Both are covered here because they are served by the one clientset, the
// same way the core primitive covers everything served by the standard kube clientset.
type ClientInterface interface {
	// VolumeSnapshot (snapshot.storage.k8s.io)
	SnapshotVolume(*contexts.Context, string, string, SnapshotVolumeOptions) (*volumesnapshotv1.VolumeSnapshot, error)
	WaitForReadySnapshot(ctx *contexts.Context, namespace, name string, opts WaitForReadySnapshotOpts) (*volumesnapshotv1.VolumeSnapshot, error)
	DeleteSnapshot(*contexts.Context, string, string) error
	// VolumeGroupSnapshot (groupsnapshot.storage.k8s.io)
	GroupSnapshotVolumes(ctx *contexts.Context, namespace string, selector metav1.LabelSelector, opts GroupSnapshotOptions) (*volumegroupsnapshotv1.VolumeGroupSnapshot, error)
	WaitForReadyGroupSnapshot(ctx *contexts.Context, namespace, name string, opts WaitForReadyGroupSnapshotOpts) (*volumegroupsnapshotv1.VolumeGroupSnapshot, error)
	// WaitForReadyGroupSnapshotMembers waits until expectedCount member VolumeSnapshots of the named
	// VolumeGroupSnapshot are present and individually ready, then returns them. Each member's source
	// PVC is available on its Spec.Source.PersistentVolumeClaimName. The members' status is populated
	// asynchronously by the snapshot controller after the group snapshot reports ready, so this waits
	// rather than listing once. expectedCount is the number of PVCs the group's selector matched.
	WaitForReadyGroupSnapshotMembers(ctx *contexts.Context, namespace, name string, expectedCount int, opts WaitForReadyGroupSnapshotMembersOpts) ([]*volumesnapshotv1.VolumeSnapshot, error)
	DeleteGroupSnapshot(ctx *contexts.Context, namespace, name string) error
}

type Client struct {
	client versioned.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingExternalSnapshotterClient, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create external-snapshotter client")
	}

	return &Client{
		client: underlyingExternalSnapshotterClient,
	}, nil
}
