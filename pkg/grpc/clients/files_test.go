package clients

import (
	"testing"

	"github.com/solidDoWant/backup-tool/pkg/files"
	files_v1 "github.com/solidDoWant/backup-tool/pkg/grpc/gen/proto/backup-tool/files/v1"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

func TestNewFilesClient(t *testing.T) {
	// Create mock gRPC connection
	mockConn := &grpc.ClientConn{}

	// Call NewFilesClient with mock connection
	client := NewFilesClient(mockConn)

	// Assert client was created and is not nil
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Implements(t, (*files.Runtime)(nil), client)
}

func FilesTransferTest(t *testing.T, call func(*FilesClient) error, funcName string, request, response interface{}) {
	tests := []struct {
		desc         string
		returnValues []interface{}
		errFunc      assert.ErrorAssertionFunc
	}{
		{
			desc:         "successful",
			returnValues: []interface{}{response, nil},
			errFunc:      assert.NoError,
		},
		{
			desc:         "failure",
			returnValues: []interface{}{nil, assert.AnError},
			errFunc:      assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mockClient := files_v1.NewMockFilesClient()
			mockClient.On(funcName, mock.Anything, request, mock.Anything).Return(tt.returnValues...)

			fc := &FilesClient{client: mockClient}

			err := call(fc)
			tt.errFunc(t, err)

			mockClient.AssertExpectations(t)
		})
	}
}

func TestFilesClient_CopyFiles(t *testing.T) {
	src := "src"
	dest := "dest"
	FilesTransferTest(t,
		func(fc *FilesClient) error {
			ctx := th.NewTestContext()
			return fc.CopyFiles(ctx, src, dest)
		},
		"CopyFiles",
		files_v1.CopyFilesRequest_builder{Source: &src, Dest: &dest}.Build(),
		&files_v1.CopyFilesResponse{},
	)
}

func TestFilesClient_SyncFiles(t *testing.T) {
	src := "src"
	dest := "dest"
	FilesTransferTest(t,
		func(fc *FilesClient) error {
			ctx := th.NewTestContext()
			return fc.SyncFiles(ctx, src, dest, files.SyncFilesOptions{})
		},
		"SyncFiles",
		files_v1.SyncFilesRequest_builder{Source: &src, Dest: &dest}.Build(),
		&files_v1.SyncFilesResponse{},
	)
}

func TestFilesClient_ListDirectory(t *testing.T) {
	path := "path"
	request := files_v1.ListDirectoryRequest_builder{Path: &path}.Build()

	t.Run("successful", func(t *testing.T) {
		entries := []string{"a", "b"}
		response := files_v1.ListDirectoryResponse_builder{Entries: entries}.Build()

		mockClient := files_v1.NewMockFilesClient()
		mockClient.On("ListDirectory", mock.Anything, request, mock.Anything).Return(response, nil)

		fc := &FilesClient{client: mockClient}
		got, err := fc.ListDirectory(th.NewTestContext(), path)
		assert.NoError(t, err)
		assert.Equal(t, entries, got)
		mockClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := files_v1.NewMockFilesClient()
		mockClient.On("ListDirectory", mock.Anything, request, mock.Anything).Return(nil, assert.AnError)

		fc := &FilesClient{client: mockClient}
		got, err := fc.ListDirectory(th.NewTestContext(), path)
		assert.Error(t, err)
		assert.Nil(t, got)
		mockClient.AssertExpectations(t)
	})
}
