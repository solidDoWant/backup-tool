package disasterrecovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBuildDRCommand(t *testing.T) {
	noEventsCommand := NewMockDRCommand(t)
	noEventsCommand.EXPECT().Name().Return("no-events-command")

	mockEventCommand := NewMockDREventCommand(t)
	mockEventCommand.EXPECT().Run().Return(nil).Maybe()
	mockEventCommand.EXPECT().ConfigureFlags(mock.Anything).Maybe()

	backupEventCommand := NewMockDRBackupCommand(t)
	backupEventCommand.EXPECT().Name().Return("backup-event-command")
	backupEventCommand.EXPECT().GetBackupCommand().Return(mockEventCommand)

	restoreEventCommand := NewMockDRRestoreCommand(t)
	restoreEventCommand.EXPECT().Name().Return("restore-event-command")
	restoreEventCommand.EXPECT().GetRestoreCommand().Return(mockEventCommand)

	tests := []struct {
		desc                 string
		command              DRCommand
		nilCheckFunc         assert.ValueAssertionFunc
		expectedCommandCount int
	}{
		{
			desc:         "no events",
			command:      noEventsCommand,
			nilCheckFunc: assert.Nil,
		},
		{
			desc:                 "backup event",
			command:              backupEventCommand,
			expectedCommandCount: 1,
		},
		{
			desc:                 "restore event",
			command:              restoreEventCommand,
			expectedCommandCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.nilCheckFunc == nil {
				tt.nilCheckFunc = assert.NotNil
			}

			builtCmd := buildDRCommand(tt.command)
			tt.nilCheckFunc(t, builtCmd)

			if builtCmd != nil {
				assert.Len(t, builtCmd.Commands(), tt.expectedCommandCount)
			}
		})
	}
}

func TestGetDRSubcommands(t *testing.T) {
	subcommands := getDRSubcommands()
	assert.Len(t, subcommands, len(drCommands))
	assert.NotContains(t, subcommands, nil)
}

func TestGetDRCommand(t *testing.T) {
	cmd := GetDRCommand()
	assert.NotNil(t, cmd)
	assert.Len(t, cmd.Commands(), len(drCommands))
}
