// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package secretsimpl

import (
	"net/http"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type testDeps struct {
	fx.In

	Telemetry telemetry.Component
}

type MockProvides struct {
	fx.Out

	Comp            secrets.Component
	InfoEndpoint    api.AgentEndpointProvider
	RefreshEndpoint api.AgentEndpointProvider
}

// MockSecretResolver is a mock of the secret Component useful for testing
type MockSecretResolver struct {
	*secretResolver
}

var _ secrets.Component = (*MockSecretResolver)(nil)

func (r *MockSecretResolver) mockHandleRequest(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// SetBackendCommand sets the backend command for the mock
func (m *MockSecretResolver) SetBackendCommand(command string) {
	m.backendCommand = command
}

// SetFetchHookFunc sets the fetchHookFunc for the mock
func (m *MockSecretResolver) SetFetchHookFunc(f func([]string) (map[string]string, error)) {
	m.fetchHookFunc = f
}

// NewMock returns a MockSecretResolver
func NewMock(testDeps testDeps) MockProvides {
	r := &MockSecretResolver{
		secretResolver: newEnabledSecretResolver(testDeps.Telemetry),
	}
	return MockProvides{
		Comp:            r,
		InfoEndpoint:    api.NewAgentEndpointProvider(r.mockHandleRequest, "/secrets", "GET"),
		RefreshEndpoint: api.NewAgentEndpointProvider(r.mockHandleRequest, "/secret/refresh", "GET"),
	}
}

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock))
}
