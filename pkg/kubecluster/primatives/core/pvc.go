package core

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
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

func (c *Client) CreatePVC(ctx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
	ctx.Log.With("name", pvcName).Info("Creating PVC")
	ctx.Log.Debug("Call parameters", "size", size.String(), "opts", opts)

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

func (kc *Client) GetPVC(ctx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	ctx.Log.With("name", name).Info("Getting PVC")

	pvc, err := kc.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, meta_v1.GetOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to query cluster for PVC %q", helpers.FullNameStr(namespace, name))
	}

	ctx.Log.Debug("Retrieved PVC", "pvc", pvc)
	return pvc, nil
}

func (c *Client) DoesPVCExist(ctx *contexts.Context, namespace, name string) (doesExist bool, err error) {
	ctx.Log.With("name", name).Info("Checking if PVC exists")
	defer ctx.Log.Debug("PVC status", "exists", doesExist, contexts.ErrorKeyvals(&err))

	_, err = c.GetPVC(ctx.Child(), namespace, name)
	if err == nil {
		return true, nil
	}

	if apierrors.IsNotFound(err) {
		return false, nil
	}

	return false, trace.Wrap(err, "failed to query cluster for PVC %q", helpers.FullNameStr(namespace, name))
}

func (c *Client) EnsurePVCExists(ctx *contexts.Context, namespace, pvcName string, size resource.Quantity, opts CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
	ctx.Log.With("name", pvcName).Info("Ensuring PVC exists")

	pvc, err := c.GetPVC(ctx.Child(), namespace, pvcName)
	if err == nil {
		return pvc, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, trace.Wrap(err, "failed to query cluster for PVC %q", helpers.FullNameStr(namespace, pvcName))
	}

	pvc, err = c.CreatePVC(ctx.Child(), namespace, pvcName, size, opts)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create PVC %q", helpers.FullNameStr(namespace, pvcName))
	}

	return pvc, nil
}

func (c *Client) DeletePVC(ctx *contexts.Context, namespace, volumeName string) error {
	ctx.Log.With("name", volumeName).Info("Deleting PVC")

	err := c.client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, volumeName, meta_v1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete volume %q in namespace %q", volumeName, namespace)
}
