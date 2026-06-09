package clonepvc

import (
	"testing"

	volumegroupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClonePVCGroup(t *testing.T) {
	namespace := "test-ns"
	groupSnapshotName := "test-group-snapshot"
	size := resource.MustParse("5Gi")
	selector := metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}

	groupSnapshot := &volumegroupsnapshotv1.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: groupSnapshotName, Namespace: namespace},
	}

	// memberFor builds a fully-reconciled member VolumeSnapshot: it references its source PVC and has
	// a restore size (WaitForReadyGroupSnapshotMembers only returns members in this state).
	memberFor := func(snapshotName, sourcePVC string) *volumesnapshotv1.VolumeSnapshot {
		return &volumesnapshotv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: snapshotName, Namespace: namespace},
			Spec: volumesnapshotv1.VolumeSnapshotSpec{
				Source: volumesnapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: new(sourcePVC)},
			},
			Status: &volumesnapshotv1.VolumeSnapshotStatus{RestoreSize: &size},
		}
	}

	defaultMembers := []*volumesnapshotv1.VolumeSnapshot{
		memberFor("snap-a", "pvc-a"),
		memberFor("snap-b", "pvc-b"),
	}

	tests := []struct {
		desc                 string
		opts                 ClonePVCGroupOptions
		members              []*volumesnapshotv1.VolumeSnapshot
		simulateListPVCsErr  bool
		simulateNoPVCs       bool
		simulateSnapshotErr  bool
		simulateWaitErr      bool
		simulateMembersErr   bool
		simulateGetPVCErr    bool
		simulateCreatePVCErr bool
		simulateForceBindErr bool
	}{
		{
			desc:    "successful clone with default options",
			members: defaultMembers,
		},
		{
			desc:    "successful clone with force bind",
			opts:    ClonePVCGroupOptions{ForceBind: true},
			members: defaultMembers,
		},
		{
			desc:    "successful clone with destination storage class override",
			opts:    ClonePVCGroupOptions{DestStorageClassName: "other-class"},
			members: defaultMembers,
		},
		{
			desc:                "error listing PVCs",
			members:             defaultMembers,
			simulateListPVCsErr: true,
		},
		{
			desc:           "selector matches no PVCs",
			members:        defaultMembers,
			simulateNoPVCs: true,
		},
		{
			desc:                "error creating group snapshot",
			members:             defaultMembers,
			simulateSnapshotErr: true,
		},
		{
			desc:            "error waiting for group snapshot",
			members:         defaultMembers,
			simulateWaitErr: true,
		},
		{
			desc:               "error waiting for members",
			members:            defaultMembers,
			simulateMembersErr: true,
		},
		{
			desc:              "error getting source PVC",
			members:           defaultMembers,
			simulateGetPVCErr: true,
		},
		{
			desc:                 "error creating cloned PVC",
			members:              defaultMembers,
			simulateCreatePVCErr: true,
		},
		{
			desc:                 "error force binding cloned volumes",
			opts:                 ClonePVCGroupOptions{ForceBind: true},
			members:              defaultMembers,
			simulateForceBindErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := newMockProvider(t)
			ctx := th.NewTestContext()

			anyCloneCreated := false

			func() {
				expectedCount := len(tt.members)
				p.coreClient.EXPECT().ListPVCs(mock.Anything, namespace, core.ListPVCsOptions{LabelSelector: selector}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace string, opts core.ListPVCsOptions) ([]corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						if tt.simulateListPVCsErr {
							return nil, assert.AnError
						}
						if tt.simulateNoPVCs {
							return []corev1.PersistentVolumeClaim{}, nil
						}
						return make([]corev1.PersistentVolumeClaim, expectedCount), nil
					})
				if tt.simulateListPVCsErr || tt.simulateNoPVCs {
					return
				}

				p.esClient.EXPECT().GroupSnapshotVolumes(mock.Anything, namespace, selector, externalsnapshotter.GroupSnapshotOptions{Name: tt.opts.GroupSnapshotName, SnapshotClass: tt.opts.SnapshotClass}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace string, selector metav1.LabelSelector, opts externalsnapshotter.GroupSnapshotOptions) (*volumegroupsnapshotv1.VolumeGroupSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(groupSnapshot, tt.simulateSnapshotErr)
					})
				if tt.simulateSnapshotErr {
					return
				}

				// The group snapshot is only deleted on error (the caller owns it on success).
				anyErrorExpected := tt.simulateWaitErr || tt.simulateMembersErr || tt.simulateGetPVCErr ||
					tt.simulateCreatePVCErr || tt.simulateForceBindErr
				if anyErrorExpected {
					p.esClient.EXPECT().DeleteGroupSnapshot(mock.Anything, namespace, groupSnapshotName).
						RunAndReturn(func(cleanupCtx *contexts.Context, namespace, name string) error {
							assert.NotEqual(t, ctx, cleanupCtx)
							return nil
						})
				}

				p.esClient.EXPECT().WaitForReadyGroupSnapshot(mock.Anything, namespace, groupSnapshotName, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, opts externalsnapshotter.WaitForReadyGroupSnapshotOpts) (*volumegroupsnapshotv1.VolumeGroupSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(groupSnapshot, tt.simulateWaitErr)
					})
				if tt.simulateWaitErr {
					return
				}

				p.esClient.EXPECT().WaitForReadyGroupSnapshotMembers(mock.Anything, namespace, groupSnapshotName, expectedCount, mock.Anything).
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, count int, opts externalsnapshotter.WaitForReadyGroupSnapshotMembersOpts) ([]*volumesnapshotv1.VolumeSnapshot, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						if tt.simulateMembersErr {
							return nil, assert.AnError
						}
						return tt.members, nil
					})
				if tt.simulateMembersErr {
					return
				}

				for _, member := range tt.members {
					member := member
					sourcePVC := *member.Spec.Source.PersistentVolumeClaimName

					if tt.opts.DestStorageClassName == "" {
						p.coreClient.EXPECT().GetPVC(mock.Anything, namespace, sourcePVC).
							RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
								assert.True(t, calledCtx.IsChildOf(ctx))
								return th.ErrOr1Val(&corev1.PersistentVolumeClaim{
									ObjectMeta: metav1.ObjectMeta{Name: sourcePVC, Namespace: namespace},
									Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: new("standard")},
								}, tt.simulateGetPVCErr)
							})
						if tt.simulateGetPVCErr {
							break
						}
					}

					expectedStorageClass := "standard"
					if tt.opts.DestStorageClassName != "" {
						expectedStorageClass = tt.opts.DestStorageClassName
					}

					p.coreClient.EXPECT().CreatePVC(mock.Anything, namespace, sourcePVC, size, core.CreatePVCOptions{
						GenerateName:     true,
						StorageClassName: expectedStorageClass,
						Source: &corev1.TypedObjectReference{
							APIGroup: new(volumesnapshotv1.SchemeGroupVersion.Group),
							Kind:     externalsnapshotter.VolumeSnapshotKind,
							Name:     member.Name,
						},
					}).RunAndReturn(func(calledCtx *contexts.Context, namespace, name string, size resource.Quantity, opts core.CreatePVCOptions) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrOr1Val(&corev1.PersistentVolumeClaim{
							ObjectMeta: metav1.ObjectMeta{Name: "clone-" + name, Namespace: namespace},
						}, tt.simulateCreatePVCErr)
					})
					if !tt.simulateCreatePVCErr {
						anyCloneCreated = true
					}
					if tt.simulateCreatePVCErr {
						break
					}
				}

				// On any error after at least one clone exists, clones are deleted during cleanup.
				cleanupExpected := tt.simulateCreatePVCErr || tt.simulateForceBindErr
				if cleanupExpected && anyCloneCreated {
					p.coreClient.EXPECT().DeletePVC(mock.Anything, namespace, mock.Anything).
						RunAndReturn(func(cleanupCtx *contexts.Context, namespace, name string) error {
							assert.NotEqual(t, ctx, cleanupCtx)
							return nil
						}).Maybe()
				}

				if tt.simulateGetPVCErr || tt.simulateCreatePVCErr {
					return
				}

				if !tt.opts.ForceBind {
					return
				}

				// Force-binding runs the internal forceBindVolumes helper against the core client.
				p.expectForceBind(t, ctx, namespace, tt.simulateForceBindErr)
			}()

			result, err := p.ClonePVCGroup(ctx, namespace, selector, tt.opts)
			if th.ErrExpected(
				tt.simulateListPVCsErr,
				tt.simulateNoPVCs,
				tt.simulateSnapshotErr,
				tt.simulateWaitErr,
				tt.simulateMembersErr,
				tt.simulateGetPVCErr,
				tt.simulateCreatePVCErr,
				tt.simulateForceBindErr,
			) {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, groupSnapshot, result.GroupSnapshot)
			require.Len(t, result.ClonedPVCs, len(tt.members))
			for _, member := range tt.members {
				require.Contains(t, result.ClonedPVCs, *member.Spec.Source.PersistentVolumeClaimName)
			}
		})
	}
}
