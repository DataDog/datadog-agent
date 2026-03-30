// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcecho

import (
	"context"
	"path"
	"strconv"

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
	stream grpc.BidiStreamingClient[pbgo.RunEchoTestRequest, pbgo.RunEchoTestResponse]
	cancel context.CancelFunc
}

func NewGrpcPingPonger(ctx context.Context, httpClient *api.HTTPClient, runCount uint64) (*GrpcPingPonger, error) {
	conn, meta, err := newGrpcClient(ctx, "/api/v0.2/echo-test-grpc", httpClient)
	if err != nil {
		return nil, err
	}

	meta.Set("X-Echo-Run-Count", strconv.FormatUint(runCount, 10))
	meta.Set("X-Agent-UUID", uuid.GetUUID())

	ctx = metadata.NewOutgoingContext(ctx, meta)
	ctx, cancel := context.WithCancel(ctx)

	client := pbgo.NewRcEchoClient(conn)
	stream, err := client.RunEchoTest(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	return &GrpcPingPonger{
		stream: stream,
		cancel: cancel,
	}, nil
}

func (g *GrpcPingPonger) Recv(ctx context.Context) ([]byte, error) {
	msg, err := g.stream.Recv()
	if err != nil {
		return nil, err
	}

	return msg.Data, nil
}

func (g *GrpcPingPonger) Send(ctx context.Context, data []byte) error {
	return g.stream.Send(&pbgo.RunEchoTestRequest{Data: data})
}

// GracefulClose cancels any in-flight RPCs and closes the underlying gRPC
// connection.
func (g *GrpcPingPonger) GracefulClose() {
	g.stream.CloseSend()
	g.cancel()
}

// newGrpcClient connects to the RC gRPC backend and returns a new connection or
// a connection.
//
// The "endpointPath" specifies the resource path to connect to, which is
// appended to the client baseURL.
func newGrpcClient(ctx context.Context, endpointPath string, httpClient *api.HTTPClient) (*grpc.ClientConn, metadata.MD, error) {
	// Extract the TLS & Proxy configuration from the HTTP client.
	transport, err := httpClient.Transport()
	if err != nil {
		return nil, nil, err
	}

	// Parse the "base URL" the client uses to connect to RC.
	url, err := httpClient.BaseUrl()
	if err != nil {
		return nil, nil, err
	}
	// Append the specific path to the API resource.
	url.Path = path.Join(url.Path, endpointPath)

	// The request MUST include the same auth credentials as the plain HTTP
	// requests.
	headers := httpClient.Headers()
	meta := metadata.MD{}
	for k, v := range headers {
		meta[k] = v
	}

	log.Debugf("connecting to grpc endpoint %s", url.String())

	conn, err := grpc.NewClient(
		url.String(),
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
