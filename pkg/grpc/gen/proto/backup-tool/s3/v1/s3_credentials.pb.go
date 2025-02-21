// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        v5.29.3
// source: s3_credentials.proto

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

type Credentials struct {
	state                      protoimpl.MessageState `protogen:"opaque.v1"`
	xxx_hidden_AccessKeyId     *string                `protobuf:"bytes,1,opt,name=access_key_id,json=accessKeyId"`
	xxx_hidden_SecretAccessKey *string                `protobuf:"bytes,2,opt,name=secret_access_key,json=secretAccessKey"`
	xxx_hidden_SessionToken    *string                `protobuf:"bytes,3,opt,name=session_token,json=sessionToken"`
	xxx_hidden_Region          *string                `protobuf:"bytes,4,opt,name=region"`
	xxx_hidden_Endpoint        *string                `protobuf:"bytes,5,opt,name=endpoint"`
	XXX_raceDetectHookData     protoimpl.RaceDetectHookData
	XXX_presence               [1]uint32
	unknownFields              protoimpl.UnknownFields
	sizeCache                  protoimpl.SizeCache
}

func (x *Credentials) Reset() {
	*x = Credentials{}
	mi := &file_s3_credentials_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Credentials) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Credentials) ProtoMessage() {}

func (x *Credentials) ProtoReflect() protoreflect.Message {
	mi := &file_s3_credentials_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

func (x *Credentials) GetAccessKeyId() string {
	if x != nil {
		if x.xxx_hidden_AccessKeyId != nil {
			return *x.xxx_hidden_AccessKeyId
		}
		return ""
	}
	return ""
}

func (x *Credentials) GetSecretAccessKey() string {
	if x != nil {
		if x.xxx_hidden_SecretAccessKey != nil {
			return *x.xxx_hidden_SecretAccessKey
		}
		return ""
	}
	return ""
}

func (x *Credentials) GetSessionToken() string {
	if x != nil {
		if x.xxx_hidden_SessionToken != nil {
			return *x.xxx_hidden_SessionToken
		}
		return ""
	}
	return ""
}

func (x *Credentials) GetRegion() string {
	if x != nil {
		if x.xxx_hidden_Region != nil {
			return *x.xxx_hidden_Region
		}
		return ""
	}
	return ""
}

func (x *Credentials) GetEndpoint() string {
	if x != nil {
		if x.xxx_hidden_Endpoint != nil {
			return *x.xxx_hidden_Endpoint
		}
		return ""
	}
	return ""
}

func (x *Credentials) SetAccessKeyId(v string) {
	x.xxx_hidden_AccessKeyId = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 0, 5)
}

func (x *Credentials) SetSecretAccessKey(v string) {
	x.xxx_hidden_SecretAccessKey = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 1, 5)
}

func (x *Credentials) SetSessionToken(v string) {
	x.xxx_hidden_SessionToken = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 2, 5)
}

func (x *Credentials) SetRegion(v string) {
	x.xxx_hidden_Region = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 3, 5)
}

func (x *Credentials) SetEndpoint(v string) {
	x.xxx_hidden_Endpoint = &v
	protoimpl.X.SetPresent(&(x.XXX_presence[0]), 4, 5)
}

func (x *Credentials) HasAccessKeyId() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 0)
}

func (x *Credentials) HasSecretAccessKey() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 1)
}

func (x *Credentials) HasSessionToken() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 2)
}

func (x *Credentials) HasRegion() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 3)
}

func (x *Credentials) HasEndpoint() bool {
	if x == nil {
		return false
	}
	return protoimpl.X.Present(&(x.XXX_presence[0]), 4)
}

func (x *Credentials) ClearAccessKeyId() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 0)
	x.xxx_hidden_AccessKeyId = nil
}

func (x *Credentials) ClearSecretAccessKey() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 1)
	x.xxx_hidden_SecretAccessKey = nil
}

func (x *Credentials) ClearSessionToken() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 2)
	x.xxx_hidden_SessionToken = nil
}

func (x *Credentials) ClearRegion() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 3)
	x.xxx_hidden_Region = nil
}

func (x *Credentials) ClearEndpoint() {
	protoimpl.X.ClearPresent(&(x.XXX_presence[0]), 4)
	x.xxx_hidden_Endpoint = nil
}

