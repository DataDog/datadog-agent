// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package start

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	configstreamconsumerfx "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetryfx "github.com/DataDog/datadog-agent/comp/core/telemetry/fx"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockCoreAgent implements the subset of AgentSecure the consumer calls:
// RegisterRemoteAgent and StreamConfigEvents. Test feeds events via the channel.
type mockCoreAgent struct {
	pb.UnimplementedAgentSecureServer
	sessionID string
	events    chan *pb.ConfigEvent
	closeOnce sync.Once
}

func (m *mockCoreAgent) RegisterRemoteAgent(_ context.Context, _ *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	return &pb.RegisterRemoteAgentResponse{SessionId: m.sessionID}, nil
}

func (m *mockCoreAgent) StreamConfigEvents(_ *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing gRPC metadata")
	}
	if got := md.Get("session_id"); len(got) == 0 || got[0] != m.sessionID {
		return status.Error(codes.Unauthenticated, "invalid session_id")
	}
	for event := range m.events {
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockCoreAgent) close() {
	m.closeOnce.Do(func() { close(m.events) })
}

// setupFakeCoreAgent generates real IPC cert + auth_token files (so the consumer's
// micro-resolution and mTLS dial succeed) and starts a gRPC server using the same
// TLS config. Returns the listener address (host:port) and a cleanup func.
func setupFakeCoreAgent(t *testing.T, dir string) (addr string, mock *mockCoreAgent, cleanup func()) {
	t.Helper()

	// Seed the global config so cert/auth helpers know where to put their files.
	// GlobalConfigBuilder needed here: it's the only accessor that exposes SetConfigFile (Setup interface).
	pkgconfigsetup.InitConfigObjects()
	cfg := pkgconfigsetup.GlobalConfigBuilder()
	cfg.SetConfigFile(filepath.Join(dir, "datadog.yaml"))

	_, err := pkgtoken.FetchOrCreateAuthToken(context.Background(), cfg)
	require.NoError(t, err)
	_, serverTLS, _, err := cert.FetchOrCreateIPCCert(context.Background(), cfg)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	mock = &mockCoreAgent{
		sessionID: "test-session-id",
		events:    make(chan *pb.ConfigEvent, 16),
	}
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	pb.RegisterAgentSecureServer(grpcServer, mock)
	go func() { _ = grpcServer.Serve(listener) }()

	cleanup = func() {
		mock.close()
		grpcServer.Stop()
		_ = listener.Close()
	}
	return listener.Addr().String(), mock, cleanup
}

func mustNewValue(t *testing.T, v interface{}) *structpb.Value {
	t.Helper()
	val, err := structpb.NewValue(v)
	require.NoError(t, err)
	return val
}

// TestRunBlocksUntilConfigStreamSnapshot verifies the end-to-end wiring:
// the consumer dials the (fake) core, registers with the RAR, opens the stream, and
// blocks fxutil.OneShot until the first snapshot is applied.
func TestRunBlocksUntilConfigStreamSnapshot(t *testing.T) {
	dir := t.TempDir()
	addr, mock, cleanup := setupFakeCoreAgent(t, dir)
	defer cleanup()

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	datadogYaml := fmt.Sprintf(`
cmd_host: %s
cmd_port: %s
auth_token_file_path: %s
ipc_cert_file_path: %s
remote_agent:
  registry:
    enabled: true
  configstream:
    consumer:
      enabled: true
`, host, port,
		filepath.Join(dir, "auth_token"),
		filepath.Join(dir, "ipc_cert.pem"),
	)
	datadogPath := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(datadogPath, []byte(datadogYaml), 0600))

	opts := fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		telemetryfx.Module(),
		fx.Supply(configstreamconsumer.NewParams("security-agent", datadogPath, configstreamconsumer.WithReadyTimeout(10*time.Second))),
		configstreamconsumerfx.Module(),
	)

	testRun := func(_ configstreamconsumer.Component) error { return nil }

	done := make(chan error, 1)
	go func() { done <- fxutil.OneShot(testRun, opts) }()

	select {
	case err := <-done:
		t.Fatalf("OneShot completed before snapshot was sent: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	mock.events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings:   []*pb.ConfigSetting{{Key: "test.key", Value: mustNewValue(t, "ok"), Source: "file"}},
			},
		},
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("OneShot did not complete after sending snapshot")
	}
}

// TestRunNoopWhenConfigstreamDisabled verifies that when configstream is disabled,
// NewComponent returns a no-op and FX startup does NOT block on the consumer.
func TestRunNoopWhenConfigstreamDisabled(t *testing.T) {
	dir := t.TempDir()
	datadogPath := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(datadogPath, []byte(""), 0600))

	// Ensure no env override re-enables.
	t.Setenv("DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED", "false")

	opts := fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		telemetryfx.Module(),
		fx.Supply(configstreamconsumer.NewParams("security-agent", datadogPath)),
		configstreamconsumerfx.Module(),
	)
	testRun := func(_ configstreamconsumer.Component) error { return nil }

	done := make(chan error, 1)
	go func() { done <- fxutil.OneShot(testRun, opts) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("OneShot blocked unexpectedly when configstream is disabled")
	}
}
