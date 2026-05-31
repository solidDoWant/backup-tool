package barmancloud

import (
	barmancloudv1 "github.com/cloudnative-pg/plugin-barman-cloud/api/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (bcc *Client) GetObjectStore(ctx *contexts.Context, namespace, name string) (*barmancloudv1.ObjectStore, error) {
	ctx.Log.With("name", name).Info("Getting object store")

	objectStore, err := bcc.barmanCloudClient.BarmancloudV1().ObjectStores(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to get barman-cloud object store %q", helpers.FullNameStr(namespace, name))
	}

	ctx.Log.Debug("Retrieved object store", "objectStore", objectStore)
	return objectStore, nil
}
