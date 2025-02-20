package disasterrecovery

import (
	"context"
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/cli/features"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterDREventCommand(t *testing.T) {
	// Type does not matter here
	cmd := &ClusterDREventCommand[interface{}]{}

	assert.Implements(t, (*DREventCommand)(nil), cmd)
	assert.Implements(t, (*DREventGenerateSchemaCommand)(nil), cmd)
}

func TestNewClusterDREventCommand(t *testing.T) {
	cmdName := "test-command"
	// Config type does not matter here
	runFunc := func(ctx *contexts.Context, config interface{}, kubeCluster kubecluster.ClientInterface) error {
		return assert.AnError
	}

	cmd := NewClusterDREventCommand(cmdName, runFunc)
	require.NotNil(t, cmd)
	assert.Equal(t, cmdName, cmd.name)
	assert.NotNil(t, cmd.context)
	assert.NotNil(t, cmd.configFile)
	assert.NotNil(t, cmd.kubeCluster)
	require.NotNil(t, cmd.run)
	assert.Error(t, cmd.run(nil, nil, nil))
}

func TestClusterDREventCommandSetup(t *testing.T) {
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

	cmd := NewClusterDREventCommand[string]("test-command", nil)
	cmd.context = mockContextCommand
	cmd.configFile = mockConfigFileCommand
	cmd.kubeCluster = mockKubeClusterCommand

	ctx, cancel, config, kubeCluster, err := cmd.setup()

	assert.Same(t, expectedCtx, ctx)
	assert.NotNil(t, cancel)
	assert.Equal(t, expectedConfig, config)
	assert.Equal(t, expectedClusterClient, kubeCluster)
	assert.NoError(t, err)
}

func TestClusterDREventCommandConfigureFlags(t *testing.T) {
	cobraCmd := &cobra.Command{}

	mockContextCommand := features.NewMockContextCommandInterface(t)
	mockContextCommand.EXPECT().ConfigureFlags(cobraCmd)

	mockConfigFileCommand := features.NewMockConfigFileCommandInterface[string](t)
	mockConfigFileCommand.EXPECT().ConfigureFlags(cobraCmd)

	mockKubeClusterCommand := features.NewMockKubeClusterCommandInterface(t)
	mockKubeClusterCommand.EXPECT().ConfigureFlags(cobraCmd)

	cmd := NewClusterDREventCommand[string]("test-command", nil)
	cmd.context = mockContextCommand
	cmd.configFile = mockConfigFileCommand
	cmd.kubeCluster = mockKubeClusterCommand

	// The EXPECT calls verify that the feature functions are called
	cmd.ConfigureFlags(cobraCmd)
}

func TestClusterDREventCommandGenerateConfigSchema(t *testing.T) {
	schema := []byte("schema")
	mockConfigFileCommand := features.NewMockConfigFileCommandInterface[string](t)
	mockConfigFileCommand.EXPECT().GenerateConfigSchema().Return(schema, nil)

	cmd := NewClusterDREventCommand[string]("test-command", nil)
	cmd.configFile = mockConfigFileCommand

	generatedSchema, err := cmd.GenerateConfigSchema()
	assert.NoError(t, err)
	assert.Equal(t, schema, generatedSchema)
}

func TestClusterDREEventCommandRun(t *testing.T) {
	ctx := contexts.NewContext(context.Background())

	mockContextCommand := features.NewMockContextCommandInterface(t)
	mockContextCommand.EXPECT().GetCommandContext().Return(ctx, func() {})

	mockConfigFileCommand := features.NewMockConfigFileCommandInterface[string](t)
	mockConfigFileCommand.EXPECT().ReadConfigFile(ctx).Return("dummy config instance", nil)

	mockKubeClusterCommand := features.NewMockKubeClusterCommandInterface(t)
	mockKubeClusterCommand.EXPECT().NewKubeClusterClient().Return(kubecluster.NewMockClientInterface(t), nil)

	cmd := NewClusterDREventCommand[string]("test-command", nil)
	cmd.context = mockContextCommand
	cmd.configFile = mockConfigFileCommand
	cmd.kubeCluster = mockKubeClusterCommand

	cmd.run = func(ctx *contexts.Context, config string, kubeCluster kubecluster.ClientInterface) error {
		return assert.AnError
	}

	assert.Error(t, cmd.Run())
}

func TestClusterDRCommand(t *testing.T) {
	// Type does not matter here
	cmd := &ClusterDRCommand[interface{}, interface{}]{}

	assert.Implements(t, (*DRCommand)(nil), cmd)
	assert.Implements(t, (*DRBackupCommand)(nil), cmd)
	assert.Implements(t, (*DRRestoreCommand)(nil), cmd)
}

func TestNewClusterDRCommand(t *testing.T) {
	cmdName := "test-command"
	// Config types do not matter here
	backupRunFunc := func(ctx *contexts.Context, config interface{}, kubeCluster kubecluster.ClientInterface) error {
		return assert.AnError
	}
	restoreRunFunc := func(ctx *contexts.Context, config interface{}, kubeCluster kubecluster.ClientInterface) error {
		return assert.AnError
	}

	cmd := NewClusterDRCommand(cmdName, backupRunFunc, restoreRunFunc)
	require.NotNil(t, cmd)
	assert.Equal(t, cmdName, cmd.name)
	assert.Error(t, cmd.backupCommand(nil, nil, nil))
	assert.Error(t, cmd.restoreCommand(nil, nil, nil))
}

func TestNewClusterDRCommandName(t *testing.T) {
	cmdName := "test-command"
	cmd := NewClusterDRCommand[interface{}, interface{}](cmdName, nil, nil)
	assert.Equal(t, cmdName, cmd.Name())
}

func TestClusterDRCommandGetBackupCommand(t *testing.T) {
	cmdName := "test-command"
	backupRunFunc := func(ctx *contexts.Context, config interface{}, kubeCluster kubecluster.ClientInterface) error {
		return assert.AnError
	}

	cmd := NewClusterDRCommand[interface{}, interface{}](cmdName, backupRunFunc, nil).GetBackupCommand()
	require.NotNil(t, cmd)
	assert.Error(t, cmd.Run())
}

func TestClusterDRCommandGetRestoreCommand(t *testing.T) {
	cmdName := "test-command"
	restoreRunFunc := func(ctx *contexts.Context, config interface{}, kubeCluster kubecluster.ClientInterface) error {
		return nil
	}

	cmd := NewClusterDRCommand[interface{}, interface{}](cmdName, nil, restoreRunFunc).GetRestoreCommand()
	require.NotNil(t, cmd)
	assert.Error(t, cmd.Run())
}
