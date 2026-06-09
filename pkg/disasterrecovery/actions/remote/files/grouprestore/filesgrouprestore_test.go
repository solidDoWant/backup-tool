package grouprestore

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote/files/layout"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
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

func TestFilesGroupRestoreOptions(t *testing.T) {
	th.OptStructTest[FilesGroupRestoreOptions](t)
}

func TestConfigure(t *testing.T) {
	expectedState := &configureState{
		kubeClusterClient: kubecluster.NewMockClientInterface(t),
		namespace:         "namespace",
		selector:          testSelector(),
		drVolName:         "drVolName",
		groupName:         "groupName",
		opts:              FilesGroupRestoreOptions{},
	}

	fr := NewFilesGroupRestore()
	err := fr.Configure(
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
		casted := fr.(*FilesGroupRestore)

		assert.NotEqual(t, "", casted.uid)
		assert.NotEqual(t, uuid.Nil.String(), casted.uid)
		expectedState.uid = casted.uid

		assert.True(t, casted.isConfigured)
		expectedState.isConfigured = casted.isConfigured

		assert.Equal(t, expectedState, &casted.configureState)
	})

	t.Run("fails to configure because already configured", func(t *testing.T) {
		err = fr.Configure(
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
	err := configuredState.Configure(nil, "namespace", testSelector(), "drVolName", "groupName", FilesGroupRestoreOptions{})
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
			desc:            "fails to list target PVCs",
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
						return []corev1.PersistentVolumeClaim{
							{ObjectMeta: metav1.ObjectMeta{Name: "pvc-a"}},
							{ObjectMeta: metav1.ObjectMeta{Name: "pvc-b"}},
						}, nil
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
			assert.ElementsMatch(t, []string{"pvc-a", "pvc-b"}, currentState.targetPVCNames)
		})
	}
}

func TestSetup(t *testing.T) {
	tests := []struct {
		desc           string
		notValidated   bool
		isAlreadySetup bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:         "fails because not validated first",
			notValidated: true,
		},
		{
			desc:           "fails if called multiple times",
			isAlreadySetup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			targetPVCNames := []string{"pvc-a", "pvc-b"}

			currentState := &setupState{
				validateState: validateState{
					configureState: configureState{
						uid:               "uid",
						isConfigured:      true,
						kubeClusterClient: kubecluster.NewMockClientInterface(t),
						namespace:         "namespace",
						selector:          testSelector(),
						drVolName:         "drVolName",
						groupName:         "groupName",
						opts:              FilesGroupRestoreOptions{},
					},
					targetPVCNames: targetPVCNames,
					isValidated:    !tt.notValidated,
				},
				isSetup: tt.isAlreadySetup,
			}

			btiOpts := &backuptoolinstance.CreateBackupToolInstanceOptions{}
			err := currentState.Setup(th.NewTestContext(), btiOpts)
			if tt.notValidated || tt.isAlreadySetup {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Contains(t, currentState.drVolumeMountPath, currentState.uid)
			// DR volume + one per target PVC.
			assert.Len(t, btiOpts.Volumes, 1+len(targetPVCNames))

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

			for _, targetPVCName := range targetPVCNames {
				targetVol, ok := volumeByClaim(targetPVCName)
				require.True(t, ok, "expected a mounted volume for target %q", targetPVCName)
				assert.Equal(t, []string{currentState.targetMountPaths[targetPVCName]}, targetVol.MountPaths)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	groupName := "app"
	drVolumeMountPath := "/dr-volume"
	groupDirPath := filepath.Join(drVolumeMountPath, layout.FileGroupsDirName, groupName)

	tests := []struct {
		desc             string
		hasNotBeenSetup  bool
		targetMountPaths map[string]string
		capturedMembers  []string
		simulateListErr  bool
		simulateSyncErr  bool
		expectMismatch   bool
	}{
		{
			desc:             "succeeds with an exact 1:1 mapping",
			targetMountPaths: map[string]string{"pvc-a": "/targets/pvc-a", "pvc-b": "/targets/pvc-b"},
			capturedMembers:  []string{"pvc-a", "pvc-b"},
		},
		{
			desc:            "fails if not setup first",
			hasNotBeenSetup: true,
		},
		{
			desc:             "fails to list captured members",
			targetMountPaths: map[string]string{"pvc-a": "/targets/pvc-a"},
			simulateListErr:  true,
		},
		{
			desc:             "fails when a target PVC has no captured data",
			targetMountPaths: map[string]string{"pvc-a": "/targets/pvc-a", "pvc-b": "/targets/pvc-b"},
			capturedMembers:  []string{"pvc-a"},
			expectMismatch:   true,
		},
		{
			desc:             "fails when a captured member has no target PVC",
			targetMountPaths: map[string]string{"pvc-a": "/targets/pvc-a"},
			capturedMembers:  []string{"pvc-a", "pvc-b"},
			expectMismatch:   true,
		},
		{
			desc:             "fails to sync a member",
			targetMountPaths: map[string]string{"pvc-a": "/targets/pvc-a", "pvc-b": "/targets/pvc-b"},
			capturedMembers:  []string{"pvc-a", "pvc-b"},
			simulateSyncErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockFilesRuntime := files.NewMockRuntime(t)
			mockGRPC := clients.NewMockClientInterface(t)
			mockGRPC.EXPECT().Files().Return(mockFilesRuntime).Maybe()

			currentState := &executeState{
				setupState: setupState{
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: kubecluster.NewMockClientInterface(t),
							namespace:         "namespace",
							groupName:         groupName,
							opts:              FilesGroupRestoreOptions{},
						},
						isValidated: true,
					},
					drVolumeMountPath: drVolumeMountPath,
					targetMountPaths:  tt.targetMountPaths,
					isSetup:           !tt.hasNotBeenSetup,
				},
			}

			ctx := th.NewTestContext()

			if currentState.isSetup {
				mockFilesRuntime.EXPECT().ListDirectory(mock.Anything, groupDirPath).
					RunAndReturn(func(calledCtx *contexts.Context, path string) ([]string, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						if tt.simulateListErr {
							return nil, assert.AnError
						}
						return tt.capturedMembers, nil
					})

				// Sync is only reached once the 1:1 check passes (no list error, no mismatch).
				if !tt.simulateListErr && !tt.expectMismatch {
					for targetPVCName, mountPath := range tt.targetMountPaths {
						srcPath := filepath.Join(groupDirPath, targetPVCName)
						mockFilesRuntime.EXPECT().SyncFiles(mock.Anything, srcPath, mountPath).
							RunAndReturn(func(calledCtx *contexts.Context, src, dest string) error {
								assert.True(t, calledCtx.IsChildOf(ctx))
								return th.ErrIfTrue(tt.simulateSyncErr)
							}).Maybe()
					}
				}
			}

			err := currentState.Execute(ctx, mockGRPC)
			if tt.hasNotBeenSetup || tt.simulateListErr || tt.expectMismatch || tt.simulateSyncErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestFilesGroupRestore(t *testing.T) {
	assert.Implements(t, (*FilesGroupRestoreInterface)(nil), (*FilesGroupRestore)(nil))
	assert.Implements(t, (*remote.RemoteAction)(nil), (*FilesGroupRestore)(nil))
}

func TestNewFilesGroupRestore(t *testing.T) {
	assert.Equal(t, &FilesGroupRestore{}, NewFilesGroupRestore())
}
