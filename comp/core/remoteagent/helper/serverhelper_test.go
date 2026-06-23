// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package helper

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// shortUDSDir returns a short-path temp directory suitable for UDS sockets.
// macOS limits sun_path to 104 bytes; the default t.TempDir() on macOS
// produces paths that easily blow past that with long test names.
func shortUDSDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "rar-uds-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestBuildRemoteAgentListener(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, err := buildRemoteAgentListener("")
		require.Error(t, err)
	})

	t.Run("https", func(t *testing.T) {
		ral, err := buildRemoteAgentListener("https://127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = ral.listener.Close() })

		assert.Empty(t, ral.cleanupSocketPath)
		assert.True(t, strings.HasPrefix(ral.apiEndpointURI, "https://127.0.0.1:"), "got %q", ral.apiEndpointURI)
		assert.NotEqual(t, "https://127.0.0.1:0", ral.apiEndpointURI, "random port should be resolved")
	})

	t.Run("unix", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("UDS not supported on Windows")
		}
		socketPath := filepath.Join(shortUDSDir(t), "ra.sock")
		listenURI := "unix://" + socketPath

		ral, err := buildRemoteAgentListener(listenURI)
		require.NoError(t, err)
		t.Cleanup(func() { _ = ral.listener.Close() })

		assert.Equal(t, listenURI, ral.apiEndpointURI, "unix URI should round-trip into the advertised URI")
		assert.Equal(t, socketPath, ral.cleanupSocketPath)

		info, err := os.Stat(socketPath)
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSocket != 0, "expected a socket at %q", socketPath)
		assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "socket should be owner-only")
	})

	t.Run("unix_stale", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("UDS not supported on Windows")
		}
		socketPath := filepath.Join(shortUDSDir(t), "s.sock")

		// Pre-create a socket and close it without unlinking the on-disk file, to
		// simulate a process that crashed and left a stale socket inode behind.
		first, err := net.Listen("unix", socketPath)
		require.NoError(t, err)
		first.(*net.UnixListener).SetUnlinkOnClose(false)
		require.NoError(t, first.Close())
		info, err := os.Stat(socketPath)
		require.NoError(t, err)
		require.NotZero(t, info.Mode()&os.ModeSocket, "precondition: stale socket file should exist on disk")

		ral, err := buildRemoteAgentListener("unix://" + socketPath)
		require.NoError(t, err)
		t.Cleanup(func() { _ = ral.listener.Close() })
		assert.Equal(t, socketPath, ral.cleanupSocketPath)
	})

	t.Run("unix_non_socket_file", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("UDS not supported on Windows")
		}
		filePath := filepath.Join(shortUDSDir(t), "f")
		require.NoError(t, os.WriteFile(filePath, []byte("hi"), 0600))

		_, err := buildRemoteAgentListener("unix://" + filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a socket")
	})

	t.Run("http_rejected", func(t *testing.T) {
		_, err := buildRemoteAgentListener("http://127.0.0.1:0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "http:// scheme is not supported")
	})

	t.Run("missing_scheme", func(t *testing.T) {
		// listenURI must be either empty (legacy) or scheme-prefixed; a bare "host:port" is rejected
		// to keep callsites unambiguous.
		_, err := buildRemoteAgentListener("127.0.0.1:0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing scheme")
	})

	t.Run("unsupported_scheme", func(t *testing.T) {
		_, err := buildRemoteAgentListener("vsock://2:50051")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported remote agent listen URI scheme")
	})
}

// TestNoSessionIDReturnsError tests that requests to the remote agent server
// return errors when no session ID is set (i.e., before registration completes)
func TestNoSessionIDReturnsError(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	configComp := configmock.New(t)

	// Create a mock core agent server that never responds to registration
	mockCoreAgent := newMockCoreAgentServer(t, ipcComp, func(ctx context.Context, _ *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
		// Block forever to prevent registration from completing
		<-ctx.Done()
		return nil, ctx.Err()
	}, nil)
	defer mockCoreAgent.stop()

	// Create the remote agent server
	server, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		mockCoreAgent.address,
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)

	// Register a test service
	pbcore.RegisterStatusProviderServer(server.GetGRPCServer(), &mockStatusProvider{})

	// Start the server (impls call this explicitly after registering services).
	server.Start()
	defer lc.Stop(context.Background())

	// Create a client to the remote agent server
	conn, err := grpc.NewClient(
		server.listener.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(ipcComp.GetAuthToken())),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pbcore.NewStatusProviderClient(conn)

	// Try to call the status service - should fail because no session ID is set
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.GetStatusDetails(ctx, &pbcore.GetStatusDetailsRequest{}, grpc.WaitForReady(true))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote agent is not registered yet")
}

