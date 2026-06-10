package groupbackup

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	volumegroupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/layout"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/externalsnapshotter"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testSelector() metav1.LabelSelector {
	return metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}
}

func TestFilesGroupBackupOptions(t *testing.T) {
	th.OptStructTest[FilesGroupBackupOptions](t)
}

func TestConfigure(t *testing.T) {
	expectedState := &configureState{
		kubeClusterClient: kubecluster.NewMockClientInterface(t),
		namespace:         "namespace",
		selector:          testSelector(),
		drVolName:         "drVolName",
		groupName:         "groupName",
		opts: FilesGroupBackupOptions{
			CleanupTimeout: helpers.ShortWaitTime,
		},
	}

	fb := NewFilesGroupBackup()
	err := fb.Configure(
		expectedState.kubeClusterClient,
		expectedState.namespace,
		expectedState.selector,
		expectedState.drVolName,
		expectedState.groupName,
		expectedState.opts,
	)

	t.Run("successfully configures the first time", func(t *testing.T) {
		require.NoError(t, err)
	})

	t.Run("all state vars are populated", func(t *testing.T) {
		casted := fb.(*FilesGroupBackup)

		assert.NotEqual(t, "", casted.uid)
		assert.NotEqual(t, uuid.Nil.String(), casted.uid)
		expectedState.uid = casted.uid

		assert.True(t, casted.isConfigured)
		expectedState.isConfigured = casted.isConfigured

		assert.Equal(t, expectedState, &casted.configureState)
	})

	t.Run("fails to configure because already configured", func(t *testing.T) {
		err = fb.Configure(
			expectedState.kubeClusterClient,
			expectedState.namespace,
			expectedState.selector,
			expectedState.drVolName,
			expectedState.groupName,
			expectedState.opts,
		)
		assert.Error(t, err)
	})
}

func TestValidate(t *testing.T) {
	notConfiguredState := &configureState{}
	configuredState := &configureState{}
	err := configuredState.Configure(nil, "namespace", testSelector(), "drVolName", "groupName", FilesGroupBackupOptions{})
	require.NoError(t, err)

	tests := []struct {
		desc                string
		configState         *configureState
		isAlreadyValidated  bool
		simulateListErr     bool
		simulateNoPVCsMatch bool
		simulateGetDRErr    bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:               "succeeds if called multiple times",
			isAlreadyValidated: true,
		},
		{
			desc:        "fails because not configured",
			configState: notConfiguredState,
		},
		{
			desc:            "fails to list source PVCs",
			simulateListErr: true,
		},
		{
			desc:                "fails because the selector matches no PVCs",
			simulateNoPVCsMatch: true,
		},
		{
			desc:             "fails to get DR PVC",
			simulateGetDRErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockCoreClient := core.NewMockClientInterface(t)
			mockClient := kubecluster.NewMockClientInterface(t)
			mockClient.EXPECT().Core().Return(mockCoreClient).Maybe()

			if tt.configState == nil {
				tt.configState = configuredState
			}
			tt.configState.kubeClusterClient = mockClient

			currentState := &validateState{
				configureState: *tt.configState,
				isValidated:    tt.isAlreadyValidated,
			}
			ctx := th.NewTestContext()

			func() {
				if !currentState.isConfigured {
					return
				}

				mockCoreClient.EXPECT().ListPVCs(mock.Anything, "namespace", core.ListPVCsOptions{LabelSelector: testSelector()}).
					RunAndReturn(func(calledCtx *contexts.Context, namespace string, opts core.ListPVCsOptions) ([]corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						if tt.simulateListErr {
							return nil, assert.AnError
						}
						if tt.simulateNoPVCsMatch {
							return []corev1.PersistentVolumeClaim{}, nil
						}
						return []corev1.PersistentVolumeClaim{{}, {}}, nil
					})
				if tt.simulateListErr || tt.simulateNoPVCsMatch {
					return
				}

				mockCoreClient.EXPECT().GetPVC(mock.Anything, "namespace", "drVolName").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return nil, th.ErrIfTrue(tt.simulateGetDRErr)
					})
			}()

			err := currentState.Validate(ctx)

			if th.ErrExpected(!currentState.isConfigured, tt.simulateListErr, tt.simulateNoPVCsMatch, tt.simulateGetDRErr) {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, currentState.isValidated)
		})
	}
}

