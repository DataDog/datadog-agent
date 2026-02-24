// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
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
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
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
	config.SetInTest("remote_agent_registry.recommended_refresh_interval", fmt.Sprintf("%ds", expectedRefreshIntervalSecs))

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

func TestGetRegisteredAgentsIdleTimeout(t *testing.T) {
	provides, lc, config, _, ipcComp := buildComponent(t)
	component := provides.Comp.(*remoteAgentRegistry)

	// Overriding default config values to have a faster test
	config.SetInTest("remote_agent_registry.idle_timeout", time.Duration(time.Second*5))
	config.SetInTest("remote_agent_registry.recommended_refresh_interval", time.Duration(time.Second*5))

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

func TestDisabled(t *testing.T) {
	config := configmock.New(t)
	config.SetInTest("remote_agent_registry.enabled", false)

	provides, _, _, _ := buildComponentWithConfig(t, config)

	require.Nil(t, provides.Comp)
	require.Nil(t, provides.FlareProvider.FlareFiller)
	require.Nil(t, provides.Status.Provider)
}

func buildComponent(t *testing.T) (Provides, *compdef.TestLifecycle, config.Component, telemetry.Component, ipc.Component) {
	config := configmock.New(t)

	// enable the remote agent registry
	config.SetInTest("remote_agent_registry.enabled", true)

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
	testServer := &testRemoteAgentServer{
		RegistrationData: remoteagent.RegistrationData{
			AgentFlavor:      agentFlavor,
			AgentPID:         agentPID,
			AgentDisplayName: agentName,
			Services:         []string{}, // Will be populated by mock providers
		},
	}

	// Make sure we can listen on the intended address.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

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

	testServer.RegistrationData.APIEndpointURI = listener.Addr().String()
	testServer.server = server

	// block until the server is started
	// initializing a dummy echo client to make sure the server is started
	client, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())))
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