// TestAuthTokenIsChecked tests that the auth_token is properly validated
// for incoming requests to the remote agent server
func TestAuthTokenIsChecked(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	configComp := configmock.New(t)

	// Create a mock core agent server
	mockCoreAgent := newMockCoreAgentServer(t, ipcComp, func(_ context.Context, _ *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
		return &pbcore.RegisterRemoteAgentResponse{
			SessionId:                      "test-session-id",
			RecommendedRefreshIntervalSecs: 60,
		}, nil
	}, nil)
	defer mockCoreAgent.stop()

	// Create the remote agent server
	server, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		mockCoreAgent.address,
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)

	// Register a test service
	pbcore.RegisterStatusProviderServer(server.GetGRPCServer(), &mockStatusProvider{})

	// Start the server (impls call this explicitly after registering services).
	server.Start()
	defer lc.Stop(context.Background())

	tests := []struct {
		name             string
		authToken        string
		shouldSucceed    bool
		expectedGRPCCode codes.Code
	}{
		{
			name:             "invalid auth token should fail",
			authToken:        "invalid-token",
			shouldSucceed:    false,
			expectedGRPCCode: codes.Unauthenticated,
		},
		{
			name:          "valid auth token should succeed",
			authToken:     ipcComp.GetAuthToken(), // will be set to valid token
			shouldSucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client with the specified auth token
			conn, err := grpc.NewClient(
				server.listener.Addr().String(),
				grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())),
				grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(tt.authToken)),
			)
			require.NoError(t, err)
			defer conn.Close()

			client := pbcore.NewStatusProviderClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			require.EventuallyWithT(t, func(c *assert.CollectT) {
				resp, err := client.GetStatusDetails(ctx, &pbcore.GetStatusDetailsRequest{}, grpc.WaitForReady(true))
				if tt.shouldSucceed {
					require.NoError(c, err)
					assert.NotNil(c, resp)
				} else {
					require.Error(c, err)
					st, ok := status.FromError(err)
					require.True(c, ok)
					assert.Equal(c, tt.expectedGRPCCode, st.Code())
				}
			}, 5*time.Second, 100*time.Millisecond)
		})
	}
}

// TestServerLifecycle tests that the server starts and stops correctly
// using compdef.NewTestLifecycle
func TestServerLifecycle(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	configComp := configmock.New(t)

	// Create a mock core agent server
	mockCoreAgent := newMockCoreAgentServer(t, ipcComp, func(_ context.Context, _ *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
		return &pbcore.RegisterRemoteAgentResponse{
			SessionId:                      "test-session-id",
			RecommendedRefreshIntervalSecs: 60,
		}, nil
	}, nil)
	defer mockCoreAgent.stop()

	// Create the remote agent server
	server, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		mockCoreAgent.address,
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)
	require.NotNil(t, server)

	// Register a test service
	pbcore.RegisterStatusProviderServer(server.GetGRPCServer(), &mockStatusProvider{})

	// Verify lifecycle hooks were registered (OnStop only; Start is called explicitly).
	lc.AssertHooksNumber(1)

	// Start the server (impls call this explicitly after registering services).
	server.Start()

	// Verify the server is running by checking if we can connect
	conn, err := grpc.NewClient(
		server.listener.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(ipcComp.GetAuthToken())),
	)
	require.NoError(t, err)
	defer conn.Close()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		client := pbcore.NewStatusProviderClient(conn)
		resp, err := client.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
		require.NoError(c, err)
		assert.NotNil(c, resp)
	}, 5*time.Second, 100*time.Millisecond)

	// Stop the server
	err = lc.Stop(context.Background())
	require.NoError(t, err)
}

// TestRegisteredServicesReported tests that services registered with the gRPC
// server are properly reported during registration with the core agent
func TestRegisteredServicesReported(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	configComp := configmock.New(t)

	var receivedServices []string
	var mu sync.Mutex

	// Create a mock core agent server that captures the registered services
	mockCoreAgent := newMockCoreAgentServer(t, ipcComp, func(_ context.Context, req *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
		mu.Lock()
		receivedServices = req.Services
		mu.Unlock()
		return &pbcore.RegisterRemoteAgentResponse{
			SessionId:                      "test-session-id",
			RecommendedRefreshIntervalSecs: 60,
		}, nil
	}, nil)
	defer mockCoreAgent.stop()

	// Create the remote agent server
	server, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		mockCoreAgent.address,
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)

	// Register multiple services
	pbcore.RegisterStatusProviderServer(server.GetGRPCServer(), &mockStatusProvider{})
	pbcore.RegisterFlareProviderServer(server.GetGRPCServer(), &mockFlareProvider{})
	pbcore.RegisterTelemetryProviderServer(server.GetGRPCServer(), &mockTelemetryProvider{})

	// Start the server (impls call this explicitly after registering services).
	server.Start()
	defer lc.Stop(context.Background())

	// Wait for registration to complete
	time.Sleep(600 * time.Millisecond)

	// Verify that all services were reported
	mu.Lock()
	services := receivedServices
	mu.Unlock()

	require.NotEmpty(t, services)
	assert.Contains(t, services, "datadog.remoteagent.status.v1.StatusProvider")
	assert.Contains(t, services, "datadog.remoteagent.flare.v1.FlareProvider")
	assert.Contains(t, services, "datadog.remoteagent.telemetry.v1.TelemetryProvider")
}

