package s3sync

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery/actions/remote"
	"github.com/solidDoWant/backup-tool/pkg/grpc/clients"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/backuptoolinstance"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/core"
	"github.com/solidDoWant/backup-tool/pkg/s3"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestS3SyncOpts(t *testing.T) {
	th.OptStructTest[S3SyncOptions](t)
}

func TestConfigure(t *testing.T) {
	mockCreds := s3.NewMockCredentialsInterface(t)
	expectedState := &configureState{
		kubeClusterClient: kubecluster.NewMockClientInterface(t),
		namespace:         "namespace",
		drVolName:         "drVolName",
		backupDirRelPath:  "backupPath",
		s3Path:            "s3Path",
		credentials:       mockCreds,
		direction:         DirectionUpload,
		opts:              S3SyncOptions{},
	}

	s3sync := NewS3Sync()
	err := s3sync.Configure(
		expectedState.kubeClusterClient,
		expectedState.namespace,
		expectedState.drVolName,
		expectedState.backupDirRelPath,
		expectedState.s3Path,
		expectedState.credentials,
		expectedState.direction,
		expectedState.opts,
	)

	t.Run("successfully configures the first time", func(t *testing.T) {
		require.NoError(t, err)
	})

	t.Run("all state vars are populated", func(t *testing.T) {
		casted := s3sync.(*S3Sync)

		assert.NotEqual(t, "", casted.uid)
		assert.NotEqual(t, uuid.Nil.String(), casted.uid)
		expectedState.uid = casted.uid

		assert.True(t, casted.isConfigured)
		expectedState.isConfigured = casted.isConfigured

		assert.Equal(t, expectedState, &casted.configureState)
	})

	t.Run("fails to configure because already configured", func(t *testing.T) {
		err = s3sync.Configure(
			expectedState.kubeClusterClient,
			expectedState.namespace,
			expectedState.drVolName,
			expectedState.backupDirRelPath,
			expectedState.s3Path,
			expectedState.credentials,
			expectedState.direction,
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
		"drVolName",
		"backupPath",
		"s3Path",
		s3.NewMockCredentialsInterface(t),
		DirectionDownload,
		S3SyncOptions{},
	)
	require.NoError(t, err)

	tests := []struct {
		desc               string
		configState        *configureState
		isAlreadyValidated bool
		invalidDirection   bool
		simulateGetPVCErr  bool
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
			desc:             "fails with invalid direction",
			invalidDirection: true,
		},
		{
			desc:              "fails to get DR PVC",
			simulateGetPVCErr: true,
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

				if tt.invalidDirection {
					currentState.direction = Direction(999)
					return
				}

				mockCoreClient.EXPECT().GetPVC(mock.Anything, "namespace", "drVolName").
					RunAndReturn(func(calledCtx *contexts.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
						assert.True(t, calledCtx.IsChildOf(ctx))

						return nil, th.ErrIfTrue(tt.simulateGetPVCErr)
					})
			}()

			err := currentState.Validate(ctx)

			if th.ErrExpected(!currentState.isConfigured, tt.invalidDirection, tt.simulateGetPVCErr) {
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
		desc                    string
		hasBeenNotBeenValidated bool
		isAlreadySetup          bool
	}{
		{
			desc: "succeeds",
		},
		{
			desc:                    "fails because not validated first",
			hasBeenNotBeenValidated: true,
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
						drVolName:         "drVolName",
						backupDirRelPath:  "backupPath",
						s3Path:            "s3Path",
						credentials:       s3.NewMockCredentialsInterface(t),
						direction:         DirectionDownload,
						opts:              S3SyncOptions{},
					},
					isValidated: !tt.hasBeenNotBeenValidated,
				},
				isSetup: tt.isAlreadySetup,
			}

			btiOpts := &backuptoolinstance.CreateBackupToolInstanceOptions{}
			err := currentState.Setup(th.NewTestContext(), btiOpts)
			if tt.hasBeenNotBeenValidated || tt.isAlreadySetup {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Contains(t, currentState.mountPaths.drVolume, currentState.uid)
			assert.Len(t, btiOpts.Volumes, 1)

			drVol := btiOpts.Volumes[0]
			assert.True(t, strings.HasPrefix(drVol.Name, currentState.drVolName))
			assert.Equal(t, currentState.mountPaths.drVolume, drVol.MountPath)
			require.NotNil(t, drVol.VolumeSource.PersistentVolumeClaim)
			assert.Equal(t, currentState.drVolName, drVol.VolumeSource.PersistentVolumeClaim.ClaimName)
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		desc              string
		hasNotBeenSetup   bool
		simulateS3SyncErr bool
		direction         Direction
	}{
		{
			desc:      "succeeds download",
			direction: DirectionDownload,
		},
		{
			desc:      "succeeds upload",
			direction: DirectionUpload,
		},
		{
			desc:            "fails if not setup first",
			hasNotBeenSetup: true,
		},
		{
			desc:              "fails to sync",
			simulateS3SyncErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := kubecluster.NewMockClientInterface(t)
			mockS3Runtime := s3.NewMockRuntime(t)
			mockGRPC := clients.NewMockClientInterface(t)
			mockGRPC.EXPECT().S3().Return(mockS3Runtime).Maybe()

			currentState := &executeState{
				setupState: setupState{
					validateState: validateState{
						configureState: configureState{
							uid:               "uid",
							isConfigured:      true,
							kubeClusterClient: mockClient,
							namespace:         "namespace",
							drVolName:         "drVolName",
							backupDirRelPath:  "backupPath",
							s3Path:            "s3Path",
							credentials:       s3.NewMockCredentialsInterface(t),
							direction:         tt.direction,
							opts:              S3SyncOptions{},
						},
						isValidated: true,
					},
					mountPaths: setupStateMountPaths{
						drVolume: "/dr-volume",
					},
					isSetup: !tt.hasNotBeenSetup,
				},
			}

			ctx := th.NewTestContext()

			if currentState.isSetup {
				backupPath := filepath.Join(currentState.mountPaths.drVolume, currentState.backupDirRelPath)
				source := currentState.s3Path
				destination := backupPath
				if currentState.direction == DirectionUpload {
					source, destination = destination, source
				}

				mockS3Runtime.EXPECT().Sync(mock.Anything, currentState.credentials, source, destination).
					RunAndReturn(func(calledCtx *contexts.Context, creds s3.CredentialsInterface, src, dst string) error {
						assert.True(t, calledCtx.IsChildOf(ctx))
						return th.ErrIfTrue(tt.simulateS3SyncErr)
					})
			}

			err := currentState.Execute(ctx, mockGRPC)
			if tt.hasNotBeenSetup || tt.simulateS3SyncErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestS3Sync(t *testing.T) {
	assert.Implements(t, (*S3SyncInterface)(nil), (*S3Sync)(nil))
	assert.Implements(t, (*remote.RemoteAction)(nil), (*S3Sync)(nil))
}

func TestNewS3Sync(t *testing.T) {
	assert.Equal(t, &S3Sync{}, NewS3Sync())
}
