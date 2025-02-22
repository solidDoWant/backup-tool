// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	scheme "github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/cnpg/gen/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// ClustersGetter has a method to return a ClusterInterface.
// A group's client should implement this interface.
type ClustersGetter interface {
	Clusters(namespace string) ClusterInterface
}

// ClusterInterface has methods to work with Cluster resources.
type ClusterInterface interface {
	Create(ctx context.Context, cluster *apiv1.Cluster, opts metav1.CreateOptions) (*apiv1.Cluster, error)
	Update(ctx context.Context, cluster *apiv1.Cluster, opts metav1.UpdateOptions) (*apiv1.Cluster, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, cluster *apiv1.Cluster, opts metav1.UpdateOptions) (*apiv1.Cluster, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*apiv1.Cluster, error)
	List(ctx context.Context, opts metav1.ListOptions) (*apiv1.ClusterList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *apiv1.Cluster, err error)
	ClusterExpansion
}

// clusters implements ClusterInterface
type clusters struct {
	*gentype.ClientWithList[*apiv1.Cluster, *apiv1.ClusterList]
}

// newClusters returns a Clusters
func newClusters(c *PostgresqlV1Client, namespace string) *clusters {
	return &clusters{
		gentype.NewClientWithList[*apiv1.Cluster, *apiv1.ClusterList](
			"clusters",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *apiv1.Cluster { return &apiv1.Cluster{} },
			func() *apiv1.ClusterList { return &apiv1.ClusterList{} },
		),
	}
}
