// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
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

// stalledIntake accepts a stream and never reads from it, so the client runs
// out of flow-control window.
type stalledIntake struct {
	UnimplementedStatefulIntakeServer
}

func (stalledIntake) StatefulStream(stream grpc.BidiStreamingServer[StatefulBatch, BatchStatus]) error {
	<-stream.Context().Done()
	return nil
}

// SendMsg waiting on flow-control window ignores every context but the
// stream's own, so a send deadline does nothing unless Send acts on it.
func TestGRPCSendHonorsContextWhenStalled(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	// A fixed window disables grpc-go's BDP growth, so the stall arrives after
	// 64KiB rather than after up to 16MiB.
	srv := NewStatefulIntakeServer(stalledIntake{}, grpc.InitialWindowSize(64<<10), grpc.InitialConnWindowSize(64<<10))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
	defer cancelDial()
	conn, err := grpc.DialContext(dialCtx, "buf", //nolint:staticcheck
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck
		grpc.WithDefaultCallOptions(grpc.ForceCodec(statefulCodec{})),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	transport := &GRPCTransport{
		specs:  []SenderSpec{{ID: 0, Address: "buf:1", Class: Reliable, APIKey: func() string { return "key" }}},
		conns:  []*grpc.ClientConn{conn},
		state:  1024,
		enc:    "identity",
		method: statefulStreamFullMethod,
	}
	stream, err := transport.OpenStream(dialCtx, 0, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		data := make([]byte, 256<<10)
		for id := uint32(1); ; id++ {
			if err := stream.Send(ctx, id, data); err != nil {
				done <- err
				return
			}
		}
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("Send ignored its context while blocked on flow control")
	}
}

// An intake that accepts the connection and never sends its HTTP/2 preface
// leaves the connection connecting, and NewStream waits on it under the
// stream's own context. OpenStream must give up when its own ctx does.
func TestGRPCOpenStreamHonorsContextWhenIntakeIsMute(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = lis.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})

	transport := NewGRPCTransport(&DestinationConfig{
		Senders: []SenderSpec{{ID: 0, Address: lis.Addr().String(), Class: Reliable, APIKey: func() string { return "key" }}},
	}, secretsnoopimpl.NewComponent().Comp)
	t.Cleanup(transport.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := transport.OpenStream(ctx, 0, 1)
		done <- err
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("OpenStream ignored its context while the connection was connecting")
	}
}

// Keepalive is configured from the destination, and only while a stream is
// open: a stream-less ping counts as abuse under default server policy.
func TestGRPCTransportKeepalive(t *testing.T) {
	transport := NewGRPCTransport(&DestinationConfig{
		KeepaliveTime:    5 * time.Minute,
		KeepaliveTimeout: 20 * time.Second,
	}, secretsnoopimpl.NewComponent().Comp)
	assert.Equal(t, 5*time.Minute, transport.keepalive.Time)
	assert.Equal(t, 20*time.Second, transport.keepalive.Timeout)
	assert.False(t, transport.keepalive.PermitWithoutStream)
}

// The stream opens on the destination's method rather than the agent's own,
// so an intake serving the same messages under other names receives it.
func TestGRPCOpenStreamUsesConfiguredMethod(t *testing.T) {
	const method = "/datadog.intake.stateful.StatefulLogsService/LogsStream"
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	opened := make(chan string, 1)
	srv := grpc.NewServer(
		grpc.ForceServerCodec(statefulCodec{}),
		grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
			name, _ := grpc.MethodFromServerStream(stream)
			opened <- name
			var batch StatefulBatch
			if err := stream.RecvMsg(&batch); err != nil {
				return err
			}
			return stream.SendMsg(&BatchStatus{BatchID: batch.BatchID, Status: AckOK})
		}),
	)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	transport := NewGRPCTransport(&DestinationConfig{
		Senders:      []SenderSpec{{ID: 0, Address: lis.Addr().String(), Class: Reliable, APIKey: func() string { return "key" }}},
		StreamMethod: method,
	}, secretsnoopimpl.NewComponent().Comp)
	t.Cleanup(transport.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := transport.OpenStream(ctx, 0, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })
	require.NoError(t, stream.Send(ctx, 7, []byte("ping")))
	batchID, status, err := stream.Recv(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint32(7), batchID)
	assert.Equal(t, AckOK, status)
	assert.Equal(t, method, <-opened)
}

