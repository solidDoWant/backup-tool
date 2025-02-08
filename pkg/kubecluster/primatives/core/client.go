package core

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	// Pods
	CreatePod(ctx *contexts.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) // TODO see if this can be refined further
	WaitForReadyPod(ctx *contexts.Context, namespace, name string, opts WaitForReadyPodOpts) (*corev1.Pod, error)
	DeletePod(ctx *contexts.Context, namespace, name string) error
	// PVCs
	CreatePVC(ctx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts CreatePVCOptions) (*corev1.PersistentVolumeClaim, error)
	GetPVC(ctx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error)
	DoesPVCExist(ctx *contexts.Context, namespace, name string) (bool, error)
	EnsurePVCExists(ctx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts CreatePVCOptions) (*corev1.PersistentVolumeClaim, error)
	DeletePVC(ctx *contexts.Context, namespace, volumeName string) error
	// Services
	CreateService(ctx *contexts.Context, namespce string, service *corev1.Service) (*corev1.Service, error)
	WaitForReadyService(ctx *contexts.Context, namespace, name string, opts WaitForReadyServiceOpts) (*corev1.Service, error)
	DeleteService(ctx *contexts.Context, namespace, name string) error
	// Endpoints
	GetEndpoint(ctx *contexts.Context, namespace, name string) (*corev1.Endpoints, error)
	WaitForReadyEndpoint(ctx *contexts.Context, namespace, name string, opts WaitForReadyEndpointOpts) (*corev1.Endpoints, error)
}

type Client struct {
	client kubernetes.Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	underlyingKubernetesClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create kubernetes client")
	}

	return &Client{
		client: underlyingKubernetesClient,
	}, nil
}