func TestBeforeConsistencyPoint(t *testing.T) {
	statusFreezeTime := time.Date(2026, time.June, 2, 12, 0, 0, 0, time.UTC)
	objectCreateTime := time.Date(2026, time.June, 2, 11, 0, 0, 0, time.UTC)

	cloneResultWith := func(status *volumegroupsnapshotv1.VolumeGroupSnapshotStatus) *clonepvc.ClonePVCGroupResult {
		return &clonepvc.ClonePVCGroupResult{
			GroupSnapshot: &volumegroupsnapshotv1.VolumeGroupSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vgs", CreationTimestamp: metav1.NewTime(objectCreateTime)},
				Status:     status,
			},
			ClonedPVCs: map[string]*corev1.PersistentVolumeClaim{
				"pvc-a": {ObjectMeta: metav1.ObjectMeta{Name: "clone-a"}},
			},
		}
	}

	tests := []struct {
		desc             string
		notValidated     bool
		alreadyCloned    bool
		simulateCloneErr bool
		cloneResult      *clonepvc.ClonePVCGroupResult
		expectedInstant  time.Time
	}{
		{
			desc:            "succeeds and pins the group freeze instant",
			cloneResult:     cloneResultWith(&volumegroupsnapshotv1.VolumeGroupSnapshotStatus{CreationTime: ptrTime(statusFreezeTime)}),
			expectedInstant: statusFreezeTime,
		},
		{
			desc:            "falls back to the object creation time when status creation time is unset",
			cloneResult:     cloneResultWith(nil),
			expectedInstant: objectCreateTime,
		},
		{
			desc:         "fails because not validated first",
			notValidated: true,
		},
		{
			desc:          "fails if called multiple times",
			alreadyCloned: true,
		},
		{
			desc:             "fails to clone the source PVC group",
			simulateCloneErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)

			currentState := &cloneState{
				validateState: validateState{
					configureState: configureState{
						uid:               "uid",
						isConfigured:      true,
						kubeClusterClient: mockClient,
						namespace:         "namespace",
						selector:          testSelector(),
						drVolName:         "drVolName",
						groupName:         "groupName",
						opts:              FilesGroupBackupOptions{SnapshotClass: "snap-class", CleanupTimeout: helpers.ShortWaitTime},
					},
					isValidated: !tt.notValidated,
				},
				isCloned: tt.alreadyCloned,
			}

			ctx := th.NewTestContext()

			if !tt.notValidated && !tt.alreadyCloned {
				mockClient.EXPECT().ClonePVCGroup(mock.Anything, currentState.namespace, currentState.selector, clonepvc.ClonePVCGroupOptions{
					SnapshotClass:  currentState.opts.SnapshotClass,
					ForceBind:      true,
					CleanupTimeout: currentState.opts.CleanupTimeout,
				}).RunAndReturn(func(calledCtx *contexts.Context, namespace string, selector metav1.LabelSelector, opts clonepvc.ClonePVCGroupOptions) (*clonepvc.ClonePVCGroupResult, error) {
					assert.True(t, calledCtx.IsChildOf(ctx))
					return th.ErrOr1Val(tt.cloneResult, tt.simulateCloneErr)
				})
			}

			pinnedTime, err := currentState.BeforeConsistencyPoint(ctx)
			if th.ErrExpected(tt.notValidated, tt.alreadyCloned, tt.simulateCloneErr) {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedInstant, pinnedTime)
			assert.Equal(t, tt.cloneResult, currentState.cloneResult)
			assert.True(t, currentState.isCloned)
		})
	}
}

func TestSetup(t *testing.T) {
	tests := []struct {
		desc           string
		notCloned      bool
		isAlreadySetup bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:      "fails because not cloned first",
			notCloned: true,
		},
		{
			desc:           "fails if called multiple times",
			isAlreadySetup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cloneResult := &clonepvc.ClonePVCGroupResult{
				GroupSnapshot: &volumegroupsnapshotv1.VolumeGroupSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "vgs"}},
				ClonedPVCs: map[string]*corev1.PersistentVolumeClaim{
					"pvc-a": {ObjectMeta: metav1.ObjectMeta{Name: "clone-a"}},
					"pvc-b": {ObjectMeta: metav1.ObjectMeta{Name: "clone-b"}},
				},
			}

			currentState := &setupState{
				cloneState: cloneState{
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: kubecluster.NewMockClientInterface(t),
							namespace:         "namespace",
							selector:          testSelector(),
							drVolName:         "drVolName",
							groupName:         "groupName",
							opts:              FilesGroupBackupOptions{},
						},
						isValidated: true,
					},
					cloneResult: cloneResult,
					isCloned:    !tt.notCloned,
				},
				isSetup: tt.isAlreadySetup,
			}

			btiOpts := &backuptoolinstance.CreateBackupToolInstanceOptions{}
			err := currentState.Setup(th.NewTestContext(), btiOpts)
			if tt.notCloned || tt.isAlreadySetup {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Contains(t, currentState.drVolumeMountPath, currentState.uid)
			// DR volume + one per member clone.
			assert.Len(t, btiOpts.Volumes, 1+len(cloneResult.ClonedPVCs))

			volumeByClaim := func(claimName string) (core.SingleContainerVolume, bool) {
				for _, v := range btiOpts.Volumes {
					if v.VolumeSource.PersistentVolumeClaim != nil && v.VolumeSource.PersistentVolumeClaim.ClaimName == claimName {
						return v, true
					}
				}
				return core.SingleContainerVolume{}, false
			}

			drVol, ok := volumeByClaim(currentState.drVolName)
			require.True(t, ok)
			assert.Equal(t, []string{currentState.drVolumeMountPath}, drVol.MountPaths)

			for sourcePVC, clone := range cloneResult.ClonedPVCs {
				memberVol, ok := volumeByClaim(clone.Name)
				require.True(t, ok, "expected a mounted volume for clone %q", clone.Name)
				assert.Equal(t, []string{currentState.memberMountPaths[sourcePVC]}, memberVol.MountPaths)
			}
		})
	}
}

