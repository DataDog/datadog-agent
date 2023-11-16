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

// MockSecretResolver is a mock of the secret Component useful for testing
type MockSecretResolver struct {
	resolve map[string]string
}

var _ secrets.Component = (*MockSecretResolver)(nil)

// Configure is not implemented
func (m *MockSecretResolver) Configure(_ string, _ []string, _, _ int, _, _ bool) {}

// GetDebugInfo is not implemented
func (m *MockSecretResolver) GetDebugInfo(_ io.Writer) {}

// IsEnabled always returns true
func (m *MockSecretResolver) IsEnabled() bool {
	return true
}

// Inject adds data to be decrypted, by returning the value for the given key
func (m *MockSecretResolver) Inject(key, value string) {
	m.resolve[key] = value
}

// Decrypt returns the secret value based upon the injected data
func (m *MockSecretResolver) Decrypt(data []byte, _ string) ([]byte, error) {
	re := regexp.MustCompile(`ENC\[(.*?)\]`)
	result := re.ReplaceAllStringFunc(string(data), func(in string) string {
		key := in[4 : len(in)-1]
		return m.resolve[key]
	})
	return []byte(result), nil
}

// NewMockSecretResolver constructs a MockSecretResolver
func NewMockSecretResolver() *MockSecretResolver {
	return &MockSecretResolver{resolve: make(map[string]string)}
}

// MockModule is a module containing the mock, useful for testing
var MockModule = fxutil.Component(
	fx.Provide(NewMockSecretResolver),
)
