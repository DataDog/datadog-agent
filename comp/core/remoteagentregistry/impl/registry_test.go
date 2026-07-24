// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func TestRemoteAgentCreation(t *testing.T) {
	provides, lc, _, _, _ := buildComponent(t)

	assert.NotNil(t, provides.Comp)
	assert.NotNil(t, provides.FlareProvider)
	assert.NotNil(t, provides.Status)

	lc.AssertHooksNumber(1)

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx))
	assert.NoError(t, lc.Stop(ctx))
}

func TestRegistration(t *testing.T) {
	expectedRefreshIntervalSecs := uint32(27)

	provides, _, config, _, ipcComp := buildComponent(t)
	config.SetInTest("remote_agent.registry.recommended_refresh_interval", fmt.Sprintf("%ds", expectedRefreshIntervalSecs))

	component := provides.Comp.(*remoteAgentRegistry)

	remoteAgent := buildRemoteAgent(t, ipcComp, "test-agent", "Test Agent", "1234")
	_, actualRefreshIntervalSecs, err := component.RegisterRemoteAgent(&remoteAgent.RegistrationData)
	require.NoError(t, err)

	require.Equal(t, expectedRefreshIntervalSecs, actualRefreshIntervalSecs)

	agents := component.GetRegisteredAgents()
	require.Len(t, agents, 1)
	require.Equal(t, "Test Agent", agents[0].DisplayName)
	require.Equal(t, "test-agent", agents[0].SanitizedDisplayName)
}

func TestReportRemoteAgentEvent(t *testing.T) {
	provides, _, _, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "1234")

	events := []remoteagent.RemoteAgentEvent{
		{Message: "invalid API key detected", Details: &remoteagent.InvalidAPIKey{}},
	}

	// A known session accepts the reported events.
	require.NoError(t, component.ReportRemoteAgentEvent(remoteAgent.registeredSessionID, events))

	// An unknown session returns an error.
	require.Error(t, component.ReportRemoteAgentEvent("does-not-exist", events))
}

func TestReportRemoteAgentEventBroadcast(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("remote_agent.registry.enabled", true)

	lc := compdef.NewTestLifecycle(t)
	tel := telemetryimpl.NewMock(t)
	ipcComp := ipcmock.New(t)

	var mu sync.Mutex
	var gotAgent remoteagent.RegisteredAgent
	var gotEvents []remoteagent.RemoteAgentEvent
	recorderCalls := 0

	// A panicking subscriber must not fail the RPC or starve the recorder, and a nil entry must be
	// skipped, so the recorder (registered last) still receives the events exactly once.
	panicker := &remoteagent.EventSubscriber{
		Name:     "panicker",
		Callback: func(remoteagent.RegisteredAgent, []remoteagent.RemoteAgentEvent) { panic("boom") },
	}
	recorder := &remoteagent.EventSubscriber{
		Name: "recorder",
		Callback: func(agent remoteagent.RegisteredAgent, events []remoteagent.RemoteAgentEvent) {
			mu.Lock()
			defer mu.Unlock()
			recorderCalls++
			gotAgent = agent
			gotEvents = events
		},
	}

	reqs := Requires{
		Config:           cfg,
		Ipc:              ipcComp,
		Lifecycle:        lc,
		Telemetry:        tel,
		EventSubscribers: []*remoteagent.EventSubscriber{panicker, nil, recorder},
	}
	component := NewComponent(reqs).Comp.(*remoteAgentRegistry)

	remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "1234")

	events := []remoteagent.RemoteAgentEvent{
		{Message: "invalid API key detected", Details: &remoteagent.InvalidAPIKey{}},
	}

	// The panicking subscriber is recovered and the nil subscriber skipped, so the call still succeeds.
	require.NoError(t, component.ReportRemoteAgentEvent(remoteAgent.registeredSessionID, events))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, recorderCalls)
	require.Equal(t, "Test Agent", gotAgent.DisplayName)
	require.Len(t, gotEvents, 1)
	require.Equal(t, "invalid_api_key", gotEvents[0].Details.EventType())
}

