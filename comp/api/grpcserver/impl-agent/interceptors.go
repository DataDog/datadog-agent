// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentimpl

import (
	"context"

	googleGrpc "google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// newOversizedMessageInterceptors builds the unary and stream server interceptors that
// increment a telemetry counter when an outgoing gRPC message's serialized size
// exceeds threshold. The counter is tagged by full RPC method
// (`/package.Service/Method`). Returns nil interceptors when threshold is non-positive,
// which the caller should drop from the server option list.
func newOversizedMessageInterceptors(threshold int, t telemetry.Component) (googleGrpc.UnaryServerInterceptor, googleGrpc.StreamServerInterceptor) {
	if threshold <= 0 {
		return nil, nil
	}
	counter := t.NewCounter(
		"agent_ipc",
		"grpc_oversized_messages",
		[]string{"method"},
		"Number of outgoing gRPC messages whose serialized size exceeded `agent_ipc.grpc_warning_message_size`.",
	)

	unary := func(ctx context.Context, req any, info *googleGrpc.UnaryServerInfo, handler googleGrpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			checkMessageSize(resp, info.FullMethod, threshold, counter)
		}
		return resp, err
	}

	stream := func(srv any, ss googleGrpc.ServerStream, info *googleGrpc.StreamServerInfo, handler googleGrpc.StreamHandler) error {
		return handler(srv, &oversizedMessageStream{
			ServerStream: ss,
			method:       info.FullMethod,
			threshold:    threshold,
			counter:      counter,
		})
	}

	return unary, stream
}

// oversizedMessageStream wraps grpc.ServerStream so we can observe the size of every
// outgoing message via SendMsg.
type oversizedMessageStream struct {
	googleGrpc.ServerStream
	method    string
	threshold int
	counter   telemetry.Counter
}

func (s *oversizedMessageStream) SendMsg(m any) error {
	checkMessageSize(m, s.method, s.threshold, s.counter)
	return s.ServerStream.SendMsg(m)
}

func checkMessageSize(m any, method string, threshold int, counter telemetry.Counter) {
	pm, ok := m.(proto.Message)
	if !ok {
		return
	}
	size := proto.Size(pm)
	if size <= threshold {
		return
	}
	counter.Inc(method)
}
