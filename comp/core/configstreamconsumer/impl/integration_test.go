// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package configstreamconsumerimpl_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
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
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockCoreAgent is a minimal AgentSecure stub that serves RegisterRemoteAgent and
// StreamConfigEvents. Events are fed through the channel.
type mockCoreAgent struct {
	pb.UnimplementedAgentSecureServer
	events    chan *pb.ConfigEvent
	closeOnce sync.Once

	mu        sync.Mutex
	sessionID string
	// pendingSessionIDs are handed out in order; the last one is reused once drained.
	pendingSessionIDs   []string
	refreshIntervalSecs uint32
	registerCount       int
	refreshCount        int
	// evicted holds session IDs the RAR reaper has already dropped.
	evicted map[string]bool
	// refreshErrs are transport-level errors returned by the next refreshes, in order.
	refreshErrs []codes.Code
}

func (m *mockCoreAgent) currentSessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

func (m *mockCoreAgent) sessionValid(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return id == m.sessionID && !m.evicted[id]
}

func (m *mockCoreAgent) counts() (registers, refreshes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registerCount, m.refreshCount
}

func (m *mockCoreAgent) RegisterRemoteAgent(_ context.Context, _ *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.registerCount++
	if len(m.pendingSessionIDs) > 0 {
		m.sessionID = m.pendingSessionIDs[0]
		m.pendingSessionIDs = m.pendingSessionIDs[1:]
	}
	return &pb.RegisterRemoteAgentResponse{
		SessionId:                      m.sessionID,
		RecommendedRefreshIntervalSecs: m.refreshIntervalSecs,
	}, nil
}

func (m *mockCoreAgent) RefreshRemoteAgent(_ context.Context, req *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.refreshCount++
	if len(m.refreshErrs) > 0 {
		code := m.refreshErrs[0]
		m.refreshErrs = m.refreshErrs[1:]
		return nil, status.Errorf(code, "injected transport failure")
	}
	// Matches the real registry, which reports an unknown session as NotFound.
	if req.SessionId != m.sessionID || m.evicted[req.SessionId] {
		return nil, status.Errorf(codes.NotFound, "session_id %q not found", req.SessionId)
	}
	return &pb.RefreshRemoteAgentResponse{}, nil
}

func (m *mockCoreAgent) StreamConfigEvents(_ *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing gRPC metadata")
	}
	// Matches the real server, which rejects an unknown session with PermissionDenied.
	if got := md.Get("session_id"); len(got) == 0 || !m.sessionValid(got[0]) {
		return status.Errorf(codes.PermissionDenied, "session_id %v not found", got)
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

// generateTestIPCCert writes a self-signed cert+key PEM (valid for 127.0.0.1) to
// certPath and returns the server TLS config derived from it.
func generateTestIPCCert(t *testing.T, certPath string) *tls.Config {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ipc"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	require.NoError(t, os.WriteFile(certPath, append(certPEM, keyPEM...), 0600))

	_, serverTLS, err := cert.GetTLSConfigFromCert(certPEM, keyPEM)
	require.NoError(t, err)
	return serverTLS
}

// setupFakeCoreAgent writes auth_token and ipc_cert.pem to dir and starts a gRPC
// server backed by that cert. Returns the listener address and a cleanup func.
func setupFakeCoreAgent(t *testing.T, dir string) (addr string, mock *mockCoreAgent, cleanup func()) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "auth_token"), []byte("test-auth-token"), 0600))
	serverTLS := generateTestIPCCert(t, filepath.Join(dir, "ipc_cert.pem"))

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

// TestRunBlocksUntilConfigStreamSnapshot verifies that for each agent the consumer
// blocks fxutil.OneShot until the first snapshot arrives from the mock core.
func TestRunBlocksUntilConfigStreamSnapshot(t *testing.T) {
	agents := []string{"trace-agent", "process-agent", "security-agent", "system-probe", "host-profiler"}
	for _, agentName := range agents {
		t.Run(agentName, func(t *testing.T) {
			// Rebuild the env var layer on each lookup so a stale schema from other
			// tests doesn't shadow SourceFile values written by SeedGlobalBuilder.
			configstreambootstrap.UseDynamicSchema(t)
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
				fx.Supply(configstreamconsumer.NewParams(agentName, datadogPath, configstreamconsumer.WithReadyTimeout(10*time.Second))),
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
		})
	}
}