// TestRegistrationRefreshContention tests what happens when the core agent's
// registration/refresh endpoints hang or are slow to respond
func TestRegistrationRefreshContention(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	configComp := configmock.New(t)
	server, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		"test.invalid",
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)

	synctest.Test(t, func(t *testing.T) {
		syncTestRegistrationRefreshContention(t, lc, server)
	})
}

func syncTestRegistrationRefreshContention(t *testing.T, lc *compdef.TestLifecycle, server *UnimplementedRemoteAgentServer) {
	registerCallCount := 0
	refreshCallCount := 0
	registered := make(chan struct{})
	require.NoError(t, server.listener.Close())
	server.listener = bufconn.Listen(1 << 20)

	// Create a fake agent client where registration hangs the 2 first calls
	server.agentClient = &fakeRemoteAgentClient{
		func(ctx context.Context, _ *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
			registerCallCount++
			if registerCallCount > 2 {
				close(registered)
				return &pbcore.RegisterRemoteAgentResponse{
					SessionId:                      "uuid_session_id",
					RecommendedRefreshIntervalSecs: 1,
				}, nil
			}
			// Block forever to prevent registration from completing
			<-ctx.Done()
			return nil, ctx.Err()
		},
		func(_ context.Context, _ *pbcore.RefreshRemoteAgentRequest) (*pbcore.RefreshRemoteAgentResponse, error) {
			refreshCallCount++
			return &pbcore.RefreshRemoteAgentResponse{}, nil
		},
	}

	// Start the server (impls call this explicitly after registering services).
	server.Start()
	defer lc.Stop(context.Background())

	// Wait for registration to be retried after the first two timeouts.
	// The third attempt should succeed and bring the counter to 3.
	<-registered
	assert.Equal(t, 3, registerCallCount, "Registration should have been retried after timeout")

	// Now test refresh contention - the server should be registered by now
	// Wait a bit more to ensure refresh is called
	time.Sleep(1500 * time.Millisecond) // at midpoint between 1st and 2nd tick
	synctest.Wait()                     // for the bubble to quiesce

	// We should have exactly one refresh call by now
	assert.Equal(t, 1, refreshCallCount, "Refresh should have been called exactly once")
}

// TestSessionIDInResponseMetadata tests that the session ID is properly
// included in response metadata after successful registration
func TestSessionIDInResponseMetadata(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	configComp := configmock.New(t)

	expectedSessionID := "test-session-id-12345"

	// Create a mock core agent server
	mockCoreAgent := newMockCoreAgentServer(t, ipcComp, func(_ context.Context, _ *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
		return &pbcore.RegisterRemoteAgentResponse{
			SessionId:                      expectedSessionID,
			RecommendedRefreshIntervalSecs: 60,
		}, nil
	}, nil)
	defer mockCoreAgent.stop()

	// Create the remote agent server
	server, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		mockCoreAgent.address,
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)

	// Register a test service
	pbcore.RegisterStatusProviderServer(server.GetGRPCServer(), &mockStatusProvider{})

	// Start the server (impls call this explicitly after registering services).
	server.Start()
	defer lc.Stop(context.Background())

	// Create a client and make a request
	conn, err := grpc.NewClient(
		server.listener.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(ipcComp.GetAuthToken())),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pbcore.NewStatusProviderClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var header metadata.MD
		_, err := client.GetStatusDetails(ctx, &pbcore.GetStatusDetailsRequest{}, grpc.Header(&header), grpc.WaitForReady(true))
		require.NoError(c, err)
		// Verify session ID is in the response metadata
		sessionIDs := header.Get("session_id")
		require.Len(c, sessionIDs, 1)
		assert.Equal(c, expectedSessionID, sessionIDs[0])
	}, 5*time.Second, 100*time.Millisecond)
}

// Helper types and functions

