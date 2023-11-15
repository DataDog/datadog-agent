// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package secretsimpl

import (
	"io"
	"regexp"
)

type MockSecretResolver struct {
	resolve map[string]string
}

func (m *MockSecretResolver) Configure(_ string, _ []string, _, _ int, _, _ bool) {}

func (m *MockSecretResolver) GetDebugInfo(_ io.Writer) {}

func (m *MockSecretResolver) Inject(key, value string) {
	m.resolve[key] = value
}

func (m *MockSecretResolver) Decrypt(data []byte, _ string) ([]byte, error) {
	re := regexp.MustCompile(`ENC\[(.*?)\]`)
	result := re.ReplaceAllStringFunc(string(data), func(in string) string {
		key := in[4 : len(in)-1]
		return m.resolve[key]
	})
	return []byte(result), nil
}

func NewMockSecretResolver() *MockSecretResolver {
	return &MockSecretResolver{resolve: make(map[string]string)}
}
