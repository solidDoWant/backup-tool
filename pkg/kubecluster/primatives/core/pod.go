package core

import (
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func (c *Client) CreatePod(ctx *contexts.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	pod, err := c.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create pod %q", helpers.FullNameStr(namespace, pod.Name))
	}

	return pod, nil
}

type WaitForReadyPodOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadyPod(ctx *contexts.Context, namespace, name string, opts WaitForReadyPodOpts) (*corev1.Pod, error) {
	processEvent := func(_ *contexts.Context, pod *corev1.Pod) (*corev1.Pod, bool, error) {
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
	pod, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(time.Minute), c.client.CoreV1().Pods(namespace), name, processEvent)

	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for pod to become ready")
	}

	return pod, nil
}

func (c *Client) DeletePod(ctx *contexts.Context, namespace, name string) error {
	err := c.client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete pod %q", helpers.FullNameStr(namespace, name))
}

// Helpers
// Represents a volume that is mounted in a single container.
type SingleContainerVolume struct {
	Name         string              `yaml:"name" jsonschema:"required"`
	MountPath    string              `yaml:"mountPath" jsonschema:"required"`
	VolumeSource corev1.VolumeSource `yaml:"volumeSource" jsonschema:"required"`
}

func NewSingleContainerPVC(pvcName, mountPath string) SingleContainerVolume {
	return SingleContainerVolume{
		Name:      pvcName,
		MountPath: mountPath,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	}
}

func NewSingleContainerSecret(secretName, mountPath string, items ...corev1.KeyToPath) SingleContainerVolume {
	return SingleContainerVolume{
		Name:      secretName,
		MountPath: mountPath,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To(int32(0400)), // Read only by owner
				Items:       items,
			},
		},
	}
}

func (scv *SingleContainerVolume) ToVolume() corev1.Volume {
	return corev1.Volume{
		Name:         scv.Name,
		VolumeSource: scv.VolumeSource,
	}
}

func (svc *SingleContainerVolume) ToVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      svc.Name,
		MountPath: svc.MountPath,
	}
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
