package clonepvc

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockProvider struct {
	*Provider
	coreClient *core.MockClientInterface
	esClient   *externalsnapshotter.MockClientInterface
}

func newMockProvider(t *testing.T) *mockProvider {
	coreClient := core.NewMockClientInterface(t)
	esClient := externalsnapshotter.NewMockClientInterface(t)

	provider := NewProvider(coreClient, esClient)

	return &mockProvider{
		Provider:   provider,
		coreClient: coreClient,
		esClient:   esClient,
	}
}

// expectForceBind sets up the core-client expectations for the internal forceBindVolumes helper: a
// pod is created, awaited (erroring when shouldErr), and always torn down. The detailed pod
// construction is asserted in forcebind_test.go; here it's just the call graph ClonePVC/ClonePVCGroup
// drive. The error case models a force-bind failure via the readiness wait.
func (p *mockProvider) expectForceBind(t *testing.T, ctx *contexts.Context, namespace string, shouldErr bool) {
	createdPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "force-bind-pod", Namespace: namespace}}

	p.coreClient.EXPECT().CreatePod(mock.Anything, namespace, mock.Anything).
		RunAndReturn(func(calledCtx *contexts.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
			assert.True(t, calledCtx.IsChildOf(ctx))
			return createdPod, nil
		})

	p.coreClient.EXPECT().DeletePod(mock.Anything, namespace, createdPod.Name).
		RunAndReturn(func(cleanupCtx *contexts.Context, namespace, name string) error {
			assert.NotEqual(t, ctx, cleanupCtx)
			return nil
		})

	p.coreClient.EXPECT().WaitForReadyPod(mock.Anything, namespace, createdPod.Name, mock.Anything).
		RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, opts core.WaitForReadyPodOpts) (*corev1.Pod, error) {
			assert.True(t, calledCtx.IsChildOf(ctx))
			return th.ErrOr1Val(createdPod, shouldErr)
		})
}

func TestNewProvider(t *testing.T) {
	coreClient := core.NewMockClientInterface(t)
	esClient := externalsnapshotter.NewMockClientInterface(t)

	provider := NewProvider(coreClient, esClient)
	require.NotNil(t, provider)

	assert.Equal(t, coreClient, provider.coreClient)
	assert.Equal(t, esClient, provider.esClient)
}
