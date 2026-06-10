package backuptoolinstance

import (
	"fmt"
	"net"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BackupToolInstanceInterface interface {
	setPod(pod *corev1.Pod)
	GetPod() *corev1.Pod
	GetGRPCClient(ctx *contexts.Context) (clients.ClientInterface, error)
	Delete(ctx *contexts.Context) error
}

type BackupToolInstance struct {
	p   providerInterfaceInternal
	pod *corev1.Pod
}

func newBackupToolInstance(p providerInterfaceInternal) BackupToolInstanceInterface {
	return &BackupToolInstance{p: p}
}

type CreateBackupToolInstanceOptions struct {
	NamePrefix     string                       `yaml:"namePrefix,omitempty"`
	Volumes        []core.SingleContainerVolume `yaml:"volumes,omitempty"`
	CleanupTimeout helpers.MaxWaitTime          `yaml:"cleanupTimeout,omitempty"`
	PodWaitTimeout helpers.MaxWaitTime          `yaml:"podWaitTimeout,omitempty"`
}

func (p *Provider) CreateBackupToolInstance(ctx *contexts.Context, namespace, instance string, opts CreateBackupToolInstanceOptions) (btInstance BackupToolInstanceInterface, err error) {
	ctx.Log.Info("Creating backup tool instance")
	btInstance = p.newBackupToolInstance()

	namePrefix := opts.NamePrefix
	if namePrefix == "" {
		namePrefix = constants.ToolName
	}

	// Prepare to handle resource cleanup in the event of an error
	errHandler := func(originalErr error, args ...any) (BackupToolInstanceInterface, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.To(btInstance.Delete).
			WithErrMessage("failed to cleanup backup tool instance %q in namespace %q", namePrefix, namespace).
			WithOriginalErr(&originalErr).
			WithParentCtx(ctx).
			WithTimeout(opts.CleanupTimeout.MaxWait(10 * time.Minute)).
			RunError()
	}

	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			GRPC: &corev1.GRPCAction{
				Port:    grpc.GRPCPort,
				Service: new(grpc.SystemService),
			},
		},
		TimeoutSeconds:   1,
		PeriodSeconds:    3,
		SuccessThreshold: 1,
		FailureThreshold: 2, // TODO maybe set this to 1. If the probe fails three seconds apart, then it's probably not going to succeed on the next try.
	}

	volumes, mounts := core.ConvertSingleContainerVolumes(opts.Volumes)

	container := corev1.Container{
		Name:         constants.ToolName,
		Image:        constants.FullImageName,
		Command:      []string{constants.ToolName},
		Args:         []string{"grpc"},
		VolumeMounts: mounts,
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: grpc.GRPCPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		// Must run as root to read, chown, and chmod arbitrary files.
		// This does not require root on the host - just on the container.
		// Note: this is not compatible with pod-security.kubernetes.io/enforce: baseline
		SecurityContext: core.PrivilegedContainerSecurityContext(),
		StartupProbe:    probe,
		ReadinessProbe:  probe,
		LivenessProbe:   probe,
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: helpers.CleanName(namePrefix),
			Labels: map[string]string{
				"app.kubernetes.io/component": constants.ToolName + "-grpc-server",
			},
		},
		Spec: corev1.PodSpec{
			// This is not compatible with NFS mounts because the NFS client in the kernel (all versions)
			// does not support idmap mounts (MOUNT_ATTR_IDMAP).
			// HostUsers:       new(false), // Don't run as node root
			Containers:      []corev1.Container{container},
			RestartPolicy:   corev1.RestartPolicyNever,
			Volumes:         volumes,
			SecurityContext: core.PrivilegedPodSecurityContext(),
		},
	}

	ctx.Log.Step().Info("Creating GRPC pod")
	pod, err = p.core().CreatePod(ctx.Child(), namespace, pod)
	if err != nil {
		return errHandler(err, "failed to create pod %q", helpers.FullNameStr(namespace, namePrefix))
	}
	// Set the pod early so the deferred cleanup can delete it even if the readiness wait fails.
	btInstance.setPod(pod)

	readyPod, err := p.core().WaitForReadyPod(ctx.Child(), namespace, pod.Name, core.WaitForReadyPodOpts{MaxWaitTime: opts.PodWaitTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for pod %q to become ready", helpers.FullNameStr(namespace, pod.Name))
	}
	// Re-set with the ready pod, which has its assigned IP address populated. The GRPC client
	// connects directly to this pod IP (the driver always runs in-cluster), so no Service is needed.
	btInstance.setPod(readyPod)

	return btInstance, nil
}

func (b *BackupToolInstance) setPod(pod *corev1.Pod) {
	b.pod = pod
}

func (b *BackupToolInstance) GetPod() *corev1.Pod {
	return b.pod
}

// GetGRPCClient connects to the backup tool instance's GRPC server using the pod's IP address.
// The driver always runs in-cluster (as a Job/pod), so the pod IP is directly routable and there
// is no need for a Service, cluster DNS, or kube-proxy. The GRPC channel is plaintext, so there is
// no TLS hostname to verify against either.
func (b *BackupToolInstance) GetGRPCClient(ctx *contexts.Context) (clients.ClientInterface, error) {
	ctx.Log.Info("Creating GRPC client for backup tool instance")

	if b.pod == nil || b.pod.Status.PodIP == "" {
		return nil, trace.NotFound("backup tool instance pod has no IP address assigned")
	}

	address := net.JoinHostPort(b.pod.Status.PodIP, fmt.Sprintf("%d", grpc.GRPCPort))
	grpcClient, err := clients.NewClient(ctx.Child(), address)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to backup tool GRPC server at %q", address)
	}

	return grpcClient, nil
}

func (b *BackupToolInstance) Delete(ctx *contexts.Context) (err error) {
	ctx.Log.Info("Deleting backup tool instance")
	defer ctx.Log.Info("Completed backup tool instance deletion", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	if b.pod == nil {
		return nil
	}

	ctx.Log.Step().Info("Deleting GRPC pod", "name", b.pod.Name)

	err = b.p.core().DeletePod(ctx.Child(), b.pod.Namespace, b.pod.Name)
	return trace.Wrap(err, "failed to cleanup backup tool instance")
}