// fakeRemoteAgentClient implements pbcore.RemoteAgentClient via callbacks, with no gRPC transport
// goroutines nor timers, safe to use inside a synctest bubble.
type fakeRemoteAgentClient struct {
	registerFunc func(context.Context, *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error)
	refreshFunc  func(context.Context, *pbcore.RefreshRemoteAgentRequest) (*pbcore.RefreshRemoteAgentResponse, error)
}

func (f *fakeRemoteAgentClient) RegisterRemoteAgent(ctx context.Context, req *pbcore.RegisterRemoteAgentRequest, _ ...grpc.CallOption) (*pbcore.RegisterRemoteAgentResponse, error) {
	return f.registerFunc(ctx, req)
}

func (f *fakeRemoteAgentClient) RefreshRemoteAgent(ctx context.Context, req *pbcore.RefreshRemoteAgentRequest, _ ...grpc.CallOption) (*pbcore.RefreshRemoteAgentResponse, error) {
	return f.refreshFunc(ctx, req)
}

func (f *fakeRemoteAgentClient) ReportRemoteAgentEvent(_ context.Context, _ *pbcore.ReportRemoteAgentEventRequest, _ ...grpc.CallOption) (*pbcore.ReportRemoteAgentEventResponse, error) {
	return &pbcore.ReportRemoteAgentEventResponse{}, nil
}

// mockStatusProvider implements a mock status provider
type mockStatusProvider struct {
	pbcore.UnimplementedStatusProviderServer
}

func (m *mockStatusProvider) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	return &pbcore.GetStatusDetailsResponse{}, nil
}

// mockFlareProvider implements a mock flare provider
type mockFlareProvider struct {
	pbcore.UnimplementedFlareProviderServer
}

func (m *mockFlareProvider) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	return &pbcore.GetFlareFilesResponse{}, nil
}

// mockTelemetryProvider implements a mock telemetry provider
type mockTelemetryProvider struct {
	pbcore.UnimplementedTelemetryProviderServer
}

func (m *mockTelemetryProvider) GetTelemetry(_ context.Context, _ *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	return &pbcore.GetTelemetryResponse{}, nil
}

// mockCoreAgentServer simulates the core agent's registration server
type mockCoreAgentServer struct {
	server       *grpc.Server
	listener     net.Listener
	address      string
	registerFunc func(context.Context, *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error)
	refreshFunc  func(context.Context, *pbcore.RefreshRemoteAgentRequest) (*pbcore.RefreshRemoteAgentResponse, error)
	pbcore.UnimplementedRemoteAgentServer
	echo.UnimplementedEchoServer
}

func (m *mockCoreAgentServer) RegisterRemoteAgent(ctx context.Context, req *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, req)
	}
	return &pbcore.RegisterRemoteAgentResponse{
		SessionId:                      "default-session-id",
		RecommendedRefreshIntervalSecs: 60,
	}, nil
}

func (m *mockCoreAgentServer) RefreshRemoteAgent(ctx context.Context, req *pbcore.RefreshRemoteAgentRequest) (*pbcore.RefreshRemoteAgentResponse, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(ctx, req)
	}
	return &pbcore.RefreshRemoteAgentResponse{}, nil
}

func (m *mockCoreAgentServer) UnaryEcho(_ context.Context, req *echo.EchoRequest) (*echo.EchoResponse, error) {
	return &echo.EchoResponse{
		Message: req.Message,
	}, nil
}

func (m *mockCoreAgentServer) stop() {
	if m.server != nil {
		m.server.Stop()
	}
}

func newMockCoreAgentServer(
	t *testing.T,
	ipcComp ipc.Component,
	registerFunc func(context.Context, *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error),
	refreshFunc func(context.Context, *pbcore.RefreshRemoteAgentRequest) (*pbcore.RefreshRemoteAgentResponse, error),
) *mockCoreAgentServer {
	// Create a real network listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	mock := &mockCoreAgentServer{
		listener:     listener,
		registerFunc: registerFunc,
		refreshFunc:  refreshFunc,
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(ipcComp.GetTLSServerConfig())),
	}

	mock.server = grpc.NewServer(serverOpts...)
	pbcore.RegisterRemoteAgentServer(mock.server, mock)

	// register echo service
	echo.RegisterEchoServer(mock.server, mock)

	// Use the listener's address
	mock.address = listener.Addr().String()

	// Start serving in the background
	go func() {
		err = mock.server.Serve(listener)
		require.NoError(t, err)
	}()

	t.Cleanup(mock.stop)

	// block until the server is started
	// initializing a dummy echo client to make sure the server is started
	client, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())))
	require.NoError(t, err)
	echoClient := echo.NewEchoClient(client)
	_, err = echoClient.UnaryEcho(context.Background(), &echo.EchoRequest{}, grpc.WaitForReady(true))
	require.NoError(t, err)

	return mock
}
