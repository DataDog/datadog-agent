// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

type recordingIntake struct {
	UnimplementedStatefulIntakeServer
	mu   sync.Mutex
	got  [][]byte
	keys []string
}

func (r *recordingIntake) StatefulStream(stream grpc.BidiStreamingServer[StatefulBatch, BatchStatus]) error {
	key := HeaderAPIKey(stream.Context())
	r.mu.Lock()
	r.keys = append(r.keys, key)
	r.mu.Unlock()
	for {
		batch, err := stream.Recv()
		if err != nil {
			return nil
		}
		r.mu.Lock()
		r.got = append(r.got, append([]byte(nil), batch.Data...))
		r.mu.Unlock()
		if err := stream.Send(&BatchStatus{BatchID: batch.BatchID, Status: AckOK}); err != nil {
			return err
		}
	}
}

func (r *recordingIntake) payloads() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.got...)
}

func startBufIntake(t *testing.T) (*recordingIntake, *bufconn.Listener, *grpc.Server) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	intake := &recordingIntake{}
	srv := NewStatefulIntakeServer(intake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})
	return intake, lis, srv
}

func TestGRPCStreamRoundTrip(t *testing.T) {
	intake, lis, _ := startBufIntake(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "buf", //nolint:staticcheck
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(statefulCodec{})),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    statefulStreamName,
		ServerStreams: true,
		ClientStreams: true,
	}, statefulStreamFullMethod, grpc.ForceCodec(statefulCodec{}))
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(&StatefulBatch{BatchID: 1, Data: []byte("ping")}))
	var status BatchStatus
	require.NoError(t, stream.RecvMsg(&status))
	assert.Equal(t, uint32(1), status.BatchID)
	assert.Equal(t, AckOK, status.Status)
	assert.Equal(t, [][]byte{[]byte("ping")}, intake.payloads())
	require.NoError(t, stream.CloseSend())
}

func TestTwoGRPCIntakesReceiveSameBytes(t *testing.T) {
	a, lisA, _ := startBufIntake(t)
	b, lisB, _ := startBufIntake(t)

	dial := func(lis *bufconn.Listener) *grpc.ClientConn {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(ctx, "buf", //nolint:staticcheck
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(grpc.ForceCodec(statefulCodec{})),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}

	transport := &GRPCTransport{
		specs: []SenderSpec{
			{ID: 0, Address: "a:1", Class: Reliable, APIKey: func() string { return "key-a" }},
			{ID: 1, Address: "b:1", Class: Reliable, APIKey: func() string { return "key-b" }},
		},
		conns:  []*grpc.ClientConn{dial(lisA), dial(lisB)},
		window: 1 << 20,
		state:  1024,
		enc:    "identity",
	}

	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable, Reliable}})
	sink := newChannelSink()
	d := startDriver(t, core, transport, sink, false)
	d.Offer(testMessage("shared-bytes"))
	waitPayloads(t, sink, 1)

	require.Eventually(t, func() bool { return len(a.payloads()) == 1 && len(b.payloads()) == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, a.payloads()[0], b.payloads()[0])
	assert.Equal(t, []byte("shared-bytes"), a.payloads()[0])
	a.mu.Lock()
	assert.Equal(t, []string{"key-a"}, a.keys)
	a.mu.Unlock()
	b.mu.Lock()
	assert.Equal(t, []string{"key-b"}, b.keys)
	b.mu.Unlock()
}

func TestBuildAndDialWindowing(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 16)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)
	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "127.0.0.1", "port": 1, "use_ssl": false})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main})
	endpoints.Main = main
	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	tr := NewGRPCTransport(dest)
	assert.Equal(t, int32(8*endpoints.BatchMaxContentSize), tr.window)
	_ = metrics.TlmFoldspaceDropped
}
