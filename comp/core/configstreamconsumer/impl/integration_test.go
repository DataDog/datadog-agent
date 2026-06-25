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
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockCoreAgent is a minimal AgentSecure stub that serves RegisterRemoteAgent and
// StreamConfigEvents. Events are fed through the channel.
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
	agents := []string{"trace-agent", "process-agent", "security-agent", "system-probe"}
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
