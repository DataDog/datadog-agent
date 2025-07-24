// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package grpc

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// extractMethodInfo extracts method and service information from the full method name
func extractMethodInfo(fullMethod string) (method, service string) {
	// Full method format: /package.service/method
	// Example: /datadog.agent.AgentService/GetStatus
	parts := strings.Split(fullMethod, "/")
	if len(parts) != 3 {
		return fullMethod, "unknown"
	}

	method = parts[2]
	service = parts[1]
	return method, service
}

// extractPeerInfo extracts peer information from the context
func extractPeerInfo(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return p.Addr.String()
	}
	return "unknown"
}

// getStatusCode returns the gRPC status code as a string
func getStatusCode(err error) string {
	if err == nil {
		return "OK"
	}

	if st, ok := status.FromError(err); ok {
		return st.Code().String()
	}
	return "UNKNOWN"
}

// getErrorCode returns a more specific error code for error tracking
func getErrorCode(err error) string {
	if err == nil {
		return "OK"
	}

	if st, ok := status.FromError(err); ok {
		code := st.Code()
		return code.String()
	}

	// Check for transport errors
	errStr := err.Error()
	if strings.Contains(errStr, "connection") {
		return "connection_error"
	}
	if strings.Contains(errStr, "timeout") {
		return "timeout_error"
	}
	if strings.Contains(errStr, "canceled") {
		return "canceled_error"
	}

	return "unknown_error"
}

// UnaryServerInterceptor returns a server-side unary interceptor for gRPC metrics
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Extract metadata
		method, service := extractMethodInfo(info.FullMethod)
		peer := extractPeerInfo(ctx)

		// Track active requests
		ActiveRequests.Add(1, method, service, peer)
		defer ActiveRequests.Add(-1, method, service, peer)

		// Track payload size if available
		if req != nil {
			if size, ok := req.(interface{ Size() int }); ok {
				payloadSize := float64(size.Size())
				PayloadSize.Observe(payloadSize, method, service, peer, "request")
			}
		}

		// Call the handler
		resp, err := handler(ctx, req)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := getStatusCode(err)
		errorCode := getErrorCode(err)

		RequestCount.Inc(method, service, peer, statusCode)
		RequestDuration.Observe(duration, method, service, peer)

		if err != nil {
			ErrorCount.Inc(method, service, peer, errorCode)
			log.Debugf("gRPC error: method=%s, service=%s, peer=%s, error=%v", method, service, peer, err)
		}

		// Track response payload size if available
		if resp != nil {
			if size, ok := resp.(interface{ Size() int }); ok {
				payloadSize := float64(size.Size())
				PayloadSize.Observe(payloadSize, method, service, peer, "response")
			}
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a server-side stream interceptor for gRPC metrics
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		// Extract metadata
		method, service := extractMethodInfo(info.FullMethod)
		peer := extractPeerInfo(ss.Context())

		// Track active requests
		ActiveRequests.Add(1, method, service, peer)
		defer ActiveRequests.Add(-1, method, service, peer)

		// Create a wrapped stream to track payload sizes
		wrappedStream := &metricsServerStream{
			ServerStream: ss,
			method:       method,
			service:      service,
			peer:         peer,
		}

		// Call the handler
		err := handler(srv, wrappedStream)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := getStatusCode(err)
		errorCode := getErrorCode(err)

		RequestCount.Inc(method, service, peer, statusCode)
		RequestDuration.Observe(duration, method, service, peer)

		if err != nil {
			ErrorCount.Inc(method, service, peer, errorCode)
			log.Debugf("gRPC stream error: method=%s, service=%s, peer=%s, error=%v", method, service, peer, err)
		}

		return err
	}
}

