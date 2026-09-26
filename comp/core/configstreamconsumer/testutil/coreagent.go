// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package testutil provides a fake core agent that satisfies the configstream consumer,
// so tests that build a remote agent's fx graph can run with configstream enabled.
package testutil

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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

const authToken = "configstream-testutil-auth-token"

// FakeCoreAgent serves the three AgentSecure RPCs the configstream consumer needs.
type FakeCoreAgent struct {
	pb.UnimplementedAgentSecureServer

	// Events carries snapshots and updates pushed by the test. Buffered, so a test can queue
	// one before the consumer opens its stream.
	Events chan *pb.ConfigEvent

	dir      string
	server   *grpc.Server
	listener net.Listener
	opts     options

	closeOnce sync.Once

	mu                sync.Mutex
	sessionID         string
	pendingSessionIDs []string
	evicted           map[string]bool
	registerCount     int
	refreshCount      int
}

type options struct {
	sessionIDs          []string
	refreshIntervalSecs uint32
	snapshotOnConnect   bool
	snapshotSettings    map[string]any
}

// Option configures a FakeCoreAgent.
type Option func(*options)

// WithSessionIDs hands out session IDs in order, one per registration, reusing the last.
func WithSessionIDs(ids ...string) Option {
	return func(o *options) { o.sessionIDs = ids }
}

// WithRefreshInterval sets the refresh interval advertised to the consumer at registration.
func WithRefreshInterval(secs uint32) Option {
	return func(o *options) { o.refreshIntervalSecs = secs }
}

// WithSnapshotSettings overrides the settings carried by the automatic snapshot.
func WithSnapshotSettings(settings map[string]any) Option {
	return func(o *options) { o.snapshotSettings = settings }
}

// WithoutSnapshotOnConnect leaves the test to push events through Events. The consumer then
// blocks until its ReadyTimeout, so only use this to exercise that path.
func WithoutSnapshotOnConnect() Option {
	return func(o *options) { o.snapshotOnConnect = false }
}

// StartFakeCoreAgent writes the IPC credentials into dir and serves AgentSecure over TLS on
// 127.0.0.1, stopping via t.Cleanup.
func StartFakeCoreAgent(t *testing.T, dir string, opt ...Option) *FakeCoreAgent {
	t.Helper()

	opts := options{
		refreshIntervalSecs: 60,
		snapshotOnConnect:   true,
	}
	for _, o := range opt {
		o(&opts)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "auth_token"), []byte(authToken), 0600))
	serverTLS := generateIPCCert(t, filepath.Join(dir, "ipc_cert.pem"))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	f := &FakeCoreAgent{
		Events:    make(chan *pb.ConfigEvent, 16),
		dir:       dir,
		listener:  listener,
		opts:      opts,
		sessionID: "testutil-session-id",
		evicted:   map[string]bool{},
	}
	if len(opts.sessionIDs) > 0 {
		f.pendingSessionIDs = opts.sessionIDs
	}

	f.server = grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	pb.RegisterAgentSecureServer(f.server, f)
	go func() { _ = f.server.Serve(listener) }()

	t.Cleanup(f.Close)
	return f
}

// Close stops the server and releases the listener.
func (f *FakeCoreAgent) Close() {
	f.closeOnce.Do(func() {
		close(f.Events)
		f.server.Stop()
		_ = f.listener.Close()
	})
}

// Addr is the host:port the consumer should dial.
func (f *FakeCoreAgent) Addr() string { return f.listener.Addr().String() }

// Port is the port the consumer should dial, for a config file's cmd_port.
func (f *FakeCoreAgent) Port() int { return f.listener.Addr().(*net.TCPAddr).Port }

// AuthTokenPath is the auth token this fake wrote, for a config file's auth_token_file_path.
func (f *FakeCoreAgent) AuthTokenPath() string { return filepath.Join(f.dir, "auth_token") }

// IPCCertPath is the IPC certificate this fake wrote, for a config file's ipc_cert_file_path.
func (f *FakeCoreAgent) IPCCertPath() string { return filepath.Join(f.dir, "ipc_cert.pem") }

// ConfigYAML is the datadog.yaml fragment a remote agent needs to reach this fake.
func (f *FakeCoreAgent) ConfigYAML() string {
	host, port, err := net.SplitHostPort(f.Addr())
	if err != nil {
		panic(fmt.Sprintf("configstream testutil: malformed listener address %q: %v", f.Addr(), err))
	}
	return fmt.Sprintf(`cmd_host: %s
cmd_port: %s
auth_token_file_path: %s
ipc_cert_file_path: %s
remote_agent:
  registry:
    enabled: true
  configstream:
    consumer:
      enabled: true
`, host, port, f.AuthTokenPath(), f.IPCCertPath())
}

