// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"testing"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/comp/core/config"
	helpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func TestRemoteAgentCreation(t *testing.T) {
	provides, lc, _ := buildComponent(t)

	assert.NotNil(t, provides.Comp)
	assert.NotNil(t, provides.FlareProvider)
	assert.NotNil(t, provides.Status)

	lc.AssertHooksNumber(1)

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx))
	assert.NoError(t, lc.Stop(ctx))
}

func TestRecommendedRefreshInterval(t *testing.T) {
	expectedRefreshIntervalSecs := uint32(27)

	provides, _, config := buildComponent(t)
	config.SetWithoutSource("remote_agent_registry.recommended_refresh_interval", fmt.Sprintf("%ds", expectedRefreshIntervalSecs))

	component := provides.Comp

	registrationData := &remoteagent.RegistrationData{
		AgentID:     "test-agent",
		DisplayName: "Test Agent",
		APIEndpoint: "localhost:1234",
		AuthToken:   "",
	}

	actualRefreshIntervalSecs, err := component.RegisterRemoteAgent(registrationData)
	require.NoError(t, err)
	require.Equal(t, expectedRefreshIntervalSecs, actualRefreshIntervalSecs)

	agents := component.GetRegisteredAgents()
	require.Len(t, agents, 1)
	require.Equal(t, "Test Agent", agents[0].DisplayName)
}

func TestGetRegisteredAgents(t *testing.T) {
	provides, _, _ := buildComponent(t)
	component := provides.Comp

	registrationData := &remoteagent.RegistrationData{
		AgentID:     "test-agent",
		DisplayName: "Test Agent",
		APIEndpoint: "localhost:1234",
		AuthToken:   "",
	}

	_, err := component.RegisterRemoteAgent(registrationData)
	require.NoError(t, err)

	agents := component.GetRegisteredAgents()
	require.Len(t, agents, 1)
	require.Equal(t, "Test Agent", agents[0].DisplayName)
}

func TestGetRegisteredAgentStatuses(t *testing.T) {
	provides, _, _ := buildComponent(t)
	component := provides.Comp

	remoteAgentServer := &testRemoteAgentServer{
		StatusMain: map[string]string{
			"test_key": "test_value",
		},
	}

	server, port := buildRemoteAgentServer(t, remoteAgentServer)
	defer server.Stop()

	registrationData := &remoteagent.RegistrationData{
		AgentID:     "test-agent",
		DisplayName: "Test Agent",
		APIEndpoint: fmt.Sprintf("localhost:%d", port),
		AuthToken:   "testing",
	}

	_, err := component.RegisterRemoteAgent(registrationData)
	require.NoError(t, err)

	statuses := component.GetRegisteredAgentStatuses()
	require.Len(t, statuses, 1)
	require.Equal(t, "test-agent", statuses[0].AgentID)
	require.Equal(t, "Test Agent", statuses[0].DisplayName)
	require.Equal(t, "test_value", statuses[0].MainSection["test_key"])
}

func TestFlareProvider(t *testing.T) {
	provides, _, _ := buildComponent(t)
	component := provides.Comp
	flareProvider := provides.FlareProvider

	remoteAgentServer := &testRemoteAgentServer{
		FlareFiles: map[string][]byte{
			"test_file.yaml": []byte("test_content"),
		},
	}

	server, port := buildRemoteAgentServer(t, remoteAgentServer)
	defer server.Stop()

	registrationData := &remoteagent.RegistrationData{
		AgentID:     "test-agent",
		DisplayName: "Test Agent",
		APIEndpoint: fmt.Sprintf("localhost:%d", port),
		AuthToken:   "testing",
	}

	_, err := component.RegisterRemoteAgent(registrationData)
	require.NoError(t, err)

	fb := helpers.NewFlareBuilderMock(t, false)
	fb.AssertNoFileExists("test-agent/test_file.yaml")

	err = flareProvider.FlareFiller.Callback(fb.Fb)
	require.NoError(t, err)
	fb.AssertFileExists("test-agent/test_file.yaml")
	fb.AssertFileContent("test_content", "test-agent/test_file.yaml")
}

