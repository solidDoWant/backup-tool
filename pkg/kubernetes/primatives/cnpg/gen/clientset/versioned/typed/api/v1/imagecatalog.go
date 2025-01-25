// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	scheme "github.com/solidDoWant/backup-tool/pkg/kubernetes/primatives/cnpg/gen/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// ImageCatalogsGetter has a method to return a ImageCatalogInterface.
// A group's client should implement this interface.
type ImageCatalogsGetter interface {
	ImageCatalogs(namespace string) ImageCatalogInterface
}

// ImageCatalogInterface has methods to work with ImageCatalog resources.
type ImageCatalogInterface interface {
	Create(ctx context.Context, imageCatalog *apiv1.ImageCatalog, opts metav1.CreateOptions) (*apiv1.ImageCatalog, error)
	Update(ctx context.Context, imageCatalog *apiv1.ImageCatalog, opts metav1.UpdateOptions) (*apiv1.ImageCatalog, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*apiv1.ImageCatalog, error)
	List(ctx context.Context, opts metav1.ListOptions) (*apiv1.ImageCatalogList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *apiv1.ImageCatalog, err error)
	ImageCatalogExpansion
}

// imageCatalogs implements ImageCatalogInterface
type imageCatalogs struct {
	*gentype.ClientWithList[*apiv1.ImageCatalog, *apiv1.ImageCatalogList]
}

// newImageCatalogs returns a ImageCatalogs
func newImageCatalogs(c *PostgresqlV1Client, namespace string) *imageCatalogs {
	return &imageCatalogs{
		gentype.NewClientWithList[*apiv1.ImageCatalog, *apiv1.ImageCatalogList](
			"imagecatalogs",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *apiv1.ImageCatalog { return &apiv1.ImageCatalog{} },
			func() *apiv1.ImageCatalogList { return &apiv1.ImageCatalogList{} },
		),
	}
}
