// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package secretsimpl

import (
	"io"
	"regexp"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type callbackArgs struct {
	yamlPath []string
	value    any
}

// MockSecretResolver is a mock of the secret Component useful for testing
type MockSecretResolver struct {
	resolve   map[string]string
	callbacks []callbackArgs
}

var _ secrets.Component = (*MockSecretResolver)(nil)

// Configure is not implemented
func (m *MockSecretResolver) Configure(_ string, _ []string, _, _ int, _, _ bool) {}

// GetDebugInfo is not implemented
func (m *MockSecretResolver) GetDebugInfo(_ io.Writer) {}

// Inject adds data to be resolved, by returning the value for the given key
func (m *MockSecretResolver) Inject(key, value string) {
	m.resolve[key] = value
}

// InjectCallback adds to the list of callbacks that will be used to mock ResolveWithCallback. Each injected callback
// will equal a call to the callback givent to 'ResolveWithCallback'
func (m *MockSecretResolver) InjectCallback(yamlPath []string, value any) {
	m.callbacks = append(m.callbacks, callbackArgs{yamlPath: yamlPath, value: value})
}

// Resolve returns the secret value based upon the injected data
func (m *MockSecretResolver) Resolve(data []byte, _ string) ([]byte, error) {
	re := regexp.MustCompile(`ENC\[(.*?)\]`)
	result := re.ReplaceAllStringFunc(string(data), func(in string) string {
		key := in[4 : len(in)-1]
		return m.resolve[key]
	})
	return []byte(result), nil
}

// ResolveWithCallback mocks the ResolveWithCallback method of the secrets Component
func (m *MockSecretResolver) ResolveWithCallback(_ []byte, _ string, cb secrets.ResolveCallback) error {
	for _, call := range m.callbacks {
		cb(call.yamlPath, call.value)
	}
	return nil
}

// NewMockSecretResolver constructs a MockSecretResolver
func NewMockSecretResolver() *MockSecretResolver {
	return &MockSecretResolver{resolve: make(map[string]string)}
}

// MockModule is a module containing the mock, useful for testing
var MockModule = fxutil.Component(
	fx.Provide(NewMockSecretResolver),
)