// rejectingIntake fails every stream with code before reading from it, as an
// intake that refuses the stream's API key does.
type rejectingIntake struct {
	UnimplementedStatefulIntakeServer
	code codes.Code
}

func (r rejectingIntake) StatefulStream(grpc.BidiStreamingServer[StatefulBatch, BatchStatus]) error {
	return status.Error(r.code, "rejected")
}

// refreshRecorder counts Refresh calls and reports every value as secret-backed
// or none of them.
type refreshRecorder struct {
	secrets.Component
	fromSecret bool
	refreshes  atomic.Int32
}

func (r *refreshRecorder) IsValueFromSecret(string) bool { return r.fromSecret }

func (r *refreshRecorder) Refresh() bool {
	r.refreshes.Add(1)
	return true
}

// An intake rejecting a secret-backed key is answered with a secrets refresh,
// as an HTTP 403 is, so the stream the core reopens can carry a fresh key.
// Other failures, and keys that did not come from a secret, refresh nothing.
func TestGRPCRefreshesSecretsWhenIntakeRejectsKey(t *testing.T) {
	cases := []struct {
		name        string
		code        codes.Code
		fromSecret  bool
		wantRefresh int32
	}{
		{"unauthenticated secret key", codes.Unauthenticated, true, 1},
		{"permission denied secret key", codes.PermissionDenied, true, 1},
		{"permission denied plain key", codes.PermissionDenied, false, 0},
		{"unavailable secret key", codes.Unavailable, true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			srv := NewStatefulIntakeServer(rejectingIntake{code: c.code})
			go func() { _ = srv.Serve(lis) }()
			t.Cleanup(srv.Stop)

			recorder := &refreshRecorder{fromSecret: c.fromSecret}
			transport := NewGRPCTransport(&DestinationConfig{
				Senders: []SenderSpec{{ID: 0, Address: lis.Addr().String(), Class: Reliable, APIKey: func() string { return "key" }}},
			}, recorder)
			t.Cleanup(transport.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stream, err := transport.OpenStream(ctx, 0, 1)
			require.NoError(t, err)
			t.Cleanup(func() { _ = stream.Close() })
			_ = stream.Send(ctx, 1, []byte("ping"))
			_, _, err = stream.Recv(ctx)
			require.Equal(t, c.code, status.Code(err))
			assert.Equal(t, c.wantRefresh, recorder.refreshes.Load())
		})
	}
}

func TestGRPCStreamReportsChangedCredentials(t *testing.T) {
	_, lis, _ := startBufIntake(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "buf", //nolint:staticcheck
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck
		grpc.WithDefaultCallOptions(grpc.ForceCodec(statefulCodec{})),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var key atomic.Value
	key.Store("old-key")
	transport := &GRPCTransport{
		specs:  []SenderSpec{{ID: 0, Address: "buf:1", Class: Reliable, APIKey: func() string { return key.Load().(string) }}},
		conns:  []*grpc.ClientConn{conn},
		state:  1024,
		enc:    "identity",
		method: statefulStreamFullMethod,
	}
	stream, err := transport.OpenStream(ctx, 0, 1)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })

	assert.False(t, stream.CredentialsChanged())
	key.Store("new-key")
	assert.True(t, stream.CredentialsChanged())
}

func TestGRPCStreamRoundTrip(t *testing.T) {
	intake, lis, _ := startBufIntake(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "buf", //nolint:staticcheck
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck
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
			grpc.WithBlock(), //nolint:staticcheck
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
		state:  1024,
		enc:    "identity",
		method: statefulStreamFullMethod,
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
