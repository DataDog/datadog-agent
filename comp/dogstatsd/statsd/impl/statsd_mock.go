// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package statsdimpl implements the statsd component.
package statsdimpl

import (
	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	statsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock interface for testing.
type Mock interface {
	statsd.Component
}

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide a NoOpClient by default;
// override this with fx.Replace(fx.Annotate(client, fx.As(new(MockClient)))).
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			newMock,
		),
		fx.Supply(fx.Annotate(&ddgostatsd.NoOpClient{}, fx.As(new(MockClient)))))
}

type mockService struct {
	client ddgostatsd.ClientInterface
}

// Get returns a pre-configured and shared statsd client (requires STATSD_URL env var to be set)
func (m *mockService) Get() (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

// Create returns a pre-configured statsd client
func (m *mockService) Create(_ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

// CreateForAddr returns a pre-configured statsd client that defaults to `addr` if no env var is set
func (m *mockService) CreateForAddr(_ string, _ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

// CreateForHostPort returns a pre-configured statsd client that defaults to `host:port` if no env var is set
func (m *mockService) CreateForHostPort(_ string, _ int, _ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

var _ Mock = (*mockService)(nil)

// MockClient is an alias for injecting a mock client.
// Usage: fx.Replace(fx.Annotate(client, fx.As(new(MockClient)))
type MockClient ddgostatsd.ClientInterface

func newMock(client MockClient) (statsd.Component, Mock) {
	mock := &mockService{client}
	return mock, mock
}
