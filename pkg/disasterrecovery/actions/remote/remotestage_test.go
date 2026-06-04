package remote

import (
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fakeProducerConsumer is a test action that implements both the optional PreConsistencyPointAction and
// ConsistencyPointConsumer capabilities (like the CNPG backup action), on top of a mocked RemoteAction.
// pinnedTime is the instant its BeforeConsistencyPoint reports as pinned (zero pins nothing).
type fakeProducerConsumer struct {
	*MockCleanupAction
	pinnedTime       time.Time
	preErr           error
	preCalled        bool
	consistencyPoint time.Time
	setCalled        bool
}

func (f *fakeProducerConsumer) BeforeConsistencyPoint(_ *contexts.Context) (time.Time, error) {
	f.preCalled = true
	return f.pinnedTime, f.preErr
}

func (f *fakeProducerConsumer) SetConsistencyPoint(c time.Time) {
	f.setCalled = true
	f.consistencyPoint = c
}

// fakeConsumer is a test action that only consumes the consistency point (like the S3 sync action).
type fakeConsumer struct {
	*MockCleanupAction
	consistencyPoint time.Time
	setCalled        bool
}

func (f *fakeConsumer) SetConsistencyPoint(c time.Time) {
	f.setCalled = true
	f.consistencyPoint = c
}

func TestNewNamedRemoteAction(t *testing.T) {
	name := "test-action"
	action := NewMockRemoteAction(t)

	namedAction := newNamedRemoteAction(name, action)

	assert.Equal(t, name, namedAction.name)
	assert.Equal(t, action, namedAction.remoteAction)
}

func TestNewRemoteStage(t *testing.T) {
	kubeClient := kubecluster.NewMockClientInterface(t)
	namespace := "test-namespace"
	eventName := "test-event"
	opts := RemoteStageOptions{
		CleanupTimeout: helpers.MaxWaitTime(5),
	}

	stage := NewRemoteStage(kubeClient, namespace, eventName, opts).(*RemoteStage)

	assert.Equal(t, kubeClient, stage.kubeClusterClient)
	assert.Equal(t, namespace, stage.namespace)
	assert.Equal(t, eventName, stage.eventName)
	assert.Equal(t, opts, stage.opts)
	assert.Equal(t, 0, len(stage.actions))
}

func TestRemoteStageWithAction(t *testing.T) {
	stage := NewRemoteStage(kubecluster.NewMockClientInterface(t), "test-namesace", "test-event", RemoteStageOptions{}).(*RemoteStage)

	actionName := "test-action"
	action := NewMockRemoteAction(t)

	// Test adding single action
	returnedStage := stage.WithAction(actionName, action)

	assert.Equal(t, stage, returnedStage)
	assert.Equal(t, 1, len(stage.actions))
	assert.Equal(t, actionName, stage.actions[0].name)
	assert.Equal(t, action, stage.actions[0].remoteAction)

	// Test adding multiple actions
	action2Name := "test-action-2"
	action2 := NewMockRemoteAction(t)

	stage.WithAction(action2Name, action2)

	assert.Equal(t, 2, len(stage.actions))
	assert.Equal(t, action2Name, stage.actions[1].name)
	assert.Equal(t, action2, stage.actions[1].remoteAction)
}

func TestCleanupFunc(t *testing.T) {
	successAction := NewMockCleanupAction(t)
	successAction.EXPECT().Cleanup(mock.Anything).Return(nil)

	failAction := NewMockCleanupAction(t)
	failAction.EXPECT().Cleanup(mock.Anything).Return(assert.AnError)

	noCleanupAction := NewMockRemoteAction(t)

	tests := []struct {
		desc      string
		actions   []namedRemoteAction
		expectErr bool
	}{
		{
			desc: "succeeds with no cleanup actions",
		},
		{
			desc: "succeeds with non-cleanup action",
			actions: []namedRemoteAction{
				newNamedRemoteAction("no-cleanup", noCleanupAction),
			},
		},
		{
			desc: "succeeds with single successful cleanup action",
			actions: []namedRemoteAction{
				newNamedRemoteAction("success", successAction),
			},
		},
		{
			desc: "succeeds with multiple successful cleanup actions",
			actions: []namedRemoteAction{
				newNamedRemoteAction("success-1", successAction),
				newNamedRemoteAction("success-2", successAction),
			},
		},
		{
			desc: "fails with single failed cleanup action",
			actions: []namedRemoteAction{
				newNamedRemoteAction("fail", failAction),
			},
			expectErr: true,
		},
		{
			desc: "fails with multiple cleanup actions, one failed",
			actions: []namedRemoteAction{
				newNamedRemoteAction("success", successAction),
				newNamedRemoteAction("fail", failAction),
			},
			expectErr: true,
		},
		{
			desc: "fails with multiple cleanup actions, all failed",
			actions: []namedRemoteAction{
				newNamedRemoteAction("fail-1", failAction),
				newNamedRemoteAction("fail-2", failAction),
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			stage := &RemoteStage{
				actions: tt.actions,
			}

			var outerErr error
			cleanupFunc := stage.cleanupFunc(th.NewTestContext(), &outerErr)
			cleanupFunc()

			if tt.expectErr {
				assert.Error(t, outerErr)
				return
			}
			assert.NoError(t, outerErr)
		})
	}
}

func TestSetup(t *testing.T) {
	t.Run("falls back to the current time when no pre-step pins an instant, then distributes and sets up every action", func(t *testing.T) {
		ctx := th.NewTestContext()

		// Two pre-consistency-point actions (e.g. CNPG base backups) that pin no instant of their own.
		coreMock := NewMockCleanupAction(t)
		coreMock.EXPECT().Setup(mock.Anything, mock.Anything).Return(nil)
		core := &fakeProducerConsumer{MockCleanupAction: coreMock}

		auditMock := NewMockCleanupAction(t)
		auditMock.EXPECT().Setup(mock.Anything, mock.Anything).Return(nil)
		audit := &fakeProducerConsumer{MockCleanupAction: auditMock}

		// A consumer-only action (e.g. S3) that receives the consistency point but has no pre-step.
		s3Mock := NewMockCleanupAction(t)
		s3Mock.EXPECT().Setup(mock.Anything, mock.Anything).Return(nil)
		s3 := &fakeConsumer{MockCleanupAction: s3Mock}

		stage := &RemoteStage{
			eventName: "test-event",
			actions: []namedRemoteAction{
				newNamedRemoteAction("core", core),
				newNamedRemoteAction("audit", audit),
				newNamedRemoteAction("s3", s3),
			},
		}

		before := time.Now()
		_, err := stage.setup(ctx)
		require.NoError(t, err)
		after := time.Now()

		assert.True(t, core.preCalled)
		assert.True(t, audit.preCalled)

		// The consistency point is stamped once and handed identically to every consumer; with nothing
		// pinned it is the instant after the pre-steps ran.
		assert.True(t, s3.setCalled)
		c := s3.consistencyPoint
		assert.Equal(t, c, core.consistencyPoint)
		assert.Equal(t, c, audit.consistencyPoint)
		assert.False(t, c.Before(before))
		assert.False(t, c.After(after))
	})

	t.Run("pins the consistency point to the earliest instant any pre-step pinned", func(t *testing.T) {
		ctx := th.NewTestContext()

		earliest := time.Date(2026, time.June, 2, 12, 0, 0, 0, time.UTC)
		later := earliest.Add(time.Hour)

		// A pre-step that pins no instant (e.g. a base backup), and two that pin instants (e.g. filesystem
		// freezes) — the earlier of the two must win, regardless of registration order.
		baseMock := NewMockCleanupAction(t)
		baseMock.EXPECT().Setup(mock.Anything, mock.Anything).Return(nil)
		base := &fakeProducerConsumer{MockCleanupAction: baseMock}

		laterMock := NewMockCleanupAction(t)
		laterMock.EXPECT().Setup(mock.Anything, mock.Anything).Return(nil)
		laterAction := &fakeProducerConsumer{MockCleanupAction: laterMock, pinnedTime: later}

		earliestMock := NewMockCleanupAction(t)
		earliestMock.EXPECT().Setup(mock.Anything, mock.Anything).Return(nil)
		earliestAction := &fakeProducerConsumer{MockCleanupAction: earliestMock, pinnedTime: earliest}

		stage := &RemoteStage{
			eventName: "test-event",
			actions: []namedRemoteAction{
				newNamedRemoteAction("base", base),
				newNamedRemoteAction("later", laterAction),
				newNamedRemoteAction("earliest", earliestAction),
			},
		}

		_, err := stage.setup(ctx)
		require.NoError(t, err)

		assert.Equal(t, earliest, base.consistencyPoint)
		assert.Equal(t, earliest, laterAction.consistencyPoint)
		assert.Equal(t, earliest, earliestAction.consistencyPoint)
	})

	t.Run("returns an error and skips setup when a pre-step fails", func(t *testing.T) {
		ctx := th.NewTestContext()

		// No Setup expectation: phase 3 must not run once a pre-step fails.
		failing := &fakeProducerConsumer{MockCleanupAction: NewMockCleanupAction(t), preErr: assert.AnError}

		stage := &RemoteStage{
			eventName: "test-event",
			actions:   []namedRemoteAction{newNamedRemoteAction("core", failing)},
		}

		_, err := stage.setup(ctx)
		assert.Error(t, err)
		assert.True(t, failing.preCalled)
		assert.False(t, failing.setCalled)
	})
}

func TestValidate(t *testing.T) {
	successAction := NewMockRemoteAction(t)
	successAction.EXPECT().Validate(mock.Anything).Return(nil)

	failAction := NewMockRemoteAction(t)
	failAction.EXPECT().Validate(mock.Anything).Return(assert.AnError)

	tests := []struct {
		desc      string
		actions   []namedRemoteAction
		expectErr bool
	}{
		{
			desc: "succeeds with no actions",
		},
		{
			desc: "succeeds with single successful action",
			actions: []namedRemoteAction{
				newNamedRemoteAction("success", successAction),
			},
		},
		{
			desc: "succeeds with multiple successful actions",
			actions: []namedRemoteAction{
				newNamedRemoteAction("success-1", successAction),
				newNamedRemoteAction("success-2", successAction),
			},
		},
		{
			desc: "fails with single failed action",
			actions: []namedRemoteAction{
				newNamedRemoteAction("fail", failAction),
			},
			expectErr: true,
		},
		{
			desc: "fails on first error with multiple actions",
			actions: []namedRemoteAction{
				newNamedRemoteAction("fail", failAction),
				newNamedRemoteAction("success", successAction),
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			stage := &RemoteStage{
				actions: tt.actions,
			}

			err := stage.validate(th.NewTestContext())

			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
