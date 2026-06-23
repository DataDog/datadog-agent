// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Code generated manually to match executor.proto. DO NOT EDIT without updating executor.proto.

package executor

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
	Executor_Status_FullMethodName     = "/datadog.privateactionrunner.executor.Executor/Status"
	Executor_SubmitTask_FullMethodName = "/datadog.privateactionrunner.executor.Executor/SubmitTask"
	Executor_Shutdown_FullMethodName   = "/datadog.privateactionrunner.executor.Executor/Shutdown"
)

// ExecutorClient is the client API for Executor service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ExecutorClient interface {
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
	SubmitTask(ctx context.Context, in *SubmitTaskRequest, opts ...grpc.CallOption) (*SubmitTaskResponse, error)
	Shutdown(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
}

type executorClient struct {
	cc grpc.ClientConnInterface
}

func NewExecutorClient(cc grpc.ClientConnInterface) ExecutorClient {
	return &executorClient{cc}
}

func (c *executorClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, Executor_Status_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *executorClient) SubmitTask(ctx context.Context, in *SubmitTaskRequest, opts ...grpc.CallOption) (*SubmitTaskResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SubmitTaskResponse)
	err := c.cc.Invoke(ctx, Executor_SubmitTask_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *executorClient) Shutdown(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, Executor_Shutdown_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExecutorServer is the server API for Executor service.
// All implementations must embed UnimplementedExecutorServer
// for forward compatibility.
type ExecutorServer interface {
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
	SubmitTask(context.Context, *SubmitTaskRequest) (*SubmitTaskResponse, error)
	Shutdown(context.Context, *StatusRequest) (*StatusResponse, error)
	mustEmbedUnimplementedExecutorServer()
}

// UnimplementedExecutorServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedExecutorServer struct{}

func (UnimplementedExecutorServer) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedExecutorServer) SubmitTask(context.Context, *SubmitTaskRequest) (*SubmitTaskResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method SubmitTask not implemented")
}
func (UnimplementedExecutorServer) Shutdown(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method Shutdown not implemented")
}
func (UnimplementedExecutorServer) mustEmbedUnimplementedExecutorServer() {}
func (UnimplementedExecutorServer) testEmbeddedByValue()                  {}

// UnsafeExecutorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ExecutorServer will
// result in compilation errors.
type UnsafeExecutorServer interface {
	mustEmbedUnimplementedExecutorServer()
}

func RegisterExecutorServer(s grpc.ServiceRegistrar, srv ExecutorServer) {
	// If the following call panics, it indicates UnimplementedExecutorServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Executor_ServiceDesc, srv)
}

func _Executor_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutorServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Executor_Status_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExecutorServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Executor_SubmitTask_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SubmitTaskRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutorServer).SubmitTask(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Executor_SubmitTask_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExecutorServer).SubmitTask(ctx, req.(*SubmitTaskRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Executor_Shutdown_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutorServer).Shutdown(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Executor_Shutdown_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExecutorServer).Shutdown(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Executor_ServiceDesc is the grpc.ServiceDesc for Executor service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Executor_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "datadog.privateactionrunner.executor.Executor",
	HandlerType: (*ExecutorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Status",
			Handler:    _Executor_Status_Handler,
		},
		{
			MethodName: "SubmitTask",
			Handler:    _Executor_SubmitTask_Handler,
		},
		{
			MethodName: "Shutdown",
			Handler:    _Executor_Shutdown_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "datadog/privateactionrunner/executor.proto",
}
