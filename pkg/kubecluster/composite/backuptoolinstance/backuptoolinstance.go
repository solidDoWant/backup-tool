package backuptoolinstance

import (
	"fmt"
	"net"
	"time"

	"github.com/gravitational/trace"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/grpc"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type BackupToolInstanceInterface interface {
	setPod(pod *corev1.Pod)
	GetPod() *corev1.Pod
	setService(service *corev1.Service)
	GetService() *corev1.Service
	GetGRPCClient(ctx *contexts.Context, searchDomains ...string) (clients.ClientInterface, error)
	Delete(ctx *contexts.Context) error
}

type BackupToolInstance struct {
	p       providerInterfaceInternal
	pod     *corev1.Pod
	service *corev1.Service
	// Used to mocking in during tests
	testConnection func(ctx *contexts.Context, address string) bool
	lookupIP       func(ctx *contexts.Context, host string) ([]net.IP, error)
}

func newBackupToolInstance(p providerInterfaceInternal) BackupToolInstanceInterface {
	return &BackupToolInstance{
		p: p,
		// Standard implementations. These just need to be vars to help with testing.
		testConnection: func(ctx *contexts.Context, address string) (succeeded bool) {
			// Short-circuit if the context is done
			select {
			case <-ctx.Done():
				return false
			default:
			}

			ctx.Log.With("address", address).Debug("Testing connection to address")
			defer ctx.Log.Debug("Completed connection test to address", "success", succeeded, ctx.Stopwatch.Keyval())

			address = net.JoinHostPort(address, fmt.Sprintf("%d", grpc.GRPCPort))
			conn, err := net.DialTimeout("tcp", address, 1*time.Second)
			if conn != nil {
				conn.Close()
			}

			return err == nil
		},
		lookupIP: func(ctx *contexts.Context, host string) (ips []net.IP, err error) {
			// Short-circuit if the context is done
			select {
			case <-ctx.Done():
				return nil, trace.Wrap(ctx.Err(), "context cancelled")
			default:
			}

			ctx.Log.With("host", host).Debug("Looking up IP address for host")
			defer ctx.Log.Debug("Completed IP address lookup for host", "ips", ips, ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

			return net.LookupIP(host)
		},
	}
}

type CreateBackupToolInstanceOptions struct {
	NamePrefix         string                       `yaml:"namePrefix,omitempty"`
	Volumes            []core.SingleContainerVolume `yaml:"volumes,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime          `yaml:"cleanupTimeout,omitempty"`
	ServiceType        corev1.ServiceType           `yaml:"serviceType,omitempty"`
	PodWaitTimeout     helpers.MaxWaitTime          `yaml:"podWaitTimeout,omitempty"`
	ServiceWaitTimeout helpers.MaxWaitTime          `yaml:"serviceWaitTimeout,omitempty"`
}

func (p *Provider) CreateBackupToolInstance(ctx *contexts.Context, namespace, instance string, opts CreateBackupToolInstanceOptions) (btInstance BackupToolInstanceInterface, err error) {
	ctx.Log.Info("Creating backup tool instance")
	btInstance = p.newBackupToolInstance()

	namePrefix := opts.NamePrefix
	if namePrefix == "" {
		namePrefix = constants.ToolName
	}

	// Prepare to handle resource cleanup in the event of an error
	errHandler := func(originalErr error, args ...interface{}) (BackupToolInstanceInterface, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.To(btInstance.Delete).
			WithErrMessage("failed to cleanup backup tool instance %q in namespace %q", namePrefix, namespace).
			WithOriginalErr(&originalErr).
			WithParentCtx(ctx).
			WithTimeout(opts.CleanupTimeout.MaxWait(10 * time.Minute)).
			Run()
	}

	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			GRPC: &corev1.GRPCAction{
				Port:    grpc.GRPCPort,
				Service: ptr.To(grpc.SystemService),
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
			// HostUsers:       ptr.To(false), // Don't run as node root
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
	btInstance.setPod(pod)

	readyPod, err := p.core().WaitForReadyPod(ctx.Child(), namespace, pod.Name, core.WaitForReadyPodOpts{MaxWaitTime: opts.PodWaitTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for pod %q to become ready", helpers.FullNameStr(namespace, pod.Name))
	}

	serviceType := opts.ServiceType
	if serviceType == "" {
		serviceType = corev1.ServiceTypeClusterIP
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: helpers.CleanName(namePrefix),
		},
		Spec: corev1.ServiceSpec{
			Selector: readyPod.Labels,
			Type:     serviceType,
			Ports: lo.Map(container.Ports, func(port corev1.ContainerPort, _ int) corev1.ServicePort {
				return core.ContainerPortToServicePort(port)
			}),
		},
	}

	ctx.Log.Step().Info("Creating GRPC service")
	service, err = p.core().CreateService(ctx.Child(), namespace, service)
	if err != nil {
		return errHandler(err, "failed to create service %q", helpers.FullNameStr(namespace, namePrefix))
	}
	btInstance.setService(service)

	_, err = p.core().WaitForReadyService(ctx.Child(), namespace, service.Name, core.WaitForReadyServiceOpts{MaxWaitTime: opts.ServiceWaitTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for service %q to become ready", helpers.FullNameStr(namespace, service.Name))
	}

	return btInstance, nil
}

func (b *BackupToolInstance) setPod(pod *corev1.Pod) {
	b.pod = pod
}

func (b *BackupToolInstance) GetPod() *corev1.Pod {
	return b.pod
}

func (b *BackupToolInstance) setService(service *corev1.Service) {
	b.service = service
}

func (b *BackupToolInstance) GetService() *corev1.Service {
	return b.service
}

func (b *BackupToolInstance) GetGRPCClient(ctx *contexts.Context, searchDomains ...string) (clients.ClientInterface, error) {
	ctx.Log.Info("Creating GRPC client for backup tool instance")

	endpoint, err := b.findReachableServiceAddress(ctx.Child(), searchDomains)
	if err != nil {
		return nil, trace.Wrap(err, "failed to find reachable service address for backup tool instance")
	}

	address := net.JoinHostPort(endpoint, fmt.Sprintf("%d", grpc.GRPCPort))
	grpcClient, err := clients.NewClient(ctx.Child(), address)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to backup tool GRPC server at %q", endpoint)
	}

	return grpcClient, nil
}

// Look through the service's DNS records, cluster IPs, and external IPs to find a reachable address from the current environment.
// This is needed to support running the tool locally, with another instance deployed to a cluster at runtime
func (b *BackupToolInstance) findReachableServiceAddress(ctx *contexts.Context, searchDomains []string) (address string, err error) {
	ctx.Log.Debug("Finding reachable service address for backup tool instance", "searchDomains", searchDomains)
	defer ctx.Log.Debug("Completed reachable service address search for backup tool instance", "address", address, ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	actualSearchDomains := make([]string, 0, len(searchDomains)+2)
	for _, searchDomains := range searchDomains {
		actualSearchDomains = append(actualSearchDomains, "."+searchDomains)
	}
	actualSearchDomains = append(actualSearchDomains, "")

	parentDomainComponents := []string{"", "." + b.service.Namespace, ".svc", ".cluster.local"}
	for _, searchDomainComponent := range actualSearchDomains {
		parentDomain := ""
		for _, parentDomainComponent := range parentDomainComponents {
			parentDomain += parentDomainComponent
			domain := fmt.Sprintf("%s%s%s", b.service.Name, parentDomain, searchDomainComponent)

			// Attempt to resolve the domain
			// Errors don't matter here - just check if any IPs were returned
			ips, _ := b.lookupIP(ctx.Child(), domain)
			if len(ips) == 0 {
				continue
			}

			for _, ip := range ips {
				if b.testConnection(ctx.Child(), ip.String()) {
					// Return the domain, not the IP. This is important for TLS verification during
					// the actual GRPC connection.
					return domain, nil
				}
			}
		}
	}

	// Cluster IP check
	for _, clusterIP := range b.service.Spec.ClusterIPs {
		if b.testConnection(ctx.Child(), clusterIP) {
			return clusterIP, nil
		}
	}

	// External IP check
	for _, ingress := range b.service.Status.LoadBalancer.Ingress {

		if ingress.IP != "" && b.testConnection(ctx.Child(), ingress.IP) {
			return ingress.IP, nil
		}
		if ingress.Hostname != "" && b.testConnection(ctx.Child(), ingress.Hostname) {
			return ingress.Hostname, nil
		}
	}

	return "", trace.NotFound("no reachable service address found")
}

func (b *BackupToolInstance) Delete(ctx *contexts.Context) (err error) {
	ctx.Log.Info("Deleting backup tool instance")
	defer ctx.Log.Info("Completed backup tool instance deletion", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	cleanupErrs := make([]error, 0, 2)

	if b.pod != nil {
		ctx.Log.Step().Info("Deleting GRPC pod", "name", b.pod.Name)
		err := b.p.core().DeletePod(ctx.Child(), b.pod.Namespace, b.pod.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}

	if b.service != nil {
		ctx.Log.Step().Info("Deleting GRPC service", "name", b.service.Name)
		err := b.p.core().DeleteService(ctx.Child(), b.service.Namespace, b.service.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed to cleanup backup tool instance")
}
