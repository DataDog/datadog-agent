// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostname

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

type mockService struct {
	name string
}

var _ Component = (*mockService)(nil)

func (m *mockService) Get(context.Context) (string, error) {
	return m.name, nil
}

func (m *mockService) GetSafe(context.Context) string {
	return m.name
}

func (m *mockService) Set(name string) {
	m.name = name
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it.
func (m *mockService) GetWithProvider(context.Context) (hostname.Data, error) {
	return hostname.Data{
		Hostname: m.name,
		Provider: "mockService",
	}, nil
}

// MockHostname is an alias for injecting a mock hostname.
// Usage: fx.Replace(hostname.MockHostname("whatever"))
type MockHostname string

func newMock(name MockHostname) Component {
	return &mockService{string(name)}
}
