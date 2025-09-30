// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package grpc

import (
	"context"

	"google.golang.org/grpc"
)

// ServerOptionsWithMetrics returns gRPC server options with metrics interceptors
// IMPORTANT: This function should be called BEFORE adding other interceptors to avoid conflicts.
// If you already have interceptors configured, you need to manually chain them or use
// the metrics interceptors directly in your server setup.
func ServerOptionsWithMetrics(opts ...grpc.ServerOption) []grpc.ServerOption {
	metricsOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(UnaryServerInterceptor()),
		grpc.StreamInterceptor(StreamServerInterceptor()),
	}

	// Prepend metrics interceptors to existing options
	return append(metricsOpts, opts...)
}

// CombinedUnaryServerInterceptor creates a unary interceptor that combines metrics and auth
func CombinedUnaryServerInterceptor(authInterceptor grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// First run metrics interceptor
		metricsHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
			// Then run auth interceptor
			authHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
				// Finally run the actual handler
				return handler(ctx, req)
			}
			return authInterceptor(ctx, req, info, authHandler)
		}
		return UnaryServerInterceptor()(ctx, req, info, metricsHandler)
	}
}

// CombinedStreamServerInterceptor creates a stream interceptor that combines metrics and auth
func CombinedStreamServerInterceptor(authInterceptor grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// First run metrics interceptor
		metricsHandler := func(srv interface{}, stream grpc.ServerStream) error {
			// Then run auth interceptor
			authHandler := func(srv interface{}, stream grpc.ServerStream) error {
				// Finally run the actual handler
				return handler(srv, stream)
			}
			return authInterceptor(srv, stream, info, authHandler)
		}
		return StreamServerInterceptor()(srv, ss, info, metricsHandler)
	}
}

// ServerOptionsWithMetricsAndAuth creates server options with both metrics and auth interceptors
func ServerOptionsWithMetricsAndAuth(authUnaryInterceptor grpc.UnaryServerInterceptor, authStreamInterceptor grpc.StreamServerInterceptor, opts ...grpc.ServerOption) []grpc.ServerOption {
	serverOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(CombinedUnaryServerInterceptor(authUnaryInterceptor)),
		grpc.StreamInterceptor(CombinedStreamServerInterceptor(authStreamInterceptor)),
	}

	// Add other options
	serverOpts = append(serverOpts, opts...)

	return serverOpts
}

// ClientOptionsWithMetrics returns gRPC client options with metrics interceptors
func ClientOptionsWithMetrics(opts ...grpc.DialOption) []grpc.DialOption {
	metricsOpts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(StreamClientInterceptor()),
	}

	// Prepend metrics interceptors to existing options
	return append(metricsOpts, opts...)
}

// NewServerWithMetrics creates a new gRPC server with metrics interceptors
func NewServerWithMetrics(opts ...grpc.ServerOption) *grpc.Server {
	return grpc.NewServer(ServerOptionsWithMetrics(opts...)...)
}
