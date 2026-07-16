// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// fakeExecutor stands in for the execute-one-action core so tests exercise only gRPC plumbing.
type fakeExecutor struct {
	prepared   *runners.PreparedWorkflowTask
	prepareErr error
	output     interface{}
	runErr     error
	runGate    chan struct{} // when non-nil, blocks RunPrepared until closed, to hold an action in-flight

	gotRawTask []byte
}

func (f *fakeExecutor) PrepareTask(_ context.Context, task *types.Task) (*runners.PreparedWorkflowTask, *types.Task, error) {
	f.gotRawTask = task.Raw
	if f.prepareErr != nil {
		return nil, task, f.prepareErr
	}
	return f.prepared, nil, nil
}

func (f *fakeExecutor) RunPrepared(ctx context.Context, _ *runners.PreparedWorkflowTask) (interface{}, error) {
	if f.runGate != nil {
		select {
		case <-f.runGate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.output, f.runErr
}

// shortSocketPath returns a socket path short enough for macOS's ~104-byte sun_path limit.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "par")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "e.sock")
}

// startTestServer serves a real executor on a socket and returns a connected client.
func startTestServer(t *testing.T, srv *Server) pb.ExecutorClient {
	t.Helper()

	socketPath := shortSocketPath(t)
	lis, err := Listen(socketPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, lis, srv, ServeOptions{}) }()

	conn, err := grpc.NewClient(
		"passthrough:///"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) {
			return Dial(dialCtx, socketPath, 2*time.Second)
		}),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		cancel()
		<-served
	})

	return pb.NewExecutorClient(conn)
}

// runAction drives RunAction to completion and returns the terminal ActionResult.
func runAction(t *testing.T, client pb.ExecutorClient, taskBytes []byte) *pb.ActionResult {
	t.Helper()

	stream, err := client.RunAction(context.Background(), &pb.RunActionRequest{Task: taskBytes})
	require.NoError(t, err)

	var result *pb.ActionResult
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if r := resp.GetResult(); r != nil {
			result = r
		}
	}
	require.NotNil(t, result, "RunAction stream ended without a terminal ActionResult")
	return result
}

func TestServeRunActionStreamsOutputAndForwardsRawTask(t *testing.T) {
	fake := &fakeExecutor{
		prepared: &runners.PreparedWorkflowTask{Task: &types.Task{}},
		output:   map[string]interface{}{"greeting": "hello"},
	}
	srv := NewServer(fake, "test-version")
	srv.SetReady(true)

	client := startTestServer(t, srv)

	rawTask := []byte(`{"data":{"id":"task-1","attributes":{"job_id":"job-1"}}}`)
	result := runAction(t, client, rawTask)

	assert.Equal(t, rawTask, fake.gotRawTask)

	require.NotNil(t, result.GetOutput(), "expected a success output, got error: %v", result.GetError())
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(result.GetOutput(), &got))
	assert.Equal(t, map[string]interface{}{"greeting": "hello"}, got)
}

func TestServeRunActionReturnsStructuredErrorOnFailure(t *testing.T) {
	fake := &fakeExecutor{
		prepareErr: util.NewPARError(
			aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR,
			errors.New("bad signature"),
		),
	}
	srv := NewServer(fake, "test-version")
	srv.SetReady(true)

	client := startTestServer(t, srv)

	result := runAction(t, client, []byte(`{"data":{"id":"task-1"}}`))

	require.Nil(t, result.GetOutput())
	require.NotNil(t, result.GetError())
	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, result.GetError().GetErrorCode())
	assert.Equal(t, "bad signature", result.GetError().GetMessage())
}

func TestServeRunActionMapsStructuredErrorCodesOverTheWire(t *testing.T) {
	// Each structured error code must reach the control plane intact; plain errors → INTERNAL_ERROR.
	cases := []struct {
		name     string
		err      error
		wantCode aperrorpb.ActionPlatformErrorCode
	}{
		{"bad signature", util.NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR, errors.New("bad sig")), aperrorpb.ActionPlatformErrorCode_SIGNATURE_ERROR},
		{"signing key not found", util.NewPARError(aperrorpb.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND, errors.New("no key")), aperrorpb.ActionPlatformErrorCode_SIGNATURE_KEY_NOT_FOUND},
		{"expired task", util.NewPARError(aperrorpb.ActionPlatformErrorCode_EXPIRED_TASK, errors.New("expired")), aperrorpb.ActionPlatformErrorCode_EXPIRED_TASK},
		{"disallowed action", util.DefaultActionError(errors.New("action not allowed")), aperrorpb.ActionPlatformErrorCode_ACTION_ERROR},
		{"unresolvable credential", errors.New("could not resolve connection"), aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeExecutor{prepareErr: tc.err}
			if tc.wantCode == aperrorpb.ActionPlatformErrorCode_ACTION_ERROR {
				fake = &fakeExecutor{
					prepared: &runners.PreparedWorkflowTask{Task: &types.Task{}},
					runErr:   tc.err,
				}
			}
			srv := NewServer(fake, "test-version")
			srv.SetReady(true)
			client := startTestServer(t, srv)

			result := runAction(t, client, []byte(`{"data":{"id":"task-1"}}`))
			require.NotNil(t, result.GetError())
			assert.Equal(t, tc.wantCode, result.GetError().GetErrorCode())
		})
	}
}

