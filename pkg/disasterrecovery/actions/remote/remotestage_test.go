package remote

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

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
		ClusterServiceSearchDomains: []string{"test.domain"},
		CleanupTimeout:              helpers.MaxWaitTime(5),
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
