// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcprotocoltestimpl

import (
	"context"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// GrpcPingPonger implements PingPonger using the RcEcho gRPC service.
type GrpcPingPonger struct {
	conn   *grpc.ClientConn
	stream grpc.BidiStreamingClient[pbgo.RunEchoTestRequest, pbgo.RunEchoTestResponse]
	cancel context.CancelFunc
}

// NewGrpcPingPonger connects to the RC gRPC echo endpoint and returns a
// GrpcPingPonger ready to exchange frames.
func NewGrpcPingPonger(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) (*GrpcPingPonger, error) {
	conn, meta, err := newGrpcClient(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	meta.Set("X-Echo-Run-Count", strconv.FormatUint(runCount, 10))
	meta.Set("X-Agent-UUID", uuid.GetUUID())

	ctx = metadata.NewOutgoingContext(ctx, meta)
	// Set a stream deadline that the gRPC library encodes as the
	// "grpc-timeout" wire header. Envoy uses this to allow long-lived
	// streams instead of applying its default route timeout.
	ctx, cancel := context.WithTimeout(ctx, 7*24*time.Hour)

	client := pbgo.NewRcEchoClient(conn)
	stream, err := client.RunEchoTest(ctx)
	if err != nil {
		cancel()
		conn.Close()
		return nil, err
	}

	return &GrpcPingPonger{
		conn:   conn,
		stream: stream,
		cancel: cancel,
	}, nil
}

// Recv reads the next frame from the server.
func (g *GrpcPingPonger) Recv(_ context.Context) ([]byte, error) {
	msg, err := g.stream.Recv()
	if err != nil {
		return nil, err
	}

	return msg.Data, nil
}

// Send transmits a frame to the server.
func (g *GrpcPingPonger) Send(_ context.Context, data []byte) error {
	return g.stream.Send(&pbgo.RunEchoTestRequest{Data: data})
}

// GracefulClose cancels any in-flight RPCs and closes the underlying gRPC
// connection.
func (g *GrpcPingPonger) GracefulClose() {
	_ = g.stream.CloseSend()
	g.cancel()
	g.conn.Close()
}

// newGrpcClient connects to the RC gRPC backend and returns a new connection
// and the metadata to attach to the outgoing stream.
func newGrpcClient(_ context.Context, httpClient *api.HTTPClient) (*grpc.ClientConn, metadata.MD, error) {
	// Extract the TLS & Proxy configuration from the HTTP client.
	transport, err := httpClient.Transport()
	if err != nil {
		return nil, nil, err
	}

	// Parse the "base URL" the client uses to connect to RC.
	url, err := httpClient.BaseURL()
	if err != nil {
		return nil, nil, err
	}

	// The request MUST include the same auth credentials as the plain HTTP
	// requests.
	headers := httpClient.Headers()
	meta := metadata.MD{}
	for k, v := range headers {
		meta[k] = v
	}

	// grpc.NewClient expects an authority (host or host:port), not a full
	// URL.  The RPC method path (/datadog.config.RcEcho/RunEchoTest) is
	// determined by the generated protobuf code.
	target := url.Host
	log.Debugf("connecting to grpc endpoint %s", target)

	conn, err := grpc.NewClient(
		target,
		// Copy the User-Agent header to propagate the agent version.
		grpc.WithUserAgent(headers.Get("User-Agent")),
		// Respect any user-provided TLS config.
		grpc.WithTransportCredentials(credentials.NewTLS(transport.TLSClientConfig)),
	)
	if err != nil {
		return nil, nil, err
	}

	log.Debug("grpc connected")

	return conn, meta, nil
}
