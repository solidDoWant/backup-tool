// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        v5.29.3
// source: s3_transfer.proto

package s3_v1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type SyncRequest struct {
	state                  protoimpl.MessageState `protogen:"opaque.v1"`
	xxx_hidden_Credentials *Credentials           `protobuf:"bytes,1,opt,name=credentials"`
	xxx_hidden_Source      *string                `protobuf:"bytes,2,opt,name=source"`
	xxx_hidden_Dest        *string                `protobuf:"bytes,3,opt,name=dest"`
	XXX_raceDetectHookData protoimpl.RaceDetectHookData
	XXX_presence           [1]uint32
	unknownFields          protoimpl.UnknownFields
	sizeCache              protoimpl.SizeCache
}

func (x *SyncRequest) Reset() {
	*x = SyncRequest{}
	mi := &file_s3_transfer_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SyncRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SyncRequest) ProtoMessage() {}

func (x *SyncRequest) ProtoReflect() protoreflect.Message {
	mi := &file_s3_transfer_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *SyncRequest) GetCredentials() *Credentials {
	if x != nil {
		return x.xxx_hidden_Credentials
	}
	return nil
}

func (x *SyncRequest) GetSource() string {
	if x != nil {
		if x.xxx_hidden_Source != nil {
			return *x.xxx_hidden_Source
		}
		return ""
	}
	return ""
}

func (x *SyncRequest) GetDest() string {
	if x != nil {
		if x.xxx_hidden_Dest != nil {
			return *x.xxx_hidden_Dest
		}
		return ""
	}
	return ""
}

func (x *SyncRequest) SetCredentials(v *Credentials) {
	x.xxx_hidden_Credentials = v
}

func (x *SyncRequest) SetSource(v string) {
	x.xxx_hidden_Source = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 1, 3)
}

func (x *SyncRequest) SetDest(v string) {
	x.xxx_hidden_Dest = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 2, 3)
}

func (x *SyncRequest) HasCredentials() bool {
	if x == nil {
		return false
	}
	return x.xxx_hidden_Credentials != nil
}

func (x *SyncRequest) HasSource() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 1)
}

func (x *SyncRequest) HasDest() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 2)
}

func (x *SyncRequest) ClearCredentials() {
	x.xxx_hidden_Credentials = nil
}

func (x *SyncRequest) ClearSource() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 1)
	x.xxx_hidden_Source = nil
}

func (x *SyncRequest) ClearDest() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 2)
	x.xxx_hidden_Dest = nil
}

type SyncRequest_builder struct {
	_ [0]func() // Prevents comparability and use of unkeyed literals for the builder.

	Credentials *Credentials
	Source      *string
	Dest        *string
}

func (b0 SyncRequest_builder) Build() *SyncRequest {
	m0 := &SyncRequest{}
	b, x := &b0, m0
	_, _ = b, x
	x.xxx_hidden_Credentials = b.Credentials
	if b.Source != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 1, 3)
		x.xxx_hidden_Source = b.Source
	}
	if b.Dest != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 2, 3)
		x.xxx_hidden_Dest = b.Dest
	}
	return m0
}

type SyncResponse struct {
	state         protoimpl.MessageState `protogen:"opaque.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SyncResponse) Reset() {
	*x = SyncResponse{}
	mi := &file_s3_transfer_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SyncResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SyncResponse) ProtoMessage() {}

func (x *SyncResponse) ProtoReflect() protoreflect.Message {
	mi := &file_s3_transfer_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

type SyncResponse_builder struct {
	_ [0]func() // Prevents comparability and use of unkeyed literals for the builder.

}

func (b0 SyncResponse_builder) Build() *SyncResponse {
	m0 := &SyncResponse{}
	b, x := &b0, m0
	_, _ = b, x
	return m0
}

var File_s3_transfer_proto protoreflect.FileDescriptor

var file_s3_transfer_proto_rawDesc = string([]byte{
	0x0a, 0x11, 0x73, 0x33, 0x5f, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x14, 0x73, 0x33, 0x5f, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69,
	0x61, 0x6c, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x69, 0x0a, 0x0b, 0x53, 0x79, 0x6e,
	0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2e, 0x0a, 0x0b, 0x63, 0x72, 0x65, 0x64,
	0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0c, 0x2e,
	0x43, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x52, 0x0b, 0x63, 0x72, 0x65,
	0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x64, 0x65, 0x73, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x64, 0x65, 0x73, 0x74, 0x22, 0x0e, 0x0a, 0x0c, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x42, 0x4f, 0x5a, 0x4d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x73, 0x6f, 0x6c, 0x69, 0x64, 0x44, 0x6f, 0x57, 0x61, 0x6e, 0x74, 0x2f, 0x62,
	0x61, 0x63, 0x6b, 0x75, 0x70, 0x2d, 0x74, 0x6f, 0x6f, 0x6c, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x67,
	0x72, 0x70, 0x63, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x61,
	0x63, 0x6b, 0x75, 0x70, 0x2d, 0x74, 0x6f, 0x6f, 0x6c, 0x2f, 0x73, 0x33, 0x2f, 0x76, 0x31, 0x3b,
	0x73, 0x33, 0x5f, 0x76, 0x31, 0x62, 0x08, 0x65, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x70,
	0xe8, 0x07,
})

var file_s3_transfer_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_s3_transfer_proto_goTypes = []any{
	(*SyncRequest)(nil),  // 0: SyncRequest
	(*SyncResponse)(nil), // 1: SyncResponse
	(*Credentials)(nil),  // 2: Credentials
}
var file_s3_transfer_proto_depIdxs = []int32{
	2, // 0: SyncRequest.credentials:type_name -> Credentials
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_s3_transfer_proto_init() }
func file_s3_transfer_proto_init() {
	if File_s3_transfer_proto != nil {
		return
	}
	file_s3_credentials_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_s3_transfer_proto_rawDesc), len(file_s3_transfer_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_s3_transfer_proto_goTypes,
		DependencyIndexes: file_s3_transfer_proto_depIdxs,
		MessageInfos:      file_s3_transfer_proto_msgTypes,
	}.Build()
	File_s3_transfer_proto = out.File
	file_s3_transfer_proto_goTypes = nil
	file_s3_transfer_proto_depIdxs = nil
}
