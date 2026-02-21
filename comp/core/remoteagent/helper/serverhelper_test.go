// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package helper

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

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

	// Start the server
	err = lc.Start(context.Background())
	require.NoError(t, err)
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

	// Start the server
	err = lc.Start(context.Background())
	require.NoError(t, err)
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

	// Verify lifecycle hooks were registered
	lc.AssertHooksNumber(1)

	// Start the server
	err = lc.Start(context.Background())
	require.NoError(t, err)

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

	// Start the server
	err = lc.Start(context.Background())
	require.NoError(t, err)
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

	// Set a short query timeout for faster test
	configComp.SetInTest("remote_agent_registry.query_timeout", 50*time.Millisecond)

	registerCallCount := 0
	refreshCallCount := 0
	var mu sync.Mutex

	// Create a mock core agent server where registration hangs the 2 first calls
	mockCoreAgent := newMockCoreAgentServer(t, ipcComp,
		func(ctx context.Context, _ *pbcore.RegisterRemoteAgentRequest) (*pbcore.RegisterRemoteAgentResponse, error) {
			mu.Lock()
			registerCallCount++
			if registerCallCount > 2 {
				defer mu.Unlock()
				return &pbcore.RegisterRemoteAgentResponse{
					SessionId:                      "uuid_session_id",
					RecommendedRefreshIntervalSecs: 1,
				}, nil
			}
			mu.Unlock()

			// Block forever to prevent registration from completing
			<-ctx.Done()
			return nil, ctx.Err()
		},
		func(_ context.Context, _ *pbcore.RefreshRemoteAgentRequest) (*pbcore.RefreshRemoteAgentResponse, error) {
			mu.Lock()
			refreshCallCount++
			mu.Unlock()
			return &pbcore.RefreshRemoteAgentResponse{}, nil
		},
	)
	defer mockCoreAgent.stop()

	// Create the remote agent server
	_, err := NewUnimplementedRemoteAgentServer(
		ipcComp,
		logComp,
		configComp,
		lc,
		mockCoreAgent.address,
		"test-agent",
		"Test Agent",
	)
	require.NoError(t, err)

	// Start the server
	err = lc.Start(context.Background())
	require.NoError(t, err)
	defer lc.Stop(context.Background())

	// Wait for multiple registration attempts
	time.Sleep(3 * time.Second)

	// Verify that registration was retried after the first timeout
	mu.Lock()
	regCount := registerCallCount
	mu.Unlock()

	require.Equal(t, regCount, 3, "Registration should have been retried after timeout")

	// Now test refresh contention - the server should be registered by now
	// Wait a bit more to ensure refresh is called
	time.Sleep(2 * time.Second)

	mu.Lock()
	refCount := refreshCallCount
	mu.Unlock()

	// We should have at least one refresh call by now
	assert.Greater(t, refCount, 0, "Refresh should have been called")
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

	// Start the server
	err = lc.Start(context.Background())
	require.NoError(t, err)
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
	pbcore.UnimplementedAgentSecureServer
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
	pbcore.RegisterAgentSecureServer(mock.server, mock)

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