// Counts reports how many registrations and refreshes the fake has served.
func (f *FakeCoreAgent) Counts() (registers, refreshes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registerCount, f.refreshCount
}

// SessionID is the session the fake currently considers valid.
func (f *FakeCoreAgent) SessionID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessionID
}

// Evict marks a session as reaped, so the next refresh and stream are rejected.
func (f *FakeCoreAgent) Evict(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evicted[sessionID] = true
}

// Snapshot builds a snapshot event from a settings map, all attributed to the file source.
func Snapshot(t *testing.T, seqID int32, settings map[string]any) *pb.ConfigEvent {
	t.Helper()
	return &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: seqID,
				Settings:   configSettings(t, settings),
			},
		},
	}
}

func configSettings(t *testing.T, settings map[string]any) []*pb.ConfigSetting {
	t.Helper()
	out := make([]*pb.ConfigSetting, 0, len(settings))
	for key, raw := range settings {
		value, err := structpb.NewValue(raw)
		require.NoError(t, err, "setting %q is not representable as a protobuf value", key)
		out = append(out, &pb.ConfigSetting{Key: key, Value: value, Source: "file"})
	}
	return out
}

// RegisterRemoteAgent implements the registration the consumer performs before streaming.
func (f *FakeCoreAgent) RegisterRemoteAgent(_ context.Context, _ *pb.RegisterRemoteAgentRequest) (*pb.RegisterRemoteAgentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.registerCount++
	if len(f.pendingSessionIDs) > 0 {
		f.sessionID = f.pendingSessionIDs[0]
		f.pendingSessionIDs = f.pendingSessionIDs[1:]
	}
	return &pb.RegisterRemoteAgentResponse{
		SessionId:                      f.sessionID,
		RecommendedRefreshIntervalSecs: f.opts.refreshIntervalSecs,
	}, nil
}

// RefreshRemoteAgent keeps the session alive, reporting an unknown one as NotFound.
func (f *FakeCoreAgent) RefreshRemoteAgent(_ context.Context, req *pb.RefreshRemoteAgentRequest) (*pb.RefreshRemoteAgentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.refreshCount++
	if req.SessionId != f.sessionID || f.evicted[req.SessionId] {
		return nil, status.Errorf(codes.NotFound, "session_id %q not found", req.SessionId)
	}
	return &pb.RefreshRemoteAgentResponse{}, nil
}

// StreamConfigEvents answers with a snapshot (unless disabled) and then forwards Events.
func (f *FakeCoreAgent) StreamConfigEvents(_ *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing gRPC metadata")
	}
	// Matches the real server, which rejects an unknown session with PermissionDenied.
	if got := md.Get("session_id"); len(got) == 0 || !f.sessionValid(got[0]) {
		return status.Errorf(codes.PermissionDenied, "session_id %v not found", got)
	}

	if f.opts.snapshotOnConnect {
		snapshot := &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{
				Snapshot: &pb.ConfigSnapshot{
					SequenceId: 1,
					Settings:   f.snapshotSettings(),
				},
			},
		}
		if err := stream.Send(snapshot); err != nil {
			return err
		}
	}

	for event := range f.Events {
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

// snapshotSettings takes no *testing.T: it runs on the server goroutine, where a failed
// require would not be attributable.
func (f *FakeCoreAgent) snapshotSettings() []*pb.ConfigSetting {
	out := make([]*pb.ConfigSetting, 0, len(f.opts.snapshotSettings))
	for key, raw := range f.opts.snapshotSettings {
		value, err := structpb.NewValue(raw)
		if err != nil {
			continue
		}
		out = append(out, &pb.ConfigSetting{Key: key, Value: value, Source: "file"})
	}
	return out
}

func (f *FakeCoreAgent) sessionValid(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return id == f.sessionID && !f.evicted[id]
}

// generateIPCCert writes a self-signed cert+key PEM for 127.0.0.1 and returns its TLS config.
func generateIPCCert(t *testing.T, certPath string) *tls.Config {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "configstream-testutil"},
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

// AuthToken is the token this package writes, for tests that authenticate by hand.
func AuthToken() string { return authToken }