func TestGetRegisteredAgentsIdleTimeout(t *testing.T) {
	provides, lc, config, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	// Overriding default config values to have a faster test
	config.SetInTest("remote_agent.registry.idle_timeout", time.Duration(time.Second*5))
	config.SetInTest("remote_agent.registry.recommended_refresh_interval", time.Duration(time.Second*5))

	lc.Start(context.Background())
	defer lc.Stop(context.Background())

	remoteAgent := buildAndRegisterRemoteAgent(t, ipcComp, component, "test-agent", "Test Agent", "123",
		withStatusProvider(map[string]string{
			"test_key": "test_value",
		}, nil),
	)

	agents := component.GetRegisteredAgents()
	require.Len(t, agents, 1)
	require.Equal(t, "Test Agent", agents[0].DisplayName)
	require.Equal(t, "test-agent", agents[0].SanitizedDisplayName)

	// Stopping the remote agent should remove it from the registry
	remoteAgent.Stop()
	time.Sleep(10 * time.Second)

	agents = component.GetRegisteredAgents()
	require.Len(t, agents, 0)
}

// TestRegistryDialsUDSRemoteAgent verifies the end-to-end UDS path: a remote agent
// registers with a "unix:///path" api_endpoint_uri, and the registry then dials it
// over a UDS connection to fetch status.
func TestRegistryDialsUDSRemoteAgent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("UDS not supported on Windows")
	}

	provides, lc, cfg, _, ipcComp := buildComponent(t)
	cfg.SetInTest("remote_agent.registry.query_timeout", 2*time.Second)

	component := provides.Comp.(*remoteAgentRegistry)
	require.NoError(t, lc.Start(context.Background()))
	t.Cleanup(func() { _ = lc.Stop(context.Background()) })

	// macOS limits sun_path to 104 bytes; use a short fixed prefix instead of t.TempDir().
	udsDir, err := os.MkdirTemp("/tmp", "rar-uds-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(udsDir) })
	socketPath := filepath.Join(udsDir, "a.sock")
	udsListener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	agent := buildRemoteAgentOnListener(t, ipcComp, udsListener, "unix://"+socketPath,
		"uds-agent", "UDS Agent", "9999",
		withStatusProvider(map[string]string{"transport": "uds"}, nil),
	)

	sessionID, _, err := component.RegisterRemoteAgent(&agent.RegistrationData)
	require.NoError(t, err)
	agent.registeredSessionID = sessionID

	// Issue a status RPC via the registry's normal call path and verify the response
	// came from the UDS-backed agent.
	type statusResult struct {
		flavor string
		fields map[string]string
		err    error
	}
	results := callAgentsForService(component, StatusServiceName,
		func(ctx context.Context, rac *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetStatusDetailsResponse, error) {
			return rac.GetStatusDetails(ctx, &pb.GetStatusDetailsRequest{}, opts...)
		},
		func(reg remoteagent.RegisteredAgent, resp *pb.GetStatusDetailsResponse, err error) statusResult {
			if err != nil || resp == nil || resp.MainSection == nil {
				return statusResult{flavor: reg.Flavor, err: err}
			}
			return statusResult{flavor: reg.Flavor, fields: resp.MainSection.Fields}
		},
	)

	require.Len(t, results, 1)
	require.NoError(t, results[0].err)
	assert.Equal(t, "uds-agent", results[0].flavor)
	assert.Equal(t, "uds", results[0].fields["transport"])
}

func TestDisabled(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("remote_agent.registry.enabled", false)

	provides, _, _, _ := buildComponentWithConfig(t, config)

	require.Nil(t, provides.Comp)
	require.Nil(t, provides.FlareProvider.FlareFiller)
	require.Nil(t, provides.Status.Provider)
}

func buildComponent(t *testing.T) (Provides, *compdef.TestLifecycle, config.Component, telemetry.Component, ipc.Component) {
	config := configmock.New(t)

	// enable the remote agent registry
	config.SetInTest("remote_agent.registry.enabled", true)

	provides, lc, telemetry, ipc := buildComponentWithConfig(t, config)
	return provides, lc, config, telemetry, ipc
}

func buildComponentWithConfig(t *testing.T, config configmodel.Config) (Provides, *compdef.TestLifecycle, telemetry.Component, ipc.Component) {
	lc := compdef.NewTestLifecycle(t)
	telemetry := telemetryimpl.NewMock(t)
	ipc := ipcmock.New(t)
	reqs := Requires{
		Config:    config,
		Ipc:       ipc,
		Lifecycle: lc,
		Telemetry: telemetry,
	}

	return NewComponent(reqs), lc, telemetry, ipc
}

