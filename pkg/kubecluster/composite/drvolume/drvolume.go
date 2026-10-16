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
	SnapshotAndWaitReady(ctx *contexts.Context, snapshotName string, opts DRVolumeSnapshotAndWaitOptions) error
	setPVC(pvc *corev1.PersistentVolumeClaim)
	setSnapshotLabels(labels map[string]string)
}

type DRVolumeCreateOptions struct {
	VolumeStorageClass string   `yaml:"volumeStorageClass,omitempty"`
	CNPGClusterNames   []string `yaml:"cnpgClusterNames,omitempty"`
	// SnapshotLabels are added to every backup snapshot of the DR volume, in addition to the tool-owned
	// and common labels. Tool-owned label keys are reserved.
	SnapshotLabels map[string]string `yaml:"snapshotLabels,omitempty"`
}

type DRVolume struct {
	p              providerInterfaceInternal
	pvc            *corev1.PersistentVolumeClaim
	snapshotLabels map[string]string
}

func newDRVolume(p providerInterfaceInternal) DRVolumeInterface {
	return &DRVolume{
		p: p,
	}
}

func (drv *DRVolume) setPVC(pvc *corev1.PersistentVolumeClaim) {
	drv.pvc = pvc
}

func (drv *DRVolume) setSnapshotLabels(labels map[string]string) {
	drv.snapshotLabels = labels
}

func (p *Provider) lookupCNPGClusterSize(ctx *contexts.Context, namespace, clusterName string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity

	ctx.Log.With("clusterName", clusterName).Debug("Getting the cluster size")
	cluster, err := p.cnpg().GetCluster(ctx.Child(), namespace, clusterName)
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

func (p *Provider) lookupCNPGClustersSize(ctx *contexts.Context, namespace string, clusterNames []string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity
	ctx.Log.Info("Looking up the core CNPG cluster sizes")

	storageSum := resource.NewQuantity(0, resource.BinarySI)
	for _, clusterName := range clusterNames {
		ctx.Log.Step()
		clusterSize, err := p.lookupCNPGClusterSize(ctx.Child(), namespace, clusterName)
		if err != nil {
			return defaultQuantityVal, trace.Wrap(err, "failed to get the cluster size")
		}
		storageSum.Add(clusterSize)
	}

	return *storageSum, nil
}

func (p *Provider) lookupDRVolumeSize(ctx *contexts.Context, namespace string, clusterNames []string) (resource.Quantity, error) {
	var defaultQuantityVal resource.Quantity
	ctx.Log.Info("Calculating the volume size based on the CNPG cluster sizes")

	storageSum := resource.NewQuantity(0, resource.BinarySI)
	if len(clusterNames) > 0 {
		clustersSize, err := p.lookupCNPGClustersSize(ctx.Child(), namespace, clusterNames)
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
// The DR volume and its backup snapshots are labeled with the tool-owned labels (identifying the backup
// name and DR type) and the client's common labels. Backup snapshots also get opts.SnapshotLabels.
func (p *Provider) NewDRVolume(ctx *contexts.Context, namespace, name, drType string, configuredSize resource.Quantity, opts DRVolumeCreateOptions) (DRVolumeInterface, error) {
	ctx.Log.Info("Ensuring the DR volume exists")

	// Validate before creating anything, so that a bad label doesn't fail the backup after all the work is done
	if err := ValidateBackupLabels(name, drType, opts.SnapshotLabels); err != nil {
		return nil, trace.Wrap(err, "invalid DR volume labels")
	}

	toolOwnedLabels := ToolOwnedLabels(name, drType)
	commonLabels := p.core().GetCommonLabels()
	// Tool-owned labels take precedence so that they can always be relied upon by selectors
	pvcLabels := mergeLabels(commonLabels, toolOwnedLabels)
	snapshotLabels := mergeLabels(commonLabels, opts.SnapshotLabels, toolOwnedLabels)

	drv := p.newDRVolume()

	drVolumeSize := configuredSize
	if drVolumeSize.IsZero() {
		var err error
		drVolumeSize, err = p.lookupDRVolumeSize(ctx.Child(), namespace, opts.CNPGClusterNames)
		if err != nil {
			return nil, trace.Wrap(err, "failed to calculate the volume size")
		}

		// Default to roughly twice the sum of the CNPG cluster sizes. This may still be too small. If it is, the user
		// should specify the volume size.
		drVolumeSize.Mul(2)
	}

	drPVC, err := p.core().EnsurePVCExists(ctx.Child(), namespace, name, drVolumeSize, core.CreatePVCOptions{StorageClassName: opts.VolumeStorageClass, Labels: pvcLabels})
	if err != nil {
		return nil, trace.Wrap(err, "failed to ensure backup volume exists")
	}

	// DR volumes created by older versions (or with different common labels) won't have all the labels
	if missing := missingLabels(drPVC.Labels, pvcLabels); len(missing) > 0 {
		ctx.Log.Info("Adding missing labels to the existing DR volume")
		drPVC, err = p.core().LabelPVC(ctx.Child(), namespace, name, missing)
		if err != nil {
			return nil, trace.Wrap(err, "failed to label the existing backup volume")
		}
	}

	drv.setPVC(drPVC)
	drv.setSnapshotLabels(snapshotLabels)
	return drv, nil
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
	snapshot, err := drv.p.es().SnapshotVolume(ctx.Child(), drv.pvc.Namespace, drv.pvc.Name, externalsnapshotter.SnapshotVolumeOptions{
		Name:          helpers.CleanName(snapshotName),
		SnapshotClass: opts.SnapshotClass,
		Labels:        drv.snapshotLabels,
	})
	if err != nil {
		return trace.Wrap(err, "failed to snapshot backup volume %q", helpers.FullName(drv.pvc))
	}

	_, err = drv.p.es().WaitForReadySnapshot(ctx.Child(), drv.pvc.Namespace, snapshot.Name, externalsnapshotter.WaitForReadySnapshotOpts{MaxWaitTime: opts.ReadyTimeout})
	if err != nil {
		return trace.Wrap(err, "failed to wait for backup snapshot %q to become ready", helpers.FullName(snapshot))
	}

	return nil
}
