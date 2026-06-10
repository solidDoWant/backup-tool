package backup

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/files"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonepvc"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFilesBackupOptions(t *testing.T) {
	th.OptStructTest[FilesBackupOptions](t)
}

func TestConfigure(t *testing.T) {
	expectedState := &configureState{
		kubeClusterClient: kubecluster.NewMockClientInterface(t),
		namespace:         "namespace",
		sourcePVCName:     "sourcePVCName",
		drVolName:         "drVolName",
		backupDirRelPath:  "backupDirRelPath",
		opts: FilesBackupOptions{
			CleanupTimeout: helpers.ShortWaitTime,
		},
	}

	fb := NewFilesBackup()
	err := fb.Configure(
		expectedState.kubeClusterClient,
		expectedState.namespace,
		expectedState.sourcePVCName,
		expectedState.drVolName,
		expectedState.backupDirRelPath,
		expectedState.opts,
	)

	t.Run("successfully configures the first time", func(t *testing.T) {
		require.NoError(t, err)
	})

	t.Run("all state vars are populated", func(t *testing.T) {
		casted := fb.(*FilesBackup)

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
			expectedState.sourcePVCName,
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
		"sourcePVCName",
		"drVolName",
		"backupDirRelPath",
		FilesBackupOptions{},
	)
	require.NoError(t, err)

	tests := []struct {
		desc               string
		configState        *configureState
		isAlreadyValidated bool
		simulateGetSrcErr  bool
		simulateGetDRErr   bool
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
			desc:              "fails to get source PVC",
			simulateGetSrcErr: true,
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

				mockCoreClient.EXPECT().GetPVC(mock.Anything, "namespace", "sourcePVCName").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return nil, th.ErrIfTrue(tt.simulateGetSrcErr)
					})
				if tt.simulateGetSrcErr {
					return
				}

				mockCoreClient.EXPECT().GetPVC(mock.Anything, "namespace", "drVolName").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return nil, th.ErrIfTrue(tt.simulateGetDRErr)
					})
			}()

			err := currentState.Validate(ctx)

			if th.ErrExpected(!currentState.isConfigured, tt.simulateGetSrcErr, tt.simulateGetDRErr) {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, currentState.isValidated)
		})
	}
}

func TestBeforeConsistencyPoint(t *testing.T) {
	cloneTime := time.Date(2026, time.June, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		desc             string
		notValidated     bool
		alreadyCloned    bool
		simulateCloneErr bool
	}{
		{
			desc: "succeeds and pins the clone time",
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
			desc:             "fails to clone the source PVC",
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
						sourcePVCName:     "sourcePVCName",
						drVolName:         "drVolName",
						backupDirRelPath:  "backupDirRelPath",
						opts:              FilesBackupOptions{SnapshotClass: "snap-class", CleanupTimeout: helpers.ShortWaitTime},
					},
					isValidated: !tt.notValidated,
				},
				isCloned: tt.alreadyCloned,
			}

			clonedPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cloned-pvc",
					CreationTimestamp: metav1.NewTime(cloneTime),
				},
			}

			ctx := th.NewTestContext()

			if !tt.notValidated && !tt.alreadyCloned {
				mockClient.EXPECT().ClonePVC(mock.Anything, currentState.namespace, currentState.sourcePVCName, clonepvc.ClonePVCOptions{
					SnapshotClass:     currentState.opts.SnapshotClass,
					DestPvcNamePrefix: currentState.drVolName,
					ForceBind:         true,
					CleanupTimeout:    currentState.opts.CleanupTimeout,
				}).RunAndReturn(func(calledCtx *contexts.Context, namespace, pvcName string, opts clonepvc.ClonePVCOptions) (*corev1.PersistentVolumeClaim, error) {
					assert.True(t, calledCtx.IsChildOf(ctx))
					return th.ErrOr1Val(clonedPVC, tt.simulateCloneErr)
				})
			}

			pinnedTime, err := currentState.BeforeConsistencyPoint(ctx)
			if th.ErrExpected(tt.notValidated, tt.alreadyCloned, tt.simulateCloneErr) {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, cloneTime, pinnedTime)
			assert.Equal(t, clonedPVC, currentState.clonedPVC)
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
			clonedPVC := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cloned-pvc"}}

			currentState := &setupState{
				cloneState: cloneState{
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: kubecluster.NewMockClientInterface(t),
							namespace:         "namespace",
							sourcePVCName:     "sourcePVCName",
							drVolName:         "drVolName",
							backupDirRelPath:  "backupDirRelPath",
							opts:              FilesBackupOptions{},
						},
						isValidated: true,
					},
					clonedPVC: clonedPVC,
					isCloned:  !tt.notCloned,
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

			// Cloned data vol
			dataVols := lo.Filter(btiOpts.Volumes, func(v core.SingleContainerVolume, _ int) bool {
				return strings.HasPrefix(v.Name, clonedPVC.Name)
			})
			require.Len(t, dataVols, 1)
			assert.Equal(t, []string{currentState.mountPaths.data}, dataVols[0].MountPaths)
			require.NotNil(t, dataVols[0].VolumeSource.PersistentVolumeClaim)
			assert.Equal(t, clonedPVC.Name, dataVols[0].VolumeSource.PersistentVolumeClaim.ClaimName)
		})
	}
}

