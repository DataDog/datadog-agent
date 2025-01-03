// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/api/security/auth"
)

// GetUnaryServerInterceptor returns a new unary server interceptor that performs
// authorization using the provided Authorizer. The interceptor retrieves metadata
// from the incoming context and verifies it using the Authorizer's VerifyGRPC method.
// If the metadata is missing or verification fails, the interceptor returns an
// Unauthenticated status error. Otherwise, it proceeds to call the handler.
func GetUnaryServerInterceptor(a auth.Authorizer) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Retrieve metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "missing metadata")
		}

		err := a.VerifyGRPC(info.FullMethod, md)

		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		return handler(ctx, req)
	}
}

// GetStreamServerInterceptor validates the signature for streaming RPCs
// GetStreamServerInterceptor returns a new grpc.StreamServerInterceptor that uses the provided
// auth.Authorizer to verify the metadata of incoming gRPC stream requests. If the metadata is
// missing or the authorization fails, it returns an Unauthenticated status error. Otherwise,
// it proceeds with the provided handler.
func GetStreamServerInterceptor(a auth.Authorizer) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Retrieve metadata
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Errorf(codes.Unauthenticated, "missing metadata")
		}

		err := a.VerifyGRPC(info.FullMethod, md)
		if err != nil {
			return status.Error(codes.Unauthenticated, err.Error())
		}

		return handler(srv, ss)
	}
}

// GetUnaryClientInterceptor returns a grpc.UnaryClientInterceptor that adds
// authorization metadata to outgoing gRPC requests. The interceptor uses the
// provided auth.Authorizer to generate a signature and timestamp, which are
// then added to the request metadata.
func GetUnaryClientInterceptor(a auth.Authorizer) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {

		// // Serialize the request body
		// body, err := proto.Marshal(req.(proto.Message))
		// if err != nil {
		// 	return err
		// }

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = make(metadata.MD)
		}

		// Generate the signature
		err := a.SignGRPC(method, md)
		if err != nil {
			return err
		}

		// Add signature and timestamp to metadata
		// md.Append(headers)
		ctx = metadata.NewOutgoingContext(ctx, md)

		// Invoke the RPC
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
