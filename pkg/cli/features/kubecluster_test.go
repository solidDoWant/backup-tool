package features

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKubeClusterCommand(t *testing.T) {
	assert.Implements(t, (*KubeClusterCommandInterface)(nil), &KubeClusterCommand{})
}

func TestNewKubeClusterCommand(t *testing.T) {
	assert.NotNil(t, NewKubeClusterCommand())
}

func TestKubeClusterGetLabels(t *testing.T) {
	tests := []struct {
		desc           string
		enabledKeys    []string
		labelsFilePath string
		errFunc        assert.ErrorAssertionFunc
		expectedLabels map[string]string
	}{
		{
			desc:           "no labels file path",
			expectedLabels: nil,
		},
		{
			desc:           "labels file path does not exist",
			labelsFilePath: "nonexistentfile",
			expectedLabels: nil,
		},
		{
			desc:           "labels file path exists",
			labelsFilePath: "testdata/kubecluster/valid_labels",
			enabledKeys:    []string{"key1", "key2", "key3"},
			expectedLabels: map[string]string{
				"key1": "value 1",
				"key2": "\"value 2\"",
				"key3": "value=3",
			},
		},
		{
			desc:           "labels file path exists but no enabled keys",
			labelsFilePath: "testdata/kubecluster/valid_labels",
			expectedLabels: map[string]string{},
		},
		{
			desc:           "invalid labels file",
			labelsFilePath: "testdata/kubecluster/invalid_labels",
			errFunc:        assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = assert.NoError
			}

			kcc := &KubeClusterCommand{
				labelsFilePath: tt.labelsFilePath,
				enabledLabels:  tt.enabledKeys,
			}

			labels, err := kcc.getLabels()
			tt.errFunc(t, err)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}
