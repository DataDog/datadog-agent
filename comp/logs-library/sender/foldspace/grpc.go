// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GRPCTransport dials one ClientConn per sender and opens stream generations
// on that connection, each on the destination's StreamMethod.
type GRPCTransport struct {
	mu        sync.Mutex
	specs     []SenderSpec
	conns     []*grpc.ClientConn
	state     int
	enc       string
	method    string
	keepalive keepalive.ClientParameters
}

// NewGRPCTransport builds a transport for dest.
//
// The server→client direction carries only BatchStatus acks (a batch ID and a
// status code), capped at pipeline_depth outstanding per sender, so it never
// approaches even grpc-go's smallest flow-control window. The client→server
// direction, which does carry the multi-megabyte batches, is governed by the
// window the server grants; a client-side InitialWindowSize/InitialConnWindowSize
// has no effect on it. There is accordingly no window worth configuring here,
// and grpc-go's own dynamic (BDP-estimated) default applies.
func NewGRPCTransport(dest *DestinationConfig) *GRPCTransport {
	enc := "identity"
	if dest.Core.Compression == Zstd {
		enc = "zstd"
	}
	method := dest.StreamMethod
	if method == "" {
		method = statefulStreamFullMethod
	}
	return &GRPCTransport{
		specs:  dest.Senders,
		conns:  make([]*grpc.ClientConn, len(dest.Senders)),
		state:  dest.StateRequestBytes,
		enc:    enc,
		method: method,
		// Pings only while a stream is open: servers reject stream-less pings by
		// default, and between streams there is nothing to detect a stall in.
		keepalive: keepalive.ClientParameters{
			Time:    dest.KeepaliveTime,
			Timeout: dest.KeepaliveTimeout,
		},
	}
}

// OpenStream dials if needed and starts a stream, giving up when ctx
// ends. NewStream waits under the stream's own context for the connection to
// become ready, which an intake that accepts TCP and never sends its HTTP/2
// preface leaves pending, so ctx is bound to that context only for the open.
func (t *GRPCTransport) OpenStream(ctx context.Context, sender SenderID, _ StreamID) (Stream, error) {
	conn, err := t.conn(ctx, sender)
	if err != nil {
		return nil, err
	}
	spec := t.specs[sender]
	md := metadata.Pairs(
		"dd-api-key", spec.APIKey(),
		"dd-content-encoding", t.enc,
		"dd-state-request-bytes", strconv.Itoa(t.state),
	)
	streamCtx, cancel := context.WithCancel(metadata.NewOutgoingContext(context.Background(), md))
	stop := context.AfterFunc(ctx, cancel)
	clientStream, err := conn.NewStream(streamCtx, &grpc.StreamDesc{
		StreamName:    path.Base(t.method),
		ServerStreams: true,
		ClientStreams: true,
	}, t.method, grpc.ForceCodec(statefulCodec{}))
	if !stop() {
		cancel()
		return nil, fmt.Errorf("open stream: %w", ctx.Err())
	}
	if err != nil {
		cancel()
		return nil, classifyGRPC(err)
	}
	return &grpcStream{stream: clientStream, cancel: cancel}, nil
}

func (t *GRPCTransport) conn(ctx context.Context, sender SenderID) (*grpc.ClientConn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conns[sender] != nil {
		return t.conns[sender], nil
	}
	spec := t.specs[sender]
	opts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.ForceCodec(statefulCodec{})),
	}
	if t.keepalive.Time > 0 {
		opts = append(opts, grpc.WithKeepaliveParams(t.keepalive))
	}
	if spec.UseTLS {
		host, _, _ := net.SplitHostPort(spec.Address)
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.DialContext(ctx, spec.Address, opts...) //nolint:staticcheck // DialContext matches the connect-timeout ctx
	if err != nil {
		return nil, err
	}
	t.conns[sender] = conn
	return conn, nil
}

