package clonepvc

import (
	"path/filepath"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type forceBindVolumesOptions struct {
	WaitForReadyTimeout helpers.MaxWaitTime
	CleanupTimeout      helpers.MaxWaitTime
}

// forceBindVolumes forces the named PVCs to bind immediately by mounting them all into a single
// short-lived pod and waiting for it to become ready. This is needed for storage classes with
// volumeBindingMode WaitForFirstConsumer when the volume backing the PVC (such as a snapshot) will be
// deleted before the PVC is otherwise consumed. Once a PVC is bound it stays bound, so the pod is
// deleted as soon as it is ready. All PVCs are mounted into one pod, so they must be co-schedulable
// onto a single node.
func (p *Provider) forceBindVolumes(ctx *contexts.Context, namespace string, pvcNames []string, opts forceBindVolumesOptions) (err error) {
	ctx.Log.With("pvcs", pvcNames).Info("Forcing immediate bind of volumes")
	defer ctx.Log.Info("Finished forcing immediate bind of volumes", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	scvs := make([]core.SingleContainerVolume, 0, len(pvcNames))
	for _, pvcName := range pvcNames {
		// Each volume needs a distinct mount path within the single container.
		scvs = append(scvs, core.NewSingleContainerPVC(pvcName, filepath.Join("/mnt", pvcName)))
	}
	volumes, volumeMounts := core.ConvertSingleContainerVolumes(scvs)

	pod, err := p.coreClient.CreatePod(ctx.Child(), namespace, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "force-bind-",
			Labels: map[string]string{
				"app.kubernetes.io/component": "force-bind",
			},
		},
		Spec: corev1.PodSpec{
			Volumes: volumes,
			Containers: []corev1.Container{
				{
					Name:            "force-bind",
					Image:           "registry.k8s.io/pause", // TODO pin
					VolumeMounts:    volumeMounts,
					SecurityContext: core.RestrictedContainerSecurityContext(1000, 1000),
				},
			},
			SecurityContext: core.RestrictedPodSecurityContext(1000, 1000),
			RestartPolicy:   corev1.RestartPolicyNever,
		},
	})
	if err != nil {
		return trace.Wrap(err, "failed to create 'force bind' pod in namespace %q", namespace)
	}
	defer cleanup.To(func(ctx *contexts.Context) error {
		return p.coreClient.DeletePod(ctx, namespace, pod.Name)
	}).WithErrMessage("failed to delete 'force bind' pod %q", helpers.FullName(pod)).WithOriginalErr(&err).
		WithParentCtx(ctx).WithTimeout(opts.CleanupTimeout.MaxWait(time.Minute)).Run()

	_, err = p.coreClient.WaitForReadyPod(ctx.Child(), namespace, pod.Name, core.WaitForReadyPodOpts{MaxWaitTime: opts.WaitForReadyTimeout})
	if err != nil {
		return trace.Wrap(err, "failed to wait for 'force bind' pod %q to become ready", helpers.FullName(pod))
	}

	return nil
}