func TestStatusProvider(t *testing.T) {
	provides, _, _ := buildComponent(t)
	component := provides.Comp
	statusProvider := provides.Status

	remoteAgentServer := &testRemoteAgentServer{
		StatusMain: map[string]string{
			"test_key": "test_value",
		},
	}

	server, port := buildRemoteAgentServer(t, remoteAgentServer)
	defer server.Stop()

	registrationData := &remoteagent.RegistrationData{
		AgentID:     "test-agent",
		DisplayName: "Test Agent",
		APIEndpoint: fmt.Sprintf("localhost:%d", port),
		AuthToken:   "testing",
	}

	_, err := component.RegisterRemoteAgent(registrationData)
	require.NoError(t, err)

	statusData := make(map[string]interface{})
	err = statusProvider.Provider.JSON(false, statusData)
	require.NoError(t, err)

	require.Len(t, statusData, 2)

	registeredAgents, ok := statusData["registeredAgents"].([]*remoteagent.RegisteredAgent)
	if !ok {
		t.Fatalf("registeredAgents is not a slice of RegisteredAgent")
	}
	require.Len(t, registeredAgents, 1)
	require.Equal(t, "Test Agent", registeredAgents[0].DisplayName)

	registeredAgentStatuses, ok := statusData["registeredAgentStatuses"].([]*remoteagent.StatusData)
	if !ok {
		t.Fatalf("registeredAgentStatuses is not a slice of StatusData")
	}
	require.Len(t, registeredAgentStatuses, 1)
	require.Equal(t, "test-agent", registeredAgentStatuses[0].AgentID)
	require.Equal(t, "Test Agent", registeredAgentStatuses[0].DisplayName)
	require.Equal(t, "test_value", registeredAgentStatuses[0].MainSection["test_key"])
}

func TestDisabled(t *testing.T) {
	config := configmock.New(t)

	provides, _ := buildComponentWithConfig(t, config)

	require.Nil(t, provides.Comp)
	require.Nil(t, provides.FlareProvider.FlareFiller)
	require.Nil(t, provides.Status.Provider)
}

func buildComponent(t *testing.T) (Provides, *compdef.TestLifecycle, config.Component) {
	config := configmock.New(t)
	config.SetWithoutSource("remote_agent_registry.enabled", true)

	provides, lc := buildComponentWithConfig(t, config)
	return provides, lc, config
}

func buildComponentWithConfig(t *testing.T, config configmodel.Config) (Provides, *compdef.TestLifecycle) {
	lc := compdef.NewTestLifecycle(t)
	reqs := Requires{
		Config:    config,
		Lifecycle: lc,
	}

	return NewComponent(reqs), lc
}

type testRemoteAgentServer struct {
	StatusMain  map[string]string
	StatusNamed map[string]map[string]string
	FlareFiles  map[string][]byte
}

func (t *testRemoteAgentServer) GetStatusDetails(context.Context, *pbgo.GetStatusDetailsRequest) (*pbgo.GetStatusDetailsResponse, error) {
	namedSections := make(map[string]*pbgo.StatusSection)
	for name, fields := range t.StatusNamed {
		namedSections[name] = &pbgo.StatusSection{
			Fields: fields,
		}
	}

	return &pbgo.GetStatusDetailsResponse{
		MainSection: &pbgo.StatusSection{
			Fields: t.StatusMain,
		},
		NamedSections: namedSections,
	}, nil
}

func (t *testRemoteAgentServer) GetFlareFiles(context.Context, *pbgo.GetFlareFilesRequest) (*pbgo.GetFlareFilesResponse, error) {
	return &pbgo.GetFlareFilesResponse{
		Files: t.FlareFiles,
	}, nil
}

func buildRemoteAgentServer(t *testing.T, remoteAgentServer *testRemoteAgentServer) (*grpc.Server, uint16) {
	tlsKeyPair, err := buildSelfSignedTLSCertificate()
	require.NoError(t, err)

	// Make sure we can listen on the intended address.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(tlsKeyPair)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor("testing"))),
	}

	server := grpc.NewServer(serverOpts...)
	pbgo.RegisterRemoteAgentServer(server, remoteAgentServer)

	go func() {
		err := server.Serve(listener)
		require.NoError(t, err)
	}()

	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	return server, uint16(port)
}

func buildSelfSignedTLSCertificate() (*tls.Certificate, error) {
	hosts := []string{"localhost"}
	_, certPEM, key, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return nil, errors.New("unable to generate certificate")
	}

	// PEM encode the private key
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("unable to generate TLS key pair: %v", err)
	}

	return &pair, nil
}
