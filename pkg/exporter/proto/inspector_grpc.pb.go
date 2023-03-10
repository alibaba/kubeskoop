// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.5.0
// source: inspector.proto

package proto

import (
	context "context"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// InspectorClient is the client API for Inspector service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type InspectorClient interface {
	WatchEvent(ctx context.Context, in *WatchRequest, opts ...grpc.CallOption) (Inspector_WatchEventClient, error)
	QueryMetric(ctx context.Context, in *QueryMetricRequest, opts ...grpc.CallOption) (*QueryMetricResponse, error)
}

type inspectorClient struct {
	cc grpc.ClientConnInterface
}

func NewInspectorClient(cc grpc.ClientConnInterface) InspectorClient {
	return &inspectorClient{cc}
}

func (c *inspectorClient) WatchEvent(ctx context.Context, in *WatchRequest, opts ...grpc.CallOption) (Inspector_WatchEventClient, error) {
	stream, err := c.cc.NewStream(ctx, &Inspector_ServiceDesc.Streams[0], "/proto.inspector/WatchEvent", opts...)
	if err != nil {
		return nil, err
	}
	x := &inspectorWatchEventClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Inspector_WatchEventClient interface {
	Recv() (*WatchReply, error)
	grpc.ClientStream
}

type inspectorWatchEventClient struct {
	grpc.ClientStream
}

func (x *inspectorWatchEventClient) Recv() (*WatchReply, error) {
	m := new(WatchReply)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *inspectorClient) QueryMetric(ctx context.Context, in *QueryMetricRequest, opts ...grpc.CallOption) (*QueryMetricResponse, error) {
	out := new(QueryMetricResponse)
	err := c.cc.Invoke(ctx, "/proto.inspector/QueryMetric", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// InspectorServer is the server API for Inspector service.
// All implementations must embed UnimplementedInspectorServer
// for forward compatibility
type InspectorServer interface {
	WatchEvent(*WatchRequest, Inspector_WatchEventServer) error
	QueryMetric(context.Context, *QueryMetricRequest) (*QueryMetricResponse, error)
	mustEmbedUnimplementedInspectorServer()
}

// UnimplementedInspectorServer must be embedded to have forward compatible implementations.
type UnimplementedInspectorServer struct {
}

func (UnimplementedInspectorServer) WatchEvent(*WatchRequest, Inspector_WatchEventServer) error {
	return status.Errorf(codes.Unimplemented, "method WatchEvent not implemented")
}
func (UnimplementedInspectorServer) QueryMetric(context.Context, *QueryMetricRequest) (*QueryMetricResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryMetric not implemented")
}
func (UnimplementedInspectorServer) mustEmbedUnimplementedInspectorServer() {}

// UnsafeInspectorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to InspectorServer will
// result in compilation errors.
type UnsafeInspectorServer interface {
	mustEmbedUnimplementedInspectorServer()
}

func RegisterInspectorServer(s grpc.ServiceRegistrar, srv InspectorServer) {
	s.RegisterService(&Inspector_ServiceDesc, srv)
}

func _Inspector_WatchEvent_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(WatchRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(InspectorServer).WatchEvent(m, &inspectorWatchEventServer{stream})
}

type Inspector_WatchEventServer interface {
	Send(*WatchReply) error
	grpc.ServerStream
}

type inspectorWatchEventServer struct {
	grpc.ServerStream
}

func (x *inspectorWatchEventServer) Send(m *WatchReply) error {
	return x.ServerStream.SendMsg(m)
}

func _Inspector_QueryMetric_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryMetricRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InspectorServer).QueryMetric(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.inspector/QueryMetric",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InspectorServer).QueryMetric(ctx, req.(*QueryMetricRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Inspector_ServiceDesc is the grpc.ServiceDesc for Inspector service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Inspector_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.inspector",
	HandlerType: (*InspectorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "QueryMetric",
			Handler:    _Inspector_QueryMetric_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "WatchEvent",
			Handler:       _Inspector_WatchEvent_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "inspector.proto",
}
