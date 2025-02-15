package disasterrecovery

import (
	"context"
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/disasterrecovery"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewVaultWardenCommand(t *testing.T) {
	cmd := NewVaultWardenDRCommand()
	assert.NotNil(t, cmd)
	assert.Implements(t, (*DRCommand)(nil), cmd)
	assert.Implements(t, (*DRBackupCommand)(nil), cmd)
	assert.Implements(t, (*DRRestoreCommand)(nil), cmd)
}

func TestVaultWardenCommandName(t *testing.T) {
	assert.Equal(t, "vaultwarden", NewVaultWardenDRCommand().Name())
}

func TestVaultWardenCommandGetBackupCommand(t *testing.T) {
	assert.NotNil(t, NewVaultWardenDRCommand().GetBackupCommand())
}

func TestVaultWardenCommandGetRestoreCommand(t *testing.T) {
	assert.NotNil(t, NewVaultWardenDRCommand().GetRestoreCommand())
}

func TestNewVaultWardenDREventCommand(t *testing.T) {
	// Type doesn't matter here
	cmd := NewVaultWardenDREventCommand[interface{}]()
	assert.NotNil(t, cmd)
	assert.NotNil(t, cmd.context)
}

func TestVaultWardenDREventCommandSetup(t *testing.T) {
	expectedCtx := contexts.NewContext(context.Background())
	expectedCancelFunc := func() {}
	expectedConfig := "dummy config instance" // Type and value are not critical
	expectedClusterClient := kubecluster.NewMockClientInterface(t)

	mockContextCommand := features.NewMockContextCommandInterface(t)
	mockContextCommand.EXPECT().GetCommandContext().Return(expectedCtx, expectedCancelFunc)

	mockConfigFileCommand := features.NewMockConfigFileCommandInterface[string](t)
	mockConfigFileCommand.EXPECT().ReadConfigFile(expectedCtx).Return(expectedConfig, nil)

	mockKubeClusterCommand := features.NewMockKubeClusterCommandInterface(t)
	mockKubeClusterCommand.EXPECT().NewKubeClusterClient().Return(expectedClusterClient, nil)

	cmd := &VaultWardenDREventCommand[string]{
		context:     mockContextCommand,
		configFile:  mockConfigFileCommand,
		kubeCluster: mockKubeClusterCommand,
	}

	ctx, cancel, config, vw, err := cmd.setup()

	assert.Same(t, expectedCtx, ctx)
	assert.NotNil(t, cancel)
	assert.Equal(t, expectedConfig, config)
	assert.Equal(t, disasterrecovery.NewVaultWarden(expectedClusterClient), vw)
	assert.NoError(t, err)
}

func TestVaultWardenDREventCommandConfigureFlags(t *testing.T) {
	cobraCmd := &cobra.Command{}

	mockContextCommand := features.NewMockContextCommandInterface(t)
	mockContextCommand.EXPECT().ConfigureFlags(cobraCmd)

	mockConfigFileCommand := features.NewMockConfigFileCommandInterface[string](t)
	mockConfigFileCommand.EXPECT().ConfigureFlags(cobraCmd)

	mockKubeClusterCommand := features.NewMockKubeClusterCommandInterface(t)
	mockKubeClusterCommand.EXPECT().ConfigureFlags(cobraCmd)

	cmd := &VaultWardenDREventCommand[string]{
		context:     mockContextCommand,
		configFile:  mockConfigFileCommand,
		kubeCluster: mockKubeClusterCommand,
	}

	cmd.ConfigureFlags(cobraCmd)
}

func TestVaultWardenDREventCommandGenerateConfigSchema(t *testing.T) {
	schema := []byte("schema")
	mockConfigFileCommand := features.NewMockConfigFileCommandInterface[string](t)
	mockConfigFileCommand.EXPECT().GenerateConfigSchema().Return(schema, nil)

	cmd := &VaultWardenDREventCommand[string]{
		configFile: mockConfigFileCommand,
	}

	generatedSchema, err := cmd.GenerateConfigSchema()
	assert.NoError(t, err)
	assert.Equal(t, schema, generatedSchema)
}

func TestVaultWardenBackupCommand(t *testing.T) {
	assert.Implements(t, (*DREventCommand)(nil), &VaultWardenBackupCommand{})
}

func TestNewVaultWardenBackupCommand(t *testing.T) {
	cmd := NewVaultWardenBackupCommand()
	assert.NotNil(t, cmd)
	assert.NotNil(t, cmd.VaultWardenDREventCommand)
}

func TestVaultWardenRestoreCommand(t *testing.T) {
	assert.Implements(t, (*DREventCommand)(nil), &VaultWardenRestoreCommand{})
}

func TestNewVaultWardenRestoreCommand(t *testing.T) {
	cmd := NewVaultWardenRestoreCommand()
	assert.NotNil(t, cmd)
	assert.NotNil(t, cmd.VaultWardenDREventCommand)
}