// TestRunNoopWhenConfigstreamDisabled verifies that a disabled consumer lets
// fxutil.OneShot complete immediately without blocking.
func TestRunNoopWhenConfigstreamDisabled(t *testing.T) {
	configstreambootstrap.UseDynamicSchema(t)
	dir := t.TempDir()
	datadogPath := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(datadogPath, []byte(""), 0600))

	// Ensure no env override re-enables.
	t.Setenv("DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED", "false")

	opts := fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		telemetryfx.Module(),
		fx.Supply(configstreamconsumer.NewParams("test-agent", datadogPath)),
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

// startConsumer returns a channel that fires once the initial snapshot has been applied.
func startConsumer(t *testing.T, dir, addr string, readyTimeout time.Duration, run func() error) <-chan error {
	t.Helper()

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
		fx.Supply(configstreamconsumer.NewParams("trace-agent", datadogPath, configstreamconsumer.WithReadyTimeout(readyTimeout))),
		configstreamconsumerfx.Module(),
	)

	done := make(chan error, 1)
	go func() { done <- fxutil.OneShot(func(_ configstreamconsumer.Component) error { return run() }, opts) }()
	return done
}

// queueSnapshot buffers a snapshot so it is delivered whenever the stream opens.
func queueSnapshot(t *testing.T, mock *mockCoreAgent, seqID int32) {
	t.Helper()
	mock.events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: seqID,
				Settings:   []*pb.ConfigSetting{{Key: "test.key", Value: mustNewValue(t, "test-value")}},
			},
		},
	}
}

// Recovery driven purely by the stream's PermissionDenied: the refresh interval is set far
// beyond the test window so the session loop cannot be what rescues the consumer.
func TestConsumerReregistersAfterStreamRejectsSession(t *testing.T) {
	configstreambootstrap.UseDynamicSchema(t)
	dir := t.TempDir()
	addr, mock, cleanup := setupFakeCoreAgent(t, dir)
	defer cleanup()

	mock.pendingSessionIDs = []string{"stale-session", "fresh-session"}
	mock.evicted = map[string]bool{"stale-session": true}
	mock.refreshIntervalSecs = 60

	queueSnapshot(t, mock, 1)
	done := startConsumer(t, dir, addr, 30*time.Second, func() error { return nil })

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(60 * time.Second):
		t.Fatal("consumer never recovered from the rejected session")
	}

	registers, refreshes := mock.counts()
	require.GreaterOrEqual(t, registers, 2, "consumer should have re-registered after PermissionDenied")
	require.Zero(t, refreshes, "refresh interval was too long to have driven recovery")
	require.Equal(t, "fresh-session", mock.currentSessionID())
}

// The other half: an open stream is not enough to keep a session out of the RAR reaper.
func TestConsumerRefreshesSession(t *testing.T) {
	configstreambootstrap.UseDynamicSchema(t)
	dir := t.TempDir()
	addr, mock, cleanup := setupFakeCoreAgent(t, dir)
	defer cleanup()

	mock.refreshIntervalSecs = 1

	queueSnapshot(t, mock, 1)
	done := startConsumer(t, dir, addr, 30*time.Second, func() error {
		require.Eventually(t, func() bool {
			_, refreshes := mock.counts()
			return refreshes >= 2
		}, 15*time.Second, 100*time.Millisecond, "session was never refreshed, so RAR would reap it")
		return nil
	})

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(60 * time.Second):
		t.Fatal("consumer did not finish")
	}

	registers, _ := mock.counts()
	require.Equal(t, 1, registers, "a refreshed session should never need re-registration")
}

// A refresh failing on its own connection says nothing about the session, which the core agent
// keeps alive through the stream. Dropping it there would strand the old registration in RAR
// while the replacement takes over, accumulating a duplicate agent per transient failure.
func TestConsumerKeepsSessionAfterTransientRefreshFailure(t *testing.T) {
	configstreambootstrap.UseDynamicSchema(t)
	dir := t.TempDir()
	addr, mock, cleanup := setupFakeCoreAgent(t, dir)
	defer cleanup()

	mock.refreshIntervalSecs = 1
	mock.refreshErrs = []codes.Code{codes.Unavailable, codes.DeadlineExceeded}

	queueSnapshot(t, mock, 1)
	done := startConsumer(t, dir, addr, 30*time.Second, func() error {
		require.Eventually(t, func() bool {
			_, refreshes := mock.counts()
			return refreshes >= 4
		}, 30*time.Second, 100*time.Millisecond, "refreshes did not resume after the transient failures")
		return nil
	})

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(60 * time.Second):
		t.Fatal("consumer did not finish")
	}

	registers, _ := mock.counts()
	require.Equal(t, 1, registers, "a transient refresh error must not re-register the session")
	require.Equal(t, "test-session-id", mock.currentSessionID(), "the original session must survive")
}