func TestServeRunActionWrapsPlainRunErrorAsActionError(t *testing.T) {
	fake := &fakeExecutor{
		prepared: &runners.PreparedWorkflowTask{Task: &types.Task{}},
		runErr:   errors.New("boom"),
	}
	srv := NewServer(fake, "test-version")
	srv.SetReady(true)

	client := startTestServer(t, srv)

	result := runAction(t, client, []byte(`{"data":{"id":"task-1"}}`))

	require.NotNil(t, result.GetError())
	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, result.GetError().GetErrorCode())
}

func TestServeRunActionRejectedWhenNotReady(t *testing.T) {
	fake := &fakeExecutor{prepared: &runners.PreparedWorkflowTask{Task: &types.Task{}}}
	srv := NewServer(fake, "test-version")

	client := startTestServer(t, srv)

	result := runAction(t, client, []byte(`{"data":{"id":"task-1"}}`))

	require.NotNil(t, result.GetError())
	assert.Equal(t, aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, result.GetError().GetErrorCode())
	assert.Nil(t, fake.gotRawTask, "action must not be dispatched to the core when not ready")
}

func TestHealthReportsReadinessAndVersion(t *testing.T) {
	srv := NewServer(&fakeExecutor{}, "test-version")
	client := startTestServer(t, srv)

	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	require.NoError(t, err)
	assert.False(t, resp.GetReady())
	assert.Equal(t, "test-version", resp.GetVersion())

	srv.SetReady(true)
	resp, err = client.Health(context.Background(), &pb.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.GetReady())
}

// dialTestExecutor connects a client to an executor already listening on socketPath.
func dialTestExecutor(t *testing.T, socketPath string) pb.ExecutorClient {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough:///"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) {
			return Dial(dialCtx, socketPath, 2*time.Second)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewExecutorClient(conn)
}

func TestServeOrphanSelfExitsWhenIdle(t *testing.T) {
	srv := NewServer(&fakeExecutor{}, "test-version")
	srv.SetReady(true)

	socketPath := shortSocketPath(t)
	lis, err := Listen(socketPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- Serve(ctx, lis, srv, ServeOptions{
			IdleShutdownTimeout: 40 * time.Millisecond,
		})
	}()

	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("orphaned executor did not self-exit")
	}
}

func TestServeDrainsInFlightActionBeforeExit(t *testing.T) {
	gate := make(chan struct{})
	fake := &fakeExecutor{
		prepared: &runners.PreparedWorkflowTask{Task: &types.Task{}},
		output:   map[string]interface{}{"drained": true},
		runGate:  gate,
	}
	srv := NewServer(fake, "test-version")
	srv.SetReady(true)

	socketPath := shortSocketPath(t)
	lis, err := Listen(socketPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, lis, srv, ServeOptions{DrainTimeout: 2 * time.Second}) }()

	client := dialTestExecutor(t, socketPath)
	stream, err := client.RunAction(context.Background(), &pb.RunActionRequest{Task: []byte(`{"data":{"id":"t"}}`)})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return srv.active.Load() == 1 }, time.Second, 5*time.Millisecond)

	// Stop mid-run, then let the action finish: graceful drain must still deliver its output.
	cancel()
	time.Sleep(50 * time.Millisecond)
	close(gate)

	var result *pb.ActionResult
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if r := resp.GetResult(); r != nil {
			result = r
		}
	}
	require.NotNil(t, result)
	require.NotNil(t, result.GetOutput(), "drained action should complete with its output")

	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return after drain")
	}
}

func newTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "par-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}

func newLeafCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func TestServeMTLSRequiresValidClientCert(t *testing.T) {
	ca, caKey := newTestCA(t)
	caPool := x509.NewCertPool()
	caPool.AddCert(ca)
	serverCert := newLeafCert(t, ca, caKey, "localhost", x509.ExtKeyUsageServerAuth)
	clientCert := newLeafCert(t, ca, caKey, "par-control", x509.ExtKeyUsageClientAuth)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	srv := NewServer(&fakeExecutor{}, "test-version")
	srv.SetReady(true)

	socketPath := shortSocketPath(t)
	lis, err := Listen(socketPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- Serve(ctx, lis, srv, ServeOptions{}, grpc.Creds(credentials.NewTLS(serverTLS)))
	}()
	t.Cleanup(func() { cancel(); <-served })

	dial := func(tlsCfg *tls.Config) pb.ExecutorClient {
		conn, err := grpc.NewClient(
			"passthrough:///"+socketPath,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
			grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) {
				return Dial(dialCtx, socketPath, 2*time.Second)
			}),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		return pb.NewExecutorClient(conn)
	}

	authed := dial(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   "localhost",
	})
	_, err = authed.Health(context.Background(), &pb.HealthRequest{})
	require.NoError(t, err, "client with a valid IPC-style cert should be accepted")

	anon := dial(&tls.Config{RootCAs: caPool, ServerName: "localhost"})
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shortCancel()
	_, err = anon.Health(shortCtx, &pb.HealthRequest{})
	require.Error(t, err, "client without a valid cert must be rejected")
}