// Close tears down every ClientConn.
func (t *GRPCTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, conn := range t.conns {
		if conn != nil {
			_ = conn.Close()
			t.conns[i] = nil
		}
	}
}

type grpcStream struct {
	stream grpc.ClientStream
	cancel context.CancelFunc
}

// Send gives up when ctx ends by cancelling the whole stream: SendMsg ignores
// any context but the stream's own, and is otherwise released from a
// flow-control wait only by the intake granting window.
func (s *grpcStream) Send(ctx context.Context, batchID uint32, data []byte) error {
	stop := context.AfterFunc(ctx, s.cancel)
	err := s.stream.SendMsg(&StatefulBatch{BatchID: batchID, Data: data})
	if !stop() {
		return fmt.Errorf("send batch %d: %w", batchID, ctx.Err())
	}
	return classifyGRPC(err)
}

func (s *grpcStream) Recv(_ context.Context) (uint32, int32, error) {
	var statusMsg BatchStatus
	if err := s.stream.RecvMsg(&statusMsg); err != nil {
		return 0, 0, classifyGRPC(err)
	}
	return statusMsg.BatchID, statusMsg.Status, nil
}

func (s *grpcStream) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return s.stream.CloseSend()
}

func classifyGRPC(err error) error {
	if err == nil {
		return nil
	}
	if status.Code(err) == codes.FailedPrecondition {
		return fmt.Errorf("stateful encoding disabled: %w", err)
	}
	return err
}

// Ensure encoding.Codec is referenced so ForceCodec type-checks.
var _ encoding.Codec = statefulCodec{}

// StatefulIntakeServer is the gRPC service mock intakes implement.
type StatefulIntakeServer interface {
	StatefulStream(grpc.BidiStreamingServer[StatefulBatch, BatchStatus]) error
	mustEmbedUnimplementedStatefulIntakeServer()
}

// UnimplementedStatefulIntakeServer is embedded for forward compatibility.
type UnimplementedStatefulIntakeServer struct{}

func (UnimplementedStatefulIntakeServer) StatefulStream(grpc.BidiStreamingServer[StatefulBatch, BatchStatus]) error {
	return status.Error(codes.Unimplemented, "method StatefulStream not implemented")
}
func (UnimplementedStatefulIntakeServer) mustEmbedUnimplementedStatefulIntakeServer() {}
func (UnimplementedStatefulIntakeServer) testEmbeddedByValue()                        {}

// RegisterStatefulIntakeServer registers the service on s.
func RegisterStatefulIntakeServer(s grpc.ServiceRegistrar, srv StatefulIntakeServer) {
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&statefulIntakeServiceDesc, srv)
}

func statefulStreamHandler(srv any, stream grpc.ServerStream) error {
	return srv.(StatefulIntakeServer).StatefulStream(&grpc.GenericServerStream[StatefulBatch, BatchStatus]{ServerStream: stream})
}

var statefulIntakeServiceDesc = grpc.ServiceDesc{
	ServiceName: statefulServiceName,
	HandlerType: (*StatefulIntakeServer)(nil),
	Streams: []grpc.StreamDesc{
		{
			StreamName:    statefulStreamName,
			Handler:       statefulStreamHandler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
}

// NewStatefulIntakeServer returns a grpc.Server that speaks StatefulStream
// with the foldspace codec.
func NewStatefulIntakeServer(srv StatefulIntakeServer, opts ...grpc.ServerOption) *grpc.Server {
	opts = append([]grpc.ServerOption{grpc.ForceServerCodec(statefulCodec{})}, opts...)
	s := grpc.NewServer(opts...)
	RegisterStatefulIntakeServer(s, srv)
	return s
}

// EncodingName is the dd-content-encoding header value.
func EncodingName(c Compression) string {
	if c == Zstd {
		return "zstd"
	}
	return "identity"
}

// HeaderAPIKey reads dd-api-key from incoming metadata.
func HeaderAPIKey(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("dd-api-key")
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}