func TestCleanup(t *testing.T) {
	tests := []struct {
		desc           string
		nothingCloned  bool
		simulateDelErr bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:          "succeeds and does nothing if nothing was cloned",
			nothingCloned: true,
		},
		{
			desc:           "fails to delete the cloned PVC",
			simulateDelErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockCoreClient := core.NewMockClientInterface(t)

			currentState := &setupState{
				cloneState: cloneState{
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: mockClient,
							namespace:         "namespace",
							sourcePVCName:     "sourcePVCName",
							drVolName:         "drVolName",
							opts:              FilesBackupOptions{CleanupTimeout: helpers.ShortWaitTime},
						},
						isValidated: true,
					},
				},
			}

			if !tt.nothingCloned {
				currentState.clonedPVC = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cloned-pvc", Namespace: "namespace"}}
				mockClient.EXPECT().Core().Return(mockCoreClient)
				mockCoreClient.EXPECT().DeletePVC(mock.Anything, currentState.namespace, currentState.clonedPVC.Name).
					Return(th.ErrIfTrue(tt.simulateDelErr))
			}

			err := currentState.Cleanup(th.NewTestContext())
			if tt.simulateDelErr {
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
								sourcePVCName:     "sourcePVCName",
								drVolName:         "drVolName",
								backupDirRelPath:  "data-vol",
								opts:              FilesBackupOptions{Filter: files.FileFilter{Include: []files.FilePattern{{Glob: "*.db"}}, Exclude: []files.FilePattern{{Glob: "*.tmp"}}}},
							},
							isValidated: true,
						},
						isCloned: true,
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
				// The configured filter must be plumbed through to the sync verbatim; matching on the exact
				// SyncFilesOptions here asserts the backup direction whitelists/blacklists files.
				mockFilesRuntime.EXPECT().SyncFiles(mock.Anything, currentState.mountPaths.data, drDataPath, files.SyncFilesOptions{Filter: currentState.opts.Filter}).
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

func TestFilesBackup(t *testing.T) {
	assert.Implements(t, (*FilesBackupInterface)(nil), (*FilesBackup)(nil))
	assert.Implements(t, (*remote.RemoteAction)(nil), (*FilesBackup)(nil))
	assert.Implements(t, (*remote.PreConsistencyPointAction)(nil), (*FilesBackup)(nil))
	assert.Implements(t, (*remote.CleanupAction)(nil), (*FilesBackup)(nil))
}

func TestNewFilesBackup(t *testing.T) {
	assert.Equal(t, &FilesBackup{}, NewFilesBackup())
}
