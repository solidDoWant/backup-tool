package clonepvc

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestForceBindVolumes(t *testing.T) {
	namespace := "test-ns"
	pvcNames := []string{"pvc-a", "pvc-b"}

	createdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "force-bind-generated", Namespace: namespace},
	}

	tests := []struct {
		desc                 string
		simulateCreateErr    bool
		simulateWaitErr      bool
		simulatePodDeleteErr bool
		shouldError          bool
	}{
		{
			desc: "successful force bind",
		},
		{
			desc:              "error creating pod",
			simulateCreateErr: true,
			shouldError:       true,
		},
		{
			desc:            "error waiting for pod",
			simulateWaitErr: true,
			shouldError:     true,
		},
		{
			desc:                 "error deleting pod",
			simulatePodDeleteErr: true,
			shouldError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := newMockProvider(t)
			ctx := th.NewTestContext()

			func() {
				p.coreClient.EXPECT().CreatePod(mock.Anything, namespace, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						// One volume + mount per PVC, in a single locked-down container.
						assert.Contains(t, pod.GenerateName, "force-bind")
						assert.Len(t, pod.Spec.Volumes, len(pvcNames))
						require.Len(t, pod.Spec.Containers, 1)
						assert.Len(t, pod.Spec.Containers[0].VolumeMounts, len(pvcNames))
						assert.Contains(t, pod.Spec.Containers[0].Image, "pause")
						assert.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)

						return th.ErrOr1Val(createdPod, tt.simulateCreateErr)
					})
				if tt.simulateCreateErr {
					return
				}

				// The pod is always torn down (deferred), whether or not it became ready.
				p.coreClient.EXPECT().DeletePod(mock.Anything, namespace, createdPod.Name).
					RunAndReturn(func(cleanupCtx *contexts.Context, namespace, name string) error {
						assert.NotEqual(t, ctx, cleanupCtx)
						return th.ErrIfTrue(tt.simulatePodDeleteErr)
					})

				p.coreClient.EXPECT().WaitForReadyPod(mock.Anything, namespace, createdPod.Name, core.WaitForReadyPodOpts{MaxWaitTime: 0}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, opts core.WaitForReadyPodOpts) (*corev1.Pod, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(createdPod, tt.simulateWaitErr)
					})
			}()

			err := p.forceBindVolumes(ctx, namespace, pvcNames, forceBindVolumesOptions{})
			if tt.shouldError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