//
// testRemoteAgentServer is a mock implementation of the remote agent server.
//

type testRemoteAgentServer struct {
	// registration values
	remoteagent.RegistrationData

	// Mock values
	statusMain    map[string]string
	statusNamed   map[string]map[string]string
	flareFiles    map[string][]byte
	promText      string
	responseDelay time.Duration
	ConfigEvents  chan *pb.ConfigEvent

	// session ID behavior
	registeredSessionID string // session ID received during registration
	overrideSessionID   bool   // if true, send the overrideSessionID instead of the correct session ID
	fakeSessionID       string // if overrideSessionID is true, send this instead of the correct session ID

	// gRPC embedded
	server *grpc.Server

	pb.UnimplementedStatusProviderServer
	pb.UnimplementedFlareProviderServer
	pb.UnimplementedTelemetryProviderServer
	echo.UnimplementedEchoServer
}

func (t *testRemoteAgentServer) GetStatusDetails(context.Context, *pb.GetStatusDetailsRequest) (*pb.GetStatusDetailsResponse, error) {
	namedSections := make(map[string]*pb.StatusSection)
	for name, fields := range t.statusNamed {
		namedSections[name] = &pb.StatusSection{
			Fields: fields,
		}
	}

	return &pb.GetStatusDetailsResponse{
		MainSection: &pb.StatusSection{
			Fields: t.statusMain,
		},
		NamedSections: namedSections,
	}, nil
}

func (t *testRemoteAgentServer) GetFlareFiles(context.Context, *pb.GetFlareFilesRequest) (*pb.GetFlareFilesResponse, error) {
	return &pb.GetFlareFilesResponse{
		Files: t.flareFiles,
	}, nil
}

func (t *testRemoteAgentServer) UnaryEcho(_ context.Context, req *echo.EchoRequest) (*echo.EchoResponse, error) {
	return &echo.EchoResponse{
		Message: req.Message,
	}, nil
}

func (t *testRemoteAgentServer) GetTelemetry(context.Context, *pb.GetTelemetryRequest) (*pb.GetTelemetryResponse, error) {
	return &pb.GetTelemetryResponse{
		Payload: &pb.GetTelemetryResponse_PromText{
			PromText: t.promText,
		},
	}, nil
}

func (t *testRemoteAgentServer) Stop() {
	t.server.Stop()
}

type mockProvider func(*grpc.Server, *testRemoteAgentServer)

func WithFlareProvider(flareFiles map[string][]byte) func(*grpc.Server, *testRemoteAgentServer) {
	return func(s *grpc.Server, tras *testRemoteAgentServer) {
		tras.flareFiles = flareFiles
		pb.RegisterFlareProviderServer(s, tras)
		// Add flare service to the registration data
		tras.RegistrationData.Services = append(tras.RegistrationData.Services, "datadog.remoteagent.flare.v1.FlareProvider")
	}
}
func withStatusProvider(statusMain map[string]string, statusNamed map[string]map[string]string) func(*grpc.Server, *testRemoteAgentServer) {
	return func(s *grpc.Server, tras *testRemoteAgentServer) {
		tras.statusMain = statusMain
		tras.statusNamed = statusNamed
		pb.RegisterStatusProviderServer(s, tras)
		// Add status service to the registration data
		tras.RegistrationData.Services = append(tras.RegistrationData.Services, "datadog.remoteagent.status.v1.StatusProvider")
	}
}
func withTelemetryProvider(promText string) func(*grpc.Server, *testRemoteAgentServer) {
	return func(s *grpc.Server, tras *testRemoteAgentServer) {
		tras.promText = promText
		pb.RegisterTelemetryProviderServer(s, tras)
		// Add telemetry service to the registration data
		tras.RegistrationData.Services = append(tras.RegistrationData.Services, "datadog.remoteagent.telemetry.v1.TelemetryProvider")
	}
}

func withDelay(delay time.Duration) func(*grpc.Server, *testRemoteAgentServer) {
	return func(_ *grpc.Server, tras *testRemoteAgentServer) {
		tras.responseDelay = delay
	}
}

func withFakeSessionID(fakeSessionID string) func(*grpc.Server, *testRemoteAgentServer) {
	return func(_ *grpc.Server, tras *testRemoteAgentServer) {
		tras.fakeSessionID = fakeSessionID
		tras.overrideSessionID = true
	}
}

