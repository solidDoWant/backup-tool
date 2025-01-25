package core

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreatePVCOptions struct {
	helpers.GenerateName
	StorageClassName string
	Source           *corev1.TypedObjectReference
}

func (c *Client) CreatePVC(ctx context.Context, namespace, pvcName string, size resource.Quantity, opts CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
			DataSourceRef: opts.Source,
		},
	}

	opts.SetName(&pvc.ObjectMeta, pvcName)

	if opts.StorageClassName != "" {
		pvc.Spec.StorageClassName = &opts.StorageClassName
	}

	pvc, err := c.client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, meta_v1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create pvc %q", helpers.FullName(pvc))
	}

	return pvc, nil
}

func (kc *Client) GetPVC(ctx context.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := kc.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, meta_v1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to query cluster for PVC %q", helpers.FullNameStr(namespace, name))
	}

	return pvc, nil
}

func (c *Client) DoesPVCExist(ctx context.Context, namespace, name string) (bool, error) {
	_, err := c.GetPVC(ctx, namespace, name)
	if err == nil {
		return true, nil
	}

	if apierrors.IsNotFound(err) {
		return false, nil
	}

	return false, trace.Wrap(err, "failed to query cluster for PVC %q", helpers.FullNameStr(namespace, name))
}

func (c *Client) EnsurePVCExists(ctx context.Context, namespace, pvcName string, size resource.Quantity, opts CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := c.GetPVC(ctx, namespace, pvcName)
	if err == nil {
		return pvc, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, trace.Wrap(err, "failed to query cluster for PVC %q", helpers.FullNameStr(namespace, pvcName))
	}

	pvc, err = c.CreatePVC(ctx, namespace, pvcName, size, opts)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create PVC %q", helpers.FullNameStr(namespace, pvcName))
	}

	return pvc, nil
}

func (c *Client) DeleteVolume(ctx context.Context, namespace, volumeName string) error {
	err := c.client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, volumeName, meta_v1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete volume %q in namespace %q", volumeName, namespace)
}
