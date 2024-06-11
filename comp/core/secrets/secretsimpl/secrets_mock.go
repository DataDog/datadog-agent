// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package secretsimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockSecretResolver is a mock of the secret Component useful for testing
type MockSecretResolver struct {
	*secretResolver
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

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type testDeps struct {
	fx.In

	Telemetry telemetry.Component
}

func newMock(testDeps testDeps) secrets.Component {
	return &MockSecretResolver{
		secretResolver: newEnabledSecretResolver(testDeps.Telemetry),
	}
}
