package core

import (
	"time"

	"github.com/gravitational/trace"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/utils/ptr"
)

func (c *Client) CreatePod(ctx *contexts.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod.Name != "" {
		ctx.Log.With("name", pod.Name)
	} else if pod.GenerateName != "" {
		ctx.Log.With("name", pod.GenerateName)
	} else {
		ctx.Log.With("name", "").Warn("Creating pod without a name")
	}

	ctx.Log.Info("Creating pod")
	ctx.Log.Debug("Call parameters", "pod", pod)

	pod, err := c.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create pod %q", helpers.FullNameStr(namespace, pod.Name))
	}

	return pod, nil
}

type WaitForReadyPodOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadyPod(ctx *contexts.Context, namespace, name string, opts WaitForReadyPodOpts) (pod *corev1.Pod, err error) {
	ctx.Log.With("name", name).Info("Waiting for pod to become ready")
	defer ctx.Log.Info("Finished waiting for pod to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	processEvent := func(ctx *contexts.Context, pod *corev1.Pod) (*corev1.Pod, bool, error) {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				if condition.Status == corev1.ConditionTrue {
					return pod, true, nil
				}
				return nil, false, nil
			}
		}
		return nil, false, nil
	}
	pod, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(time.Minute), c.client.CoreV1().Pods(namespace), name, processEvent)

	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for pod to become ready")
	}

	return pod, nil
}

func (c *Client) DeletePod(ctx *contexts.Context, namespace, name string) error {
	ctx.Log.With("name", name).Info("Deleting pod")

	err := c.client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete pod %q", helpers.FullNameStr(namespace, name))
}

// Helpers
// Represents a volume that is mounted in a single container.
type SingleContainerVolume struct {
	Name         string              `yaml:"name" jsonschema:"required"`
	MountPaths   []string            `yaml:"mountPaths" jsonschema:"required"`
	VolumeSource corev1.VolumeSource `yaml:"volumeSource" jsonschema:"required"`
}

func (scv *SingleContainerVolume) WithMountPath(mountPath string) *SingleContainerVolume {
	scv.MountPaths = append(scv.MountPaths, mountPath)
	return scv
}

func (scv *SingleContainerVolume) ToVolume() corev1.Volume {
	return corev1.Volume{
		Name:         scv.Name,
		VolumeSource: scv.VolumeSource,
	}
}

func (svc *SingleContainerVolume) ToVolumeMounts() []corev1.VolumeMount {
	return lo.Map(svc.MountPaths, func(mountPath string, _ int) corev1.VolumeMount {
		return corev1.VolumeMount{
			Name:      svc.Name,
			MountPath: mountPath,
		}
	})
}

func NewSingleContainerPVC(pvcName, mountPath string) SingleContainerVolume {
	return SingleContainerVolume{
		Name:       pvcName,
		MountPaths: []string{mountPath},
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	}
}

func NewSingleContainerSecret(secretName, mountPath string, items ...corev1.KeyToPath) SingleContainerVolume {
	return SingleContainerVolume{
		Name:       secretName,
		MountPaths: []string{mountPath},
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To(int32(0400)), // Read only by owner
				Items:       items,
			},
		},
	}
}

// TODO replace this with a new, more generic approach
// Converts SCVs into k8s volumes and volume mounts. If multiple SCVs have the same name, they are merged.
// This prevents issues with certain CSIs that don't support the same volume being listed multiple times under
// different names.
func ConvertSingleContainerVolumes(scvs []SingleContainerVolume) ([]corev1.Volume, []corev1.VolumeMount) {
	reducedSCVs := make(map[string]SingleContainerVolume, len(scvs))
	for _, incomingSVC := range scvs {
		if reducedSCV, ok := reducedSCVs[incomingSVC.Name]; ok {
			// case: both are PVCs
			if reducedSCV.VolumeSource.PersistentVolumeClaim != nil && incomingSVC.VolumeSource.PersistentVolumeClaim != nil {
				if reducedSCV.VolumeSource.PersistentVolumeClaim.ClaimName != incomingSVC.VolumeSource.PersistentVolumeClaim.ClaimName {
					// Names for each volume must be unique, so generate a new one for the new svc
					incomingSVC.Name = names.SimpleNameGenerator.GenerateName(incomingSVC.Name + "-")
					reducedSCVs[incomingSVC.Name] = incomingSVC
					continue
				}

				// Merge the mount paths
				reducedSCV.MountPaths = lo.Uniq(append(reducedSCV.MountPaths, incomingSVC.MountPaths...))
				reducedSCVs[reducedSCV.Name] = reducedSCV
				continue
			}

			// case: both are secrets
			if reducedSCV.VolumeSource.Secret != nil && incomingSVC.VolumeSource.Secret != nil {
				if reducedSCV.VolumeSource.Secret.SecretName != incomingSVC.VolumeSource.Secret.SecretName {
					// Names for each volume must be unique, so generate a new one for the new svc
					incomingSVC.Name = names.SimpleNameGenerator.GenerateName(incomingSVC.Name + "-")
					reducedSCVs[incomingSVC.Name] = incomingSVC
					continue
				}

				// Merge the mount paths and the items
				reducedSCV.MountPaths = lo.Uniq(append(reducedSCV.MountPaths, incomingSVC.MountPaths...))
				mergedItems := lo.Uniq(append(reducedSCV.VolumeSource.Secret.Items, incomingSVC.VolumeSource.Secret.Items...))
				if len(mergedItems) > 0 {
					reducedSCV.VolumeSource.Secret.Items = mergedItems
				}
				reducedSCVs[reducedSCV.Name] = reducedSCV
				continue
			}

			// case: unsupported, TODO
			continue
		}
		reducedSCVs[incomingSVC.Name] = incomingSVC
	}

	volumes := lo.MapToSlice(reducedSCVs, func(_ string, svc SingleContainerVolume) corev1.Volume {
		return svc.ToVolume()
	})
	volumeMounts := lo.Flatten(
		lo.MapToSlice(reducedSCVs, func(_ string, svc SingleContainerVolume) []corev1.VolumeMount {
			return svc.ToVolumeMounts()
		}),
	)

	return volumes, volumeMounts
}

func RestrictedPodSecurityContext(uid, gid int64) *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsUser:    ptr.To(uid),
		RunAsGroup:   ptr.To(gid),
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func RestrictedContainerSecurityContext(uid, gid int64) *corev1.SecurityContext {
	return &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		Privileged:               ptr.To(false),
		RunAsUser:                ptr.To(uid),
		RunAsGroup:               ptr.To(gid),
		RunAsNonRoot:             ptr.To(true),
		ReadOnlyRootFilesystem:   ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func PrivilegedPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsUser:    ptr.To(int64(0)),
		RunAsGroup:   ptr.To(int64(0)),
		RunAsNonRoot: ptr.To(false),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func PrivilegedContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		Privileged:               ptr.To(true),
		RunAsUser:                ptr.To(int64(0)),
		RunAsGroup:               ptr.To(int64(0)),
		RunAsNonRoot:             ptr.To(false),
		ReadOnlyRootFilesystem:   ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}