func TestCleanup(t *testing.T) {
	tests := []struct {
		desc              string
		nothingCloned     bool
		simulatePVCDelErr bool
		simulateVGSDelErr bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:          "succeeds and does nothing if nothing was cloned",
			nothingCloned: true,
		},
		{
			desc:              "fails to delete a cloned PVC",
			simulatePVCDelErr: true,
		},
		{
			desc:              "fails to delete the group snapshot",
			simulateVGSDelErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)
			mockESClient := externalsnapshotter.NewMockClientInterface(t)

			currentState := &setupState{
				cloneState: cloneState{
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: mockClient,
							namespace:         "namespace",
							groupName:         "groupName",
							opts:              FilesGroupBackupOptions{CleanupTimeout: helpers.ShortWaitTime},
						},
						isValidated: true,
					},
				},
			}

			if !tt.nothingCloned {
				currentState.cloneResult = &clonepvc.ClonePVCGroupResult{
					GroupSnapshot: &volumegroupsnapshotv1.VolumeGroupSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "vgs", Namespace: "namespace"}},
					ClonedPVCs: map[string]*corev1.PersistentVolumeClaim{
						"pvc-a": {ObjectMeta: metav1.ObjectMeta{Name: "clone-a", Namespace: "namespace"}},
					},
				}

				mockClient.EXPECT().Core().Return(mockCoreClient)
				mockCoreClient.EXPECT().DeletePVC(mock.Anything, currentState.namespace, "clone-a").
					Return(th.ErrIfTrue(tt.simulatePVCDelErr))

				mockClient.EXPECT().ES().Return(mockESClient)
				mockESClient.EXPECT().DeleteGroupSnapshot(mock.Anything, currentState.namespace, "vgs").
					Return(th.ErrIfTrue(tt.simulateVGSDelErr))
			}

			err := currentState.Cleanup(th.NewTestContext())
			if tt.simulatePVCDelErr || tt.simulateVGSDelErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		desc            string
		hasNotBeenSetup bool
		simulateSyncErr bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:            "fails if not setup first",
			hasNotBeenSetup: true,
		},
		{
			desc:            "fails to sync files",
			simulateSyncErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockFilesRuntime := files.NewMockRuntime(t)
			mockGRPC := clients.NewMockClientInterface(t)
			mockGRPC.EXPECT().Files().Return(mockFilesRuntime).Maybe()

			currentState := &executeState{
				setupState: setupState{
					cloneState: cloneState{
						validateState: validateState{
							configureState: configureState{
								uid:               "uid",
								isConfigured:      true,
								kubeClusterClient: kubecluster.NewMockClientInterface(t),
								namespace:         "namespace",
								groupName:         "app",
								opts:              FilesGroupBackupOptions{Filter: files.FileFilter{Exclude: []files.FilePattern{{Glob: "**/*.tmp"}}}},
							},
							isValidated: true,
						},
						isCloned: true,
					},
					drVolumeMountPath: "/dr-volume",
					memberMountPaths: map[string]string{
						"pvc-a": "/members/pvc-a",
						"pvc-b": "/members/pvc-b",
					},
					isSetup: !tt.hasNotBeenSetup,
				},
			}

			ctx := th.NewTestContext()
			if currentState.isSetup {
				for sourcePVC, mountPath := range currentState.memberMountPaths {
					drDataPath := filepath.Join(currentState.drVolumeMountPath, layout.FileGroupsDirName, currentState.groupName, sourcePVC)
					// The group-wide filter must be plumbed through to every member's sync verbatim.
					mockFilesRuntime.EXPECT().SyncFiles(mock.Anything, mountPath, drDataPath, files.SyncFilesOptions{Filter: currentState.opts.Filter}).
						RunAndReturn(func(calledCtx *contexts.Context, src, dest string, _ files.SyncFilesOptions) error {
							assert.True(t, calledCtx.IsChildOf(ctx))
							return th.ErrIfTrue(tt.simulateSyncErr)
						}).Maybe()
				}
			}

			err := currentState.Execute(ctx, mockGRPC)
			if tt.hasNotBeenSetup || tt.simulateSyncErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestFilesGroupBackup(t *testing.T) {
	assert.Implements(t, (*FilesGroupBackupInterface)(nil), (*FilesGroupBackup)(nil))
	assert.Implements(t, (*remote.RemoteAction)(nil), (*FilesGroupBackup)(nil))
	assert.Implements(t, (*remote.PreConsistencyPointAction)(nil), (*FilesGroupBackup)(nil))
	assert.Implements(t, (*remote.CleanupAction)(nil), (*FilesGroupBackup)(nil))
}

func TestNewFilesGroupBackup(t *testing.T) {
	assert.Equal(t, &FilesGroupBackup{}, NewFilesGroupBackup())
}

func ptrTime(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}
