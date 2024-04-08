// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package secretsimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type MockProvides struct {
	fx.Out

	Comp            secrets.Mock
	InfoEndpoint    api.AgentEndpointProvider
	RefreshEndpoint api.AgentEndpointProvider
}

// MockSecretResolver is a mock of the secret Component useful for testing
type MockSecretResolver struct {
	*secretResolver
}

type MockInfoEndpoint struct {
	Comp *MockSecretResolver
}

func (e MockInfoEndpoint) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

type MockRefreshEndpoint struct {
	Comp *MockSecretResolver
}

func (e MockRefreshEndpoint) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

var _ secrets.Component = (*MockSecretResolver)(nil)

// SetBackendCommand sets the backend command for the mock
func (m *MockSecretResolver) SetBackendCommand(command string) {
	m.backendCommand = command
}

// SetFetchHookFunc sets the fetchHookFunc for the mock
func (m *MockSecretResolver) SetFetchHookFunc(f func([]string) (map[string]string, error)) {
	m.fetchHookFunc = f
}

// NewMock returns a MockSecretResolver
func NewMock() MockProvides {
	r := &MockSecretResolver{
		secretResolver: newEnabledSecretResolver(),
	}
	return MockProvides{
		Comp:            r,
		InfoEndpoint:    api.NewAgentEndpointProvider(MockInfoEndpoint{Comp: r}, "/secrets", "GET"),
		RefreshEndpoint: api.NewAgentEndpointProvider(MockRefreshEndpoint{Comp: r}, "/secret/refresh", "GET"),
	}
}

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock))
}
