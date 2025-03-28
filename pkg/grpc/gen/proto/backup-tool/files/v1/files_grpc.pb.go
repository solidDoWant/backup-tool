// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v5.29.3
// source: files.proto

package files_v1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Files_CopyFiles_FullMethodName = "/Files/CopyFiles"
	Files_SyncFiles_FullMethodName = "/Files/SyncFiles"
)

// FilesClient is the client API for Files service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FilesClient interface {
	CopyFiles(ctx context.Context, in *CopyFilesRequest, opts ...grpc.CallOption) (*CopyFilesResponse, error)
	SyncFiles(ctx context.Context, in *SyncFilesRequest, opts ...grpc.CallOption) (*SyncFilesResponse, error)
}

type filesClient struct {
	cc grpc.ClientConnInterface
}

func NewFilesClient(cc grpc.ClientConnInterface) FilesClient {
	return &filesClient{cc}
}

func (c *filesClient) CopyFiles(ctx context.Context, in *CopyFilesRequest, opts ...grpc.CallOption) (*CopyFilesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CopyFilesResponse)
	err := c.cc.Invoke(ctx, Files_CopyFiles_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *filesClient) SyncFiles(ctx context.Context, in *SyncFilesRequest, opts ...grpc.CallOption) (*SyncFilesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SyncFilesResponse)
	err := c.cc.Invoke(ctx, Files_SyncFiles_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FilesServer is the server API for Files service.
// All implementations must embed UnimplementedFilesServer
// for forward compatibility.
type FilesServer interface {
	CopyFiles(context.Context, *CopyFilesRequest) (*CopyFilesResponse, error)
	SyncFiles(context.Context, *SyncFilesRequest) (*SyncFilesResponse, error)
	mustEmbedUnimplementedFilesServer()
}

// UnimplementedFilesServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedFilesServer struct{}

func (UnimplementedFilesServer) CopyFiles(context.Context, *CopyFilesRequest) (*CopyFilesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CopyFiles not implemented")
}
func (UnimplementedFilesServer) SyncFiles(context.Context, *SyncFilesRequest) (*SyncFilesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SyncFiles not implemented")
}
func (UnimplementedFilesServer) mustEmbedUnimplementedFilesServer() {}
func (UnimplementedFilesServer) testEmbeddedByValue()               {}

// UnsafeFilesServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FilesServer will
// result in compilation errors.
type UnsafeFilesServer interface {
	mustEmbedUnimplementedFilesServer()
}

func RegisterFilesServer(s grpc.ServiceRegistrar, srv FilesServer) {
	// If the following call pancis, it indicates UnimplementedFilesServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Files_ServiceDesc, srv)
}

func _Files_CopyFiles_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CopyFilesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FilesServer).CopyFiles(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Files_CopyFiles_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FilesServer).CopyFiles(ctx, req.(*CopyFilesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Files_SyncFiles_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SyncFilesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FilesServer).SyncFiles(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Files_SyncFiles_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FilesServer).SyncFiles(ctx, req.(*SyncFilesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Files_ServiceDesc is the grpc.ServiceDesc for Files service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Files_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "Files",
	HandlerType: (*FilesServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CopyFiles",
			Handler:    _Files_CopyFiles_Handler,
		},
		{
			MethodName: "SyncFiles",
			Handler:    _Files_SyncFiles_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "files.proto",
}
