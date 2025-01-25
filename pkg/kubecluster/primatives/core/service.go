package core

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func ContainerPortToServicePort(containerPort corev1.ContainerPort) corev1.ServicePort {
	port := corev1.ServicePort{
		Name:     containerPort.Name,
		Protocol: containerPort.Protocol,
		Port:     containerPort.ContainerPort,
	}

	if containerPort.Name != "" {
		port.TargetPort = intstr.FromString(containerPort.Name)
	} else {
		port.TargetPort = intstr.FromInt(int(containerPort.ContainerPort))
	}

	return port
}

// TODO resolve service dialer func?

func (c *Client) CreateService(ctx context.Context, namespce string, service *corev1.Service) (*corev1.Service, error) {
	service, err := c.client.CoreV1().Services(namespce).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create service %q", helpers.FullNameStr(namespce, service.Name))
	}

	return service, nil
}

type WaitForReadyServiceOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadyService(ctx context.Context, namespace, name string, opts WaitForReadyServiceOpts) error {
	_, err := helpers.WaitForResourceCondition(ctx, opts.MaxWait(time.Minute), c.client.CoreV1().Services(namespace), name,
		func(_ context.Context, service *corev1.Service) (interface{}, bool, error) {
			switch service.Spec.Type {
			case corev1.ServiceTypeExternalName:
				fallthrough
			case corev1.ServiceTypeLoadBalancer:
				// Ensure that at least one LB IP or hostname has been assigned
				externallyReady := lo.ContainsBy(service.Status.LoadBalancer.Ingress, func(ingress corev1.LoadBalancerIngress) bool {
					return ingress.IP != "" || ingress.Hostname != ""
				})
				if !externallyReady {
					return nil, false, nil
				}
			case corev1.ServiceTypeNodePort:
				fallthrough
			case corev1.ServiceTypeClusterIP:
				hasClusterIPSet := service.Spec.ClusterIP != "" || len(service.Spec.ClusterIPs) > 0
				return nil, hasClusterIPSet, nil
			}

			return nil, true, nil
		},
	)

	if err != nil {
		return trace.Wrap(err, "failed waiting for service to become ready")
	}

	err = c.WaitForReadyEndpoint(ctx, namespace, name, WaitForReadyEndpointOpts(opts))
	if err != nil {
		return trace.Wrap(err, "failed waiting for at least one service endpoint to become ready")
	}

	return nil
}

func (c *Client) DeleteService(ctx context.Context, namespace, name string) error {
	err := c.client.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete service %q", helpers.FullNameStr(namespace, name))
}
