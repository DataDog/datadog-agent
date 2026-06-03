// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the hostnameinterface component.
package mock

import (
	"context"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// Mock implements mock-specific methods.
type Mock interface {
	// Component methods are included in Mock.
	hostnameinterface.Component
}

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide the hostname 'my-hostname';
// override this with fx.Replace(hostname.MockHostname("whatever")).
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			NewMock,
		),
		fx.Supply(MockHostname("my-hostname")))
}

type mockService struct {
	name string
}

var _ Mock = (*mockService)(nil)

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
func (m *mockService) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: m.name,
		Provider: "mockService",
	}, nil
}

// MockHostname is an alias for injecting a mock hostname.
// Usage: fx.Replace(hostname.MockHostname("whatever"))
type MockHostname string

// NewMock returns a new instance of the mock for the component hostname
func NewMock(name MockHostname) (hostnameinterface.Component, Mock) {
	mock := &mockService{string(name)}
	return mock, mock
}
