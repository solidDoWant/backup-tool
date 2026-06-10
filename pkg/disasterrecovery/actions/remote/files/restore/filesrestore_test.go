package restore

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
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
)

func TestFilesRestoreOptions(t *testing.T) {
	th.OptStructTest[FilesRestoreOptions](t)
}

func TestConfigure(t *testing.T) {
	expectedState := &configureState{
		kubeClusterClient: kubecluster.NewMockClientInterface(t),
		namespace:         "namespace",
		targetPVCName:     "targetPVCName",
		drVolName:         "drVolName",
		backupDirRelPath:  "backupDirRelPath",
		opts:              FilesRestoreOptions{},
	}

	fr := NewFilesRestore()
	err := fr.Configure(
		expectedState.kubeClusterClient,
		expectedState.namespace,
		expectedState.targetPVCName,
		expectedState.drVolName,
		expectedState.backupDirRelPath,
		expectedState.opts,
	)

	t.Run("successfully configures the first time", func(t *testing.T) {
		require.NoError(t, err)
	})

	t.Run("all state vars are populated", func(t *testing.T) {
		casted := fr.(*FilesRestore)

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
			expectedState.targetPVCName,
			expectedState.drVolName,
			expectedState.backupDirRelPath,
			expectedState.opts,
		)
		assert.Error(t, err)
	})
}

func TestValidate(t *testing.T) {
	notConfiguredState := &configureState{}
	configuredState := &configureState{}
	err := configuredState.Configure(
		nil,
		"namespace",
		"targetPVCName",
		"drVolName",
		"backupDirRelPath",
		FilesRestoreOptions{},
	)
	require.NoError(t, err)

	tests := []struct {
		desc                 string
		configState          *configureState
		isAlreadyValidated   bool
		simulateGetTargetErr bool
		simulateGetDRErr     bool
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
			desc:                 "fails to get target PVC",
			simulateGetTargetErr: true,
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

				mockCoreClient.EXPECT().GetPVC(mock.Anything, "namespace", "targetPVCName").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return nil, th.ErrIfTrue(tt.simulateGetTargetErr)
					})
				if tt.simulateGetTargetErr {
					return
				}

				mockCoreClient.EXPECT().GetPVC(mock.Anything, "namespace", "drVolName").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return nil, th.ErrIfTrue(tt.simulateGetDRErr)
					})
			}()

			err := currentState.Validate(ctx)

			if th.ErrExpected(!currentState.isConfigured, tt.simulateGetTargetErr, tt.simulateGetDRErr) {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, currentState.isValidated)
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
			currentState := &setupState{
				validateState: validateState{
					configureState: configureState{
						uid:               "uid",
						isConfigured:      true,
						kubeClusterClient: kubecluster.NewMockClientInterface(t),
						namespace:         "namespace",
						targetPVCName:     "targetPVCName",
						drVolName:         "drVolName",
						backupDirRelPath:  "backupDirRelPath",
						opts:              FilesRestoreOptions{},
					},
					isValidated: !tt.notValidated,
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

			assert.Contains(t, currentState.mountPaths.drVolume, currentState.uid)
			assert.Contains(t, currentState.mountPaths.data, currentState.uid)
			assert.Len(t, btiOpts.Volumes, 2)

			// DR vol
			drVols := lo.Filter(btiOpts.Volumes, func(v core.SingleContainerVolume, _ int) bool {
				return strings.HasPrefix(v.Name, currentState.drVolName)
			})
			require.Len(t, drVols, 1)
			assert.Equal(t, []string{currentState.mountPaths.drVolume}, drVols[0].MountPaths)
			require.NotNil(t, drVols[0].VolumeSource.PersistentVolumeClaim)
			assert.Equal(t, currentState.drVolName, drVols[0].VolumeSource.PersistentVolumeClaim.ClaimName)

			// Target data vol
			dataVols := lo.Filter(btiOpts.Volumes, func(v core.SingleContainerVolume, _ int) bool {
				return strings.HasPrefix(v.Name, currentState.targetPVCName)
			})
			require.Len(t, dataVols, 1)
			assert.Equal(t, []string{currentState.mountPaths.data}, dataVols[0].MountPaths)
			require.NotNil(t, dataVols[0].VolumeSource.PersistentVolumeClaim)
			assert.Equal(t, currentState.targetPVCName, dataVols[0].VolumeSource.PersistentVolumeClaim.ClaimName)
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
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: kubecluster.NewMockClientInterface(t),
							namespace:         "namespace",
							targetPVCName:     "targetPVCName",
							drVolName:         "drVolName",
							backupDirRelPath:  "data-vol",
							opts:              FilesRestoreOptions{},
						},
						isValidated: true,
					},
					mountPaths: setupStateMountPaths{
						drVolume: "/dr-volume",
						data:     "/data",
					},
					isSetup: !tt.hasNotBeenSetup,
				},
			}

			ctx := th.NewTestContext()
			if currentState.isSetup {
				drDataPath := filepath.Join(currentState.mountPaths.drVolume, currentState.backupDirRelPath)
				mockFilesRuntime.EXPECT().SyncFiles(mock.Anything, drDataPath, currentState.mountPaths.data, files.SyncFilesOptions{}).
					RunAndReturn(func(calledCtx *contexts.Context, src, dest string, _ files.SyncFilesOptions) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateSyncErr)
					})
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

func TestFilesRestore(t *testing.T) {
	assert.Implements(t, (*FilesRestoreInterface)(nil), (*FilesRestore)(nil))
	assert.Implements(t, (*remote.RemoteAction)(nil), (*FilesRestore)(nil))
}

func TestNewFilesRestore(t *testing.T) {
	assert.Equal(t, &FilesRestore{}, NewFilesRestore())
}