func buildRemoteAgent(t *testing.T, ipcComp ipc.Component, agentFlavor string, agentName string, agentPID string, mockProviders ...mockProvider) *testRemoteAgentServer {
	// Default to a random localhost TCP port; the advertised api_endpoint_uri is the
	// bare host:port form (backwards-compatible default for the registry client).
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return buildRemoteAgentOnListener(t, ipcComp, listener, listener.Addr().String(), agentFlavor, agentName, agentPID, mockProviders...)
}

// buildRemoteAgentOnListener is like buildRemoteAgent but reuses a caller-supplied
// listener, letting tests exercise non-TCP transports (e.g. UDS).
//
// apiEndpointURI is recorded verbatim in RegistrationData and must match what the
// registry-side client expects to dial — e.g. "unix:///path/to/sock" for UDS or
// "https://host:port" for TLS-over-TCP.
func buildRemoteAgentOnListener(t *testing.T, ipcComp ipc.Component, listener net.Listener, apiEndpointURI string, agentFlavor string, agentName string, agentPID string, mockProviders ...mockProvider) *testRemoteAgentServer {
	testServer := &testRemoteAgentServer{
		RegistrationData: remoteagent.RegistrationData{
			AgentFlavor:      agentFlavor,
			AgentPID:         agentPID,
			AgentDisplayName: agentName,
			Services:         []string{}, // Will be populated by mock providers
		},
	}

	// Create delay interceptor that uses testServer.responseDelay
	delayInterceptor := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if testServer.responseDelay > 0 {
			time.Sleep(testServer.responseDelay)
		}
		return handler(ctx, req)
	}

	// Create session ID interceptor that adds session_id to response metadata
	sessionIDInterceptor := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		sessionID := testServer.registeredSessionID
		if testServer.overrideSessionID {
			sessionID = testServer.fakeSessionID
		}
		grpc.SetHeader(ctx, metadata.New(map[string]string{"session_id": sessionID}))
		return handler(ctx, req)
	}

	// Chain interceptors: auth first, then session ID, then delay
	chainedInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Don't verify session ID for echo requests
		if _, ok := req.(*echo.EchoRequest); ok {
			return handler(ctx, req)
		}

		// First apply auth interceptor
		authHandler := grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(ipcComp.GetAuthToken()))
		return authHandler(ctx, req, info, func(ctx context.Context, req any) (any, error) {
			// Then apply session ID interceptor
			return sessionIDInterceptor(ctx, req, info, func(ctx context.Context, req any) (any, error) {
				// Then apply delay interceptor
				return delayInterceptor(ctx, req, info, handler)
			})
		})
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(ipcComp.GetTLSServerConfig())),
		grpc.UnaryInterceptor(chainedInterceptor),
	}

	server := grpc.NewServer(serverOpts...)
	for _, provider := range mockProviders {
		provider(server, testServer)
	}

	// register echo service
	echo.RegisterEchoServer(server, testServer)

	go func() {
		err := server.Serve(listener)
		require.NoError(t, err)
	}()

	t.Cleanup(server.Stop)

	testServer.RegistrationData.APIEndpointURI = apiEndpointURI
	testServer.server = server

	// block until the server is started
	// initializing a dummy echo client to make sure the server is started
	probeTarget, probeCreds, err := resolveDialTarget(apiEndpointURI, ipcComp.GetTLSClientConfig())
	require.NoError(t, err)
	client, err := grpc.NewClient(probeTarget, grpc.WithTransportCredentials(probeCreds))
	require.NoError(t, err)
	echoClient := echo.NewEchoClient(client)
	_, err = echoClient.UnaryEcho(context.Background(), &echo.EchoRequest{}, grpc.WaitForReady(true))
	require.NoError(t, err)

	return testServer
}

func buildAndRegisterRemoteAgent(t *testing.T, ipcComp ipc.Component, registryComp remoteagent.Component, agentFlavor string, agentName string, agentPID string, mockProviders ...mockProvider) *testRemoteAgentServer {
	testServer := buildRemoteAgent(t, ipcComp, agentFlavor, agentName, agentPID, mockProviders...)
	sessionID, _, err := registryComp.RegisterRemoteAgent(&testServer.RegistrationData)
	require.NoError(t, err)

	testServer.registeredSessionID = sessionID
	return testServer
}