type Credentials_builder struct {
	_ [0]func() // Prevents comparability and use of unkeyed literals for the builder.

	AccessKeyId     *string
	SecretAccessKey *string
	SessionToken    *string
	Region          *string
	Endpoint        *string
}

func (b0 Credentials_builder) Build() *Credentials {
	m0 := &Credentials{}
	b, x := &b0, m0
	_, _ = b, x
	if b.AccessKeyId != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 0, 5)
		x.xxx_hidden_AccessKeyId = b.AccessKeyId
	}
	if b.SecretAccessKey != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 1, 5)
		x.xxx_hidden_SecretAccessKey = b.SecretAccessKey
	}
	if b.SessionToken != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 2, 5)
		x.xxx_hidden_SessionToken = b.SessionToken
	}
	if b.Region != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 3, 5)
		x.xxx_hidden_Region = b.Region
	}
	if b.Endpoint != nil {
		protoimpl.X.SetPresentNonAtomic(&(x.XXX_presence[0]), 4, 5)
		x.xxx_hidden_Endpoint = b.Endpoint
	}
	return m0
}

var File_s3_credentials_proto protoreflect.FileDescriptor

var file_s3_credentials_proto_rawDesc = string([]byte{
	0x0a, 0x14, 0x73, 0x33, 0x5f, 0x63, 0x72, 0x65, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb6, 0x01, 0x0a, 0x0b, 0x43, 0x72, 0x65, 0x64, 0x65,
	0x6e, 0x74, 0x69, 0x61, 0x6c, 0x73, 0x12, 0x22, 0x0a, 0x0d, 0x61, 0x63, 0x63, 0x65, 0x73, 0x73,
	0x5f, 0x6b, 0x65, 0x79, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x61,
	0x63, 0x63, 0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x49, 0x64, 0x12, 0x2a, 0x0a, 0x11, 0x73, 0x65,
	0x63, 0x72, 0x65, 0x74, 0x5f, 0x61, 0x63, 0x63, 0x65, 0x73, 0x73, 0x5f, 0x6b, 0x65, 0x79, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x73, 0x65, 0x63, 0x72, 0x65, 0x74, 0x41, 0x63, 0x63,
	0x65, 0x73, 0x73, 0x4b, 0x65, 0x79, 0x12, 0x23, 0x0a, 0x0d, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f,
	0x6e, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x73,
	0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x16, 0x0a, 0x06, 0x72,
	0x65, 0x67, 0x69, 0x6f, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x65, 0x67,
	0x69, 0x6f, 0x6e, 0x12, 0x1a, 0x0a, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x42,
	0x4f, 0x5a, 0x4d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x73, 0x6f,
	0x6c, 0x69, 0x64, 0x44, 0x6f, 0x57, 0x61, 0x6e, 0x74, 0x2f, 0x62, 0x61, 0x63, 0x6b, 0x75, 0x70,
	0x2d, 0x74, 0x6f, 0x6f, 0x6c, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x67, 0x72, 0x70, 0x63, 0x2f, 0x67,
	0x65, 0x6e, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x61, 0x63, 0x6b, 0x75, 0x70, 0x2d,
	0x74, 0x6f, 0x6f, 0x6c, 0x2f, 0x73, 0x33, 0x2f, 0x76, 0x31, 0x3b, 0x73, 0x33, 0x5f, 0x76, 0x31,
	0x62, 0x08, 0x65, 0x64, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x70, 0xe8, 0x07,
})

var file_s3_credentials_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_s3_credentials_proto_goTypes = []any{
	(*Credentials)(nil), // 0: Credentials
}
var file_s3_credentials_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_s3_credentials_proto_init() }
func file_s3_credentials_proto_init() {
	if File_s3_credentials_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_s3_credentials_proto_rawDesc), len(file_s3_credentials_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_s3_credentials_proto_goTypes,
		DependencyIndexes: file_s3_credentials_proto_depIdxs,
		MessageInfos:      file_s3_credentials_proto_msgTypes,
	}.Build()
	File_s3_credentials_proto = out.File
	file_s3_credentials_proto_goTypes = nil
	file_s3_credentials_proto_depIdxs = nil
}
