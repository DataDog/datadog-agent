// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
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

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockConfigStreamServer is a gRPC server that streams config events when sent on the events channel.
type mockConfigStreamServer struct {
	pb.UnimplementedAgentSecureServer
	events chan *pb.ConfigEvent
	closed bool
}

func (m *mockConfigStreamServer) StreamConfigEvents(_ *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing gRPC metadata")
	}
	sessionIDs := md.Get("session_id")
	if len(sessionIDs) == 0 || sessionIDs[0] == "" {
		return status.Error(codes.Unauthenticated, "session_id required in metadata")
	}
	for event := range m.events {
		if m.closed {
			return io.EOF
		}
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

// mockRAR provides a fixed session ID so the config stream consumer can connect without a real RAR.
type mockRAR struct{}

func (m *mockRAR) WaitSessionID(_ context.Context) (string, error) {
	return "test-session", nil
}

func mustNewValue(t *testing.T, v interface{}) *structpb.Value {
	val, err := structpb.NewValue(v)
	require.NoError(t, err)
	return val
}

// setupMockConfigStreamServer starts a gRPC server that implements the config stream and returns
// the server address and a channel to send events. Calls the returned cleanup when done.
func setupMockConfigStreamServer(t *testing.T, ipcComp *ipcmock.IPCMock) (addr string, events chan *pb.ConfigEvent, cleanup func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr = listener.Addr().String()
	events = make(chan *pb.ConfigEvent, 100)
	mockServer := &mockConfigStreamServer{events: events}
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(ipcComp.GetTLSServerConfig())))
	pb.RegisterAgentSecureServer(grpcServer, mockServer)
	go func() { _ = grpcServer.Serve(listener) }()
	cleanup = func() {
		mockServer.closed = true
		close(events)
		grpcServer.Stop()
		_ = listener.Close()
	}
	return addr, events, cleanup
}

// TestRunBlocksUntilConfigStreamSnapshot verifies that system-probe startup does not complete
// until the config stream sends a snapshot, by running the real FX graph with a mock config stream server.
func TestRunBlocksUntilConfigStreamSnapshot(t *testing.T) {
	configStreamReadyTimeoutForTest = 2 * time.Second
	defer func() { configStreamReadyTimeoutForTest = 0 }()

	ipcComp := ipcmock.New(t)
	serverAddr, events, cleanup := setupMockConfigStreamServer(t, ipcComp)
	defer cleanup()

	host, port, err := net.SplitHostPort(serverAddr)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	datadogPath := filepath.Join(tmpDir, "datadog.yaml")
	sysprobePath := filepath.Join(tmpDir, "system_probe.yaml")

	// Config so system-probe uses our mock server and does not start the real probe.
	datadogYaml := fmt.Sprintf(`
cmd_host: %s
cmd_port: %s
remote_agent_registry:
  enabled: false
`, host, port)
	require.NoError(t, os.WriteFile(datadogPath, []byte(datadogYaml), 0600))
	sysprobeYaml := `
system_probe_config:
  enabled: false
`
	require.NoError(t, os.WriteFile(sysprobePath, []byte(sysprobeYaml), 0600))

	baseOpts := fx.Options(
		fx.Supply(config.NewAgentParams(datadogPath)),
		fx.Supply(sysprobeconfigimpl.NewParams(
			sysprobeconfigimpl.WithSysProbeConfFilePath(sysprobePath),
			sysprobeconfigimpl.WithFleetPoliciesDirPath(""),
		)),
		fx.Supply(pidimpl.NewParams("")),
		getSharedFxOption(),
	)
	overrides := fx.Options(
		fx.Replace(ipcComp),
		fx.Replace(remoteagent.Component(&mockRAR{})),
	)
	opts := fx.Options(baseOpts, overrides)

	t.Run("startup_completes_after_snapshot", func(t *testing.T) {
		done := make(chan error, 1)
		go func() {
			done <- fxutil.OneShot(run, opts)
		}()

		// Startup should still be blocking (no snapshot yet).
		select {
		case err := <-done:
			t.Fatalf("run completed before snapshot was sent: %v", err)
		case <-time.After(500 * time.Millisecond):
			// Good: still blocking.
		}

		// Send snapshot so WaitReady unblocks.
		events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{
				Snapshot: &pb.ConfigSnapshot{
					SequenceId: 1,
					Settings: []*pb.ConfigSetting{
						{Key: "test.key", Value: mustNewValue(t, "ok")},
					},
				},
			},
		}

		// run() should complete (startSystemProbe returns ErrNotEnabled then 5s sleep).
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(30 * time.Second):
			t.Fatal("run did not complete after sending snapshot")
		}
	})
}
