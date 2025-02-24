package drvolume

import (
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type DRVolumeInterface interface {
	EnsureExists(ctx *contexts.Context, namespace, name string, configuredSize resource.Quantity, opts DRVolumeCreateOptions) error
	SnapshotAndWaitReady(ctx *contexts.Context, snapshotName string, opts DRVolumeSnapshotAndWaitOptions) error
}

type DRVolumeCreateOptions struct {
	VolumeStorageClass string   `yaml:"volumeStorageClass,omitempty"`
	CNPGClusterNames   []string `yaml:"cnpgClusterNames,omitempty"`
}

type DRVolume struct {
	p   providerInterfaceInternal
	pvc *corev1.PersistentVolumeClaim
}

func newDRVolume(p providerInterfaceInternal) DRVolumeInterface {
	return &DRVolume{
		p: p,
	}
}

func (drv *DRVolume) lookupCNPGClusterSize(ctx *contexts.Context, namespace, clusterName string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity

	ctx.Log.With("clusterName", clusterName).Debug("Getting the cluster size")
	cluster, err := drv.p.cnpg().GetCluster(ctx.Child(), namespace, clusterName)
	if err != nil {
		return defaultQuantityVal, trace.Wrap(err, "failed to get the %q cluster", helpers.FullNameStr(namespace, clusterName))
	}

	ctx.Log.With("clusterSize", cluster.Spec.StorageConfiguration.Size).Debug("Parsing the cluster size")
	clusterSize, err := resource.ParseQuantity(cluster.Spec.StorageConfiguration.Size)
	if err != nil {
		return defaultQuantityVal, trace.Wrap(err, "failed to parse the %q cluster size %q", helpers.FullName(cluster), cluster.Spec.StorageConfiguration.Size)
	}

	return clusterSize, nil
}

func (drv *DRVolume) lookupCNPGClustersSize(ctx *contexts.Context, namespace string, clusterNames []string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity
	ctx.Log.Info("Looking up the core CNPG cluster sizes")

	storageSum := resource.NewQuantity(0, resource.BinarySI)
	for _, clusterName := range clusterNames {
		ctx.Log.Step()
		clusterSize, err := drv.lookupCNPGClusterSize(ctx.Child(), namespace, clusterName)
		if err != nil {
			return defaultQuantityVal, trace.Wrap(err, "failed to get the cluster size")
		}
		storageSum.Add(clusterSize)
	}

	return *storageSum, nil
}

func (drv *DRVolume) lookupDRVolumeSize(ctx *contexts.Context, namespace string, clusterNames []string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity
	ctx.Log.Info("Calculating the volume size based on the CNPG cluster sizes")

	storageSum := resource.NewQuantity(0, resource.BinarySI)
	if len(clusterNames) > 0 {
		clustersSize, err := drv.lookupCNPGClustersSize(ctx.Child(), namespace, clusterNames)
		if err != nil {
			return defaultQuantityVal, trace.Wrap(err, "failed to get the CNPG cluster sizes")
		}

		storageSum.Add(clustersSize)
	}

	if storageSum.IsZero() {
		return defaultQuantityVal, trace.Errorf("calculated storage size of 0")
	}

	return *storageSum, nil
}

// TODO handle expansion?
func (drv *DRVolume) EnsureExists(ctx *contexts.Context, namespace, name string, configuredSize resource.Quantity, opts DRVolumeCreateOptions) error {
	ctx.Log.Info("Ensuring the DR volume exists")

	drVolumeSize := configuredSize
	if drVolumeSize.IsZero() {
		var err error
		drVolumeSize, err = drv.lookupDRVolumeSize(ctx.Child(), namespace, opts.CNPGClusterNames)
		if err != nil {
			return trace.Wrap(err, "failed to calculate the volume size")
		}

		// Default to roughly twice the sum of the CNPG cluster sizes. This may still be too small. If it is, the user
		// should specify the volume size.
		drVolumeSize.Mul(2)
	}

	drPVC, err := drv.p.core().EnsurePVCExists(ctx.Child(), namespace, name, drVolumeSize, core.CreatePVCOptions{StorageClassName: opts.VolumeStorageClass})
	if err != nil {
		return trace.Wrap(err, "failed to ensure backup volume exists")
	}

	drv.pvc = drPVC
	return nil
}

type DRVolumeSnapshotAndWaitOptions struct {
	ReadyTimeout  helpers.MaxWaitTime `yaml:"snapshotReadyTimeout,omitempty"`
	SnapshotClass string              `yaml:"snapshotClass,omitempty"`
}

func (drv *DRVolume) SnapshotAndWaitReady(ctx *contexts.Context, snapshotName string, opts DRVolumeSnapshotAndWaitOptions) error {
	ctx.Log.With("snapshotName", snapshotName).Info("Snapshotting the DR volume")

	if drv.pvc == nil {
		return trace.Errorf("no PVC to snapshot")
	}

	ctx.Log.Step().Info("Snapshotting the DR volume")
	snapshot, err := drv.p.es().SnapshotVolume(ctx.Child(), drv.pvc.Namespace, drv.pvc.Name, externalsnapshotter.SnapshotVolumeOptions{Name: helpers.CleanName(snapshotName), SnapshotClass: opts.SnapshotClass})
	if err != nil {
		return trace.Wrap(err, "failed to snapshot backup volume %q", helpers.FullName(drv.pvc))
	}

	_, err = drv.p.es().WaitForReadySnapshot(ctx.Child(), drv.pvc.Namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: opts.ReadyTimeout})
	if err != nil {
		return trace.Wrap(err, "failed to wait for backup snapshot %q to become ready", helpers.FullName(snapshot))
	}

	return nil
}
