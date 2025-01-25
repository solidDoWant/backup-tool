// Code generated by protoc-gen-go-grpcmock. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpcmock dev
// - protoc                 v5.29.3
// - testify                v1.10.0
// source: files.proto

package files_v1

import (
	context "context"
	mock "github.com/stretchr/testify/mock"
	grpc "google.golang.org/grpc"
)

type MockFilesClient struct {
	mock.Mock
}

func NewMockFilesClient() *MockFilesClient {
	return &MockFilesClient{}
}

func (c *MockFilesClient) CopyFiles(ctx context.Context, in *CopyFilesRequest, opts ...grpc.CallOption) (*CopyFilesResponse, error) {
	opts0 := []interface{}{ctx, in}
	for _, opts1 := range opts {
		opts0 = append(opts0, opts1)
	}
	args := c.Called(opts0...)
	var ret0 *CopyFilesResponse
	if args.Get(0) != nil {
		ret0 = args.Get(0).(*CopyFilesResponse)
	}
	return ret0, args.Error(1)
}

func (c *MockFilesClient) OnCopyFiles(ctx interface{}, in interface{}, opts ...interface{}) *mock.Call {
	return c.On("CopyFiles", append([]interface{}{ctx, in}, opts...)...)
}

func (c *MockFilesClient) SyncFiles(ctx context.Context, in *SyncFilesRequest, opts ...grpc.CallOption) (*SyncFilesResponse, error) {
	opts0 := []interface{}{ctx, in}
	for _, opts1 := range opts {
		opts0 = append(opts0, opts1)
	}
	args := c.Called(opts0...)
	var ret0 *SyncFilesResponse
	if args.Get(0) != nil {
		ret0 = args.Get(0).(*SyncFilesResponse)
	}
	return ret0, args.Error(1)
}

func (c *MockFilesClient) OnSyncFiles(ctx interface{}, in interface{}, opts ...interface{}) *mock.Call {
	return c.On("SyncFiles", append([]interface{}{ctx, in}, opts...)...)
}

type MockFilesServer struct {
	mock.Mock
}

func NewMockFilesServer() *MockFilesServer {
	return &MockFilesServer{}
}

func (s *MockFilesServer) CopyFiles(ctx context.Context, in *CopyFilesRequest) (*CopyFilesResponse, error) {
	args := s.Called(ctx, in)
	var ret0 *CopyFilesResponse
	if args.Get(0) != nil {
		ret0 = args.Get(0).(*CopyFilesResponse)
	}
	return ret0, args.Error(1)
}

func (s *MockFilesServer) OnCopyFiles(ctx interface{}, in interface{}) *mock.Call {
	return s.On("CopyFiles", ctx, in)
}

func (s *MockFilesServer) SyncFiles(ctx context.Context, in *SyncFilesRequest) (*SyncFilesResponse, error) {
	args := s.Called(ctx, in)
	var ret0 *SyncFilesResponse
	if args.Get(0) != nil {
		ret0 = args.Get(0).(*SyncFilesResponse)
	}
	return ret0, args.Error(1)
}

func (s *MockFilesServer) OnSyncFiles(ctx interface{}, in interface{}) *mock.Call {
	return s.On("SyncFiles", ctx, in)
}