// TestUnsetUpdateRestoresShadowedValue covers the scenario from the review thread: a value supplied by
// an env var, shadowed by an agent-runtime write, then unset. Snapshots only carry the core agent's
// merged view, and the consumer's own env layer is deliberately disabled, so it has no lower layer to
// fall back to -- the removal has to supply the resolution or the key silently reverts to default.
func TestUnsetUpdateRestoresShadowedValue(t *testing.T) {
	configstreambootstrap.UseDynamicSchema(t)
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
		fx.Supply(configstreamconsumer.NewParams("trace-agent", datadogPath, configstreamconsumer.WithReadyTimeout(10*time.Second))),
		configstreamconsumerfx.Module(),
	)

	testRun := func(_ configstreamconsumer.Component) error {
		cfg := configstreambootstrap.Config()
		require.Equal(t, "from-runtime", cfg.Get("test.key"))
		require.Equal(t, model.SourceAgentRuntime, cfg.GetSource("test.key"))

		// SourceEnvVar is why this cannot be applied as an ordinary update: Set rejects writes to
		// the env layer, and the consumer's local one is cleared at startup.
		mock.events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Update{
				Update: &pb.ConfigUpdate{
					SequenceId: 2,
					Setting: &pb.ConfigSetting{
						Key:         "test.key",
						Value:       mustNewValue(t, "from-env"),
						Source:      string(model.SourceEnvVar),
						UnsetSource: string(model.SourceAgentRuntime),
					},
				},
			},
		}

		require.Eventually(t, func() bool {
			return cfg.Get("test.key") == "from-env"
		}, 10*time.Second, 20*time.Millisecond, "key never fell back to the shadowed env value")
		require.Equal(t, model.SourceEnvVar, cfg.GetSource("test.key"), "the layer the core agent resolves from")
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- fxutil.OneShot(testRun, opts) }()

	// One entry per key, as createConfigSnapshot sends: the resolved value tagged with the winning
	// layer. The shadowed env value is never transmitted.
	mock.events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings: []*pb.ConfigSetting{
					{Key: "test.key", Value: mustNewValue(t, "from-runtime"), Source: string(model.SourceAgentRuntime)},
				},
			},
		},
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("OneShot did not complete")
	}
}

// A layer cleared while this client was disconnected is only visible as its absence from the
// reconnect snapshot, so the snapshot has to retract it rather than merely add what it does claim.
func TestSnapshotRetractsLayerRemovedWhileDisconnected(t *testing.T) {
	configstreambootstrap.UseDynamicSchema(t)
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
		fx.Supply(configstreamconsumer.NewParams("trace-agent", datadogPath, configstreamconsumer.WithReadyTimeout(10*time.Second))),
		configstreamconsumerfx.Module(),
	)

	testRun := func(_ configstreamconsumer.Component) error {
		cfg := configstreambootstrap.Config()
		require.Equal(t, "from-file", cfg.Get("test.key"))

		mock.events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Update{
				Update: &pb.ConfigUpdate{
					SequenceId: 2,
					Setting: &pb.ConfigSetting{
						Key:    "test.key",
						Value:  mustNewValue(t, "from-cli"),
						Source: string(model.SourceCLI),
					},
				},
			},
		}
		require.Eventually(t, func() bool {
			return cfg.Get("test.key") == "from-cli"
		}, 10*time.Second, 20*time.Millisecond, "the cli layer never took effect")

		// The core agent cleared the cli layer while this client was away. The resynchronization
		// snapshot reports the file layer as the winner and says nothing about cli.
		mock.events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{
				Snapshot: &pb.ConfigSnapshot{
					SequenceId: 3,
					Settings: []*pb.ConfigSetting{
						{Key: "test.key", Value: mustNewValue(t, "from-file"), Source: string(model.SourceFile)},
					},
				},
			},
		}
		require.Eventually(t, func() bool {
			return cfg.Get("test.key") == "from-file"
		}, 10*time.Second, 20*time.Millisecond, "the stale cli layer outlived the snapshot that dropped it")
		require.Equal(t, model.SourceFile, cfg.GetSource("test.key"))
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- fxutil.OneShot(testRun, opts) }()

	mock.events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings: []*pb.ConfigSetting{
					{Key: "test.key", Value: mustNewValue(t, "from-file"), Source: string(model.SourceFile)},
				},
			},
		},
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("OneShot did not complete")
	}
}
