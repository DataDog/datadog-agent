// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides mock implementation of the rcclient component
package mock

import (
	"go.uber.org/fx"

	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods.
type Mock interface {
	rcclient.Component
}

type mockRCClient struct{}

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return Module()
}

// NewMock creates a new mock rcclient component
func NewMock() rcclient.Component {
	return &mockRCClient{}
}

// SubscribeAgentTask is a mock implementation
func (m *mockRCClient) SubscribeAgentTask() {
	// No-op for mock
}

// Subscribe is a mock implementation
func (m *mockRCClient) Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	// No-op for mock
}
