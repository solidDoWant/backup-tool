package kubecluster

import (
	context "context"
	"fmt"
	"net"
	"time"

	"github.com/gravitational/trace"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/constants"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/grpc/servers"
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
	GetGRPCClient(ctx context.Context, searchDomains ...string) (clients.ClientInterface, error)
	Delete(ctx context.Context) error
}

type BackupToolInstance struct {
	c       ClientInterface
	pod     *corev1.Pod
	service *corev1.Service
	// Used to mocking in during tests
	testConnection func(address string) bool
	lookupIP       func(host string) ([]net.IP, error)
}

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

func NewSingleContainerSecret(secretName, mountPath string) SingleContainerVolume {
	return SingleContainerVolume{
		Name:      secretName,
		MountPath: mountPath,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To(int32(0400)), // Read only by owner
			},
		},
	}
}

func newBackupToolInstance(c ClientInterface) BackupToolInstanceInterface {
	return &BackupToolInstance{
		c: c,
		// Standard implementations. These just need to be vars to help with testing.
		testConnection: func(address string) bool {
			address = net.JoinHostPort(address, fmt.Sprintf("%d", servers.GRPCPort))
			conn, err := net.DialTimeout("tcp", address, 1*time.Second)
			if conn != nil {
				conn.Close()
			}

			return err == nil
		},
		lookupIP: net.LookupIP,
	}
}

type CreateBackupToolInstanceOptions struct {
	NamePrefix         string                  `yaml:"namePrefix,omitempty"`
	Volumes            []SingleContainerVolume `yaml:"volumes,omitempty"`
	CleanupTimeout     helpers.MaxWaitTime     `yaml:"cleanupTimeout,omitempty"`
	ServiceType        corev1.ServiceType      `yaml:"serviceType,omitempty"`
	PodWaitTimeout     helpers.MaxWaitTime     `yaml:"podWaitTimeout,omitempty"`
	ServiceWaitTimeout helpers.MaxWaitTime     `yaml:"serviceWaitTimeout,omitempty"`
}

func (c *Client) CreateBackupToolInstance(ctx context.Context, namespace, instance string, opts CreateBackupToolInstanceOptions) (btInstance BackupToolInstanceInterface, err error) {
	btInstance = c.newBackupToolInstance()

	namePrefix := opts.NamePrefix
	if namePrefix == "" {
		namePrefix = constants.ToolName + "-"
	}

	// Prepare to handle resource cleanup in the event of an error
	errHandler := func(originalErr error, args ...interface{}) (BackupToolInstanceInterface, error) {
		originalErr = trace.Wrap(originalErr, args...)
		return nil, cleanup.WithTimeoutTo(opts.CleanupTimeout.MaxWait(10*time.Minute), btInstance.Delete).
			WithErrMessage("failed to cleanup backup tool instance %q in namespace %q", namePrefix, namespace).
			WithOriginalErr(&originalErr).
			Run()
	}

	container := corev1.Container{
		Name:    constants.ToolName,
		Image:   constants.FullImageName,
		Command: []string{constants.ToolName},
		Args:    []string{"grpc"},
		VolumeMounts: lo.Map(opts.Volumes, func(vol SingleContainerVolume, _ int) corev1.VolumeMount {
			return corev1.VolumeMount{
				Name:      vol.Name,
				MountPath: vol.MountPath,
			}
		}),
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: servers.GRPCPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
			Labels: map[string]string{
				"app.kubernetes.io/name":     constants.ToolName,
				"app.kubernetes.io/instance": instance,
			},
		},
		// TODO security, probes, etc
		Spec: corev1.PodSpec{
			Containers:    []corev1.Container{container},
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: lo.Map(opts.Volumes, func(vol SingleContainerVolume, _ int) corev1.Volume {
				return corev1.Volume{
					Name:         vol.Name,
					VolumeSource: vol.VolumeSource,
				}
			}),
		},
	}

	pod, err = c.Core().CreatePod(ctx, namespace, pod)
	if err != nil {
		return errHandler(err, "failed to create pod %q", helpers.FullNameStr(namespace, namePrefix))
	}
	btInstance.setPod(pod)

	err = c.Core().WaitForReadyPod(ctx, namespace, pod.Name, core.WaitForReadyPodOpts{MaxWaitTime: opts.PodWaitTimeout})
	if err != nil {
		return errHandler(err, "failed to wait for pod %q to become ready", helpers.FullNameStr(namespace, pod.Name))
	}

	serviceType := opts.ServiceType
	if serviceType == "" {
		serviceType = corev1.ServiceTypeClusterIP
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
		},
		Spec: corev1.ServiceSpec{
			Selector: pod.Labels,
			Type:     serviceType,
			Ports: lo.Map(container.Ports, func(port corev1.ContainerPort, _ int) corev1.ServicePort {
				return core.ContainerPortToServicePort(port)
			}),
		},
	}

	service, err = c.Core().CreateService(ctx, namespace, service)
	if err != nil {
		return errHandler(err, "failed to create service %q", helpers.FullNameStr(namespace, namePrefix))
	}
	btInstance.setService(service)

	err = c.Core().WaitForReadyService(ctx, namespace, service.Name, core.WaitForReadyServiceOpts{MaxWaitTime: opts.ServiceWaitTimeout})
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

func (b *BackupToolInstance) GetGRPCClient(ctx context.Context, searchDomains ...string) (clients.ClientInterface, error) {
	endpoint, err := b.findReachableServiceAddress(searchDomains)
	if err != nil {
		return nil, trace.Wrap(err, "failed to find reachable service address for backup tool instance")
	}

	grpcClient, err := clients.NewClient(ctx, endpoint)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to backup tool GRPC server at %q", endpoint)
	}

	return grpcClient, nil
}

// Look through the service's DNS records, cluster IPs, and external IPs to find a reachable address from the current environment.
// This is needed to support running the tool locally, with another instance deployed to a cluster at runtime
func (b *BackupToolInstance) findReachableServiceAddress(searchDomains []string) (string, error) {
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
			ips, _ := b.lookupIP(domain)
			if len(ips) == 0 {
				continue
			}

			for _, ip := range ips {
				if b.testConnection(ip.String()) {
					// Return the domain, not the IP. This is important for TLS verification during
					// the actual GRPC connection.
					return domain, nil
				}
			}
		}
	}

	// Cluster IP check
	for _, clusterIP := range b.service.Spec.ClusterIPs {
		if b.testConnection(clusterIP) {
			return clusterIP, nil
		}
	}

	// External IP check
	for _, ingress := range b.service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" && b.testConnection(ingress.IP) {
			return ingress.IP, nil
		}
		if ingress.Hostname != "" && b.testConnection(ingress.Hostname) {
			return ingress.Hostname, nil
		}
	}

	return "", trace.NotFound("no reachable service address found")
}

func (b *BackupToolInstance) Delete(ctx context.Context) error {
	cleanupErrs := make([]error, 0, 2)

	if b.pod != nil {
		err := b.c.Core().DeletePod(ctx, b.pod.Namespace, b.pod.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}

	if b.service != nil {
		err := b.c.Core().DeleteService(ctx, b.service.Namespace, b.service.Name)
		if err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}

	return trace.Wrap(trace.NewAggregate(cleanupErrs...), "failed to cleanup backup tool instance")
}