// UnaryClientInterceptor returns a client-side unary interceptor for gRPC metrics
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()

		// Extract metadata
		methodName, service := extractMethodInfo(method)
		peer := extractPeerInfo(ctx)

		// Track active requests
		ActiveRequests.Add(1, methodName, service, peer)
		defer ActiveRequests.Add(-1, methodName, service, peer)

		// Track payload size if available
		if req != nil {
			if size, ok := req.(interface{ Size() int }); ok {
				payloadSize := float64(size.Size())
				PayloadSize.Observe(payloadSize, methodName, service, peer, "request")
			}
		}

		// Call the invoker
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := getStatusCode(err)
		errorCode := getErrorCode(err)

		RequestCount.Inc(methodName, service, peer, statusCode)
		RequestDuration.Observe(duration, methodName, service, peer)

		if err != nil {
			ErrorCount.Inc(methodName, service, peer, errorCode)
			log.Debugf("gRPC client error: method=%s, service=%s, peer=%s, error=%v", methodName, service, peer, err)
		}

		// Track response payload size if available
		if reply != nil {
			if size, ok := reply.(interface{ Size() int }); ok {
				payloadSize := float64(size.Size())
				PayloadSize.Observe(payloadSize, methodName, service, peer, "response")
			}
		}

		return err
	}
}

// StreamClientInterceptor returns a client-side stream interceptor for gRPC metrics
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		start := time.Now()

		// Extract metadata
		methodName, service := extractMethodInfo(method)
		peer := extractPeerInfo(ctx)

		// Track active requests
		ActiveRequests.Add(1, methodName, service, peer)

		// Call the streamer
		clientStream, err := streamer(ctx, desc, cc, method, opts...)

		// Record metrics
		duration := time.Since(start).Seconds()
		statusCode := getStatusCode(err)
		errorCode := getErrorCode(err)

		RequestCount.Inc(methodName, service, peer, statusCode)
		RequestDuration.Observe(duration, methodName, service, peer)

		if err != nil {
			ErrorCount.Inc(methodName, service, peer, errorCode)
			log.Debugf("gRPC client stream error: method=%s, service=%s, peer=%s, error=%v", methodName, service, peer, err)
			ActiveRequests.Add(-1, methodName, service, peer)
			return nil, err
		}

		// Wrap the client stream to track payload sizes and active requests
		wrappedStream := &metricsClientStream{
			ClientStream: clientStream,
			method:       methodName,
			service:      service,
			peer:         peer,
		}

		return wrappedStream, nil
	}
}

// metricsServerStream wraps a grpc.ServerStream to track payload sizes
type metricsServerStream struct {
	grpc.ServerStream
	method  string
	service string
	peer    string
}

func (s *metricsServerStream) SendMsg(m interface{}) error {
	err := s.ServerStream.SendMsg(m)

	// Track payload size if available
	if m != nil {
		if size, ok := m.(interface{ Size() int }); ok {
			payloadSize := float64(size.Size())
			PayloadSize.Observe(payloadSize, s.method, s.service, s.peer, "response")
		}
	}

	return err
}

func (s *metricsServerStream) RecvMsg(m interface{}) error {
	err := s.ServerStream.RecvMsg(m)

	// Track payload size if available
	if m != nil {
		if size, ok := m.(interface{ Size() int }); ok {
			payloadSize := float64(size.Size())
			PayloadSize.Observe(payloadSize, s.method, s.service, s.peer, "request")
		}
	}

	return err
}

// metricsClientStream wraps a grpc.ClientStream to track payload sizes and active requests
type metricsClientStream struct {
	grpc.ClientStream
	method  string
	service string
	peer    string
}

func (s *metricsClientStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)

	// Track payload size if available
	if m != nil {
		if size, ok := m.(interface{ Size() int }); ok {
			payloadSize := float64(size.Size())
			PayloadSize.Observe(payloadSize, s.method, s.service, s.peer, "request")
		}
	}

	return err
}

func (s *metricsClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)

	// Track payload size if available
	if m != nil {
		if size, ok := m.(interface{ Size() int }); ok {
			payloadSize := float64(size.Size())
			PayloadSize.Observe(payloadSize, s.method, s.service, s.peer, "response")
		}
	}

	return err
}

func (s *metricsClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		ErrorCount.Inc(s.method, s.service, s.peer, "close_send_error")
	}
	return err
}

func (s *metricsClientStream) Context() context.Context {
	ctx := s.ClientStream.Context()

	// Decrement active requests when the stream ends
	go func() {
		<-ctx.Done()
		ActiveRequests.Add(-1, s.method, s.service, s.peer)
	}()

	return ctx
}
