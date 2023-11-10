// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package hostnameimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide the hostname 'my-hostname';
// override this with fx.Replace(hostname.MockHostname("whatever")).
var MockModule = fxutil.Component(
	fx.Provide(
		newMock,
	),
	fx.Supply(MockHostname("my-hostname")),
)

type mockService struct {
	name string
}

var _ hostname.Mock = (*mockService)(nil)

func (m *mockService) Get(_ context.Context) (string, error) {
	return m.name, nil
}

func (m *mockService) GetSafe(_ context.Context) string {
	return m.name
}

func (m *mockService) Set(name string) {
	m.name = name
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it.
func (m *mockService) GetWithProvider(_ context.Context) (pkghostname.Data, error) {
	return pkghostname.Data{
		Hostname: m.name,
		Provider: "mockService",
	}, nil
}

// MockHostname is an alias for injecting a mock hostname.
// Usage: fx.Replace(hostname.MockHostname("whatever"))
type MockHostname string

func newMock(name MockHostname) (hostname.Component, hostname.Mock) {
	mock := &mockService{string(name)}
	return mock, mock
}
