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

// ScheduledBackupsGetter has a method to return a ScheduledBackupInterface.
// A group's client should implement this interface.
type ScheduledBackupsGetter interface {
	ScheduledBackups(namespace string) ScheduledBackupInterface
}

// ScheduledBackupInterface has methods to work with ScheduledBackup resources.
type ScheduledBackupInterface interface {
	Create(ctx context.Context, scheduledBackup *apiv1.ScheduledBackup, opts metav1.CreateOptions) (*apiv1.ScheduledBackup, error)
	Update(ctx context.Context, scheduledBackup *apiv1.ScheduledBackup, opts metav1.UpdateOptions) (*apiv1.ScheduledBackup, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, scheduledBackup *apiv1.ScheduledBackup, opts metav1.UpdateOptions) (*apiv1.ScheduledBackup, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*apiv1.ScheduledBackup, error)
	List(ctx context.Context, opts metav1.ListOptions) (*apiv1.ScheduledBackupList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *apiv1.ScheduledBackup, err error)
	ScheduledBackupExpansion
}

// scheduledBackups implements ScheduledBackupInterface
type scheduledBackups struct {
	*gentype.ClientWithList[*apiv1.ScheduledBackup, *apiv1.ScheduledBackupList]
}

// newScheduledBackups returns a ScheduledBackups
func newScheduledBackups(c *PostgresqlV1Client, namespace string) *scheduledBackups {
	return &scheduledBackups{
		gentype.NewClientWithList[*apiv1.ScheduledBackup, *apiv1.ScheduledBackupList](
			"scheduledbackups",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *apiv1.ScheduledBackup { return &apiv1.ScheduledBackup{} },
			func() *apiv1.ScheduledBackupList { return &apiv1.ScheduledBackupList{} },
		),
	}
}
