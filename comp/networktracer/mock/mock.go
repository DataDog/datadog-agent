// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the networktracer component.
package mock

import (
	"context"
	"io"
	"testing"

	networktracer "github.com/DataDog/datadog-agent/comp/networktracer/def"
	"github.com/DataDog/datadog-agent/pkg/network"
)

// mockTracer is a controllable test implementation of the networktracer Component.
type mockTracer struct {
	activeConnections     func(clientID string) (*network.Connections, func(), error)
	registerClient        func(clientID string) error
	getStats              func() map[string]interface{}
	pause                 func() error
	resume                func() error
	debugNetworkMaps      func() (*network.Connections, error)
	debugNetworkState     func(clientID string) (interface{}, error)
	debugEBPFMaps         func(w io.Writer, maps ...string) error
	debugCachedConntrack  func(ctx context.Context, w io.Writer, maxSize int) error
	debugHostConntrack    func(ctx context.Context, w io.Writer, maxSize int) error
	debugDumpProcessCache func(ctx context.Context) (interface{}, error)
	isSupported           func() bool
}

// Mock returns a mock networktracer component that returns zero values by default.
// Fields on the returned mock can be overridden for specific test scenarios.
func Mock(_ *testing.T) networktracer.Component {
	return &mockTracer{
		activeConnections: func(_ string) (*network.Connections, func(), error) {
			return &network.Connections{}, func() {}, nil
		},
		registerClient:        func(_ string) error { return nil },
		getStats:              func() map[string]interface{} { return nil },
		pause:                 func() error { return nil },
		resume:                func() error { return nil },
		debugNetworkMaps:      func() (*network.Connections, error) { return &network.Connections{}, nil },
		debugNetworkState:     func(_ string) (interface{}, error) { return nil, nil },
		debugEBPFMaps:         func(_ io.Writer, _ ...string) error { return nil },
		debugCachedConntrack:  func(_ context.Context, _ io.Writer, _ int) error { return nil },
		debugHostConntrack:    func(_ context.Context, _ io.Writer, _ int) error { return nil },
		debugDumpProcessCache: func(_ context.Context) (interface{}, error) { return nil, nil },
		isSupported:           func() bool { return true },
	}
}

// GetActiveConnections implements Component.
func (m *mockTracer) GetActiveConnections(clientID string) (*network.Connections, func(), error) {
	return m.activeConnections(clientID)
}

// RegisterClient implements Component.
func (m *mockTracer) RegisterClient(clientID string) error {
	return m.registerClient(clientID)
}

// GetStats implements Component.
func (m *mockTracer) GetStats() map[string]interface{} {
	return m.getStats()
}

// Pause implements Component.
func (m *mockTracer) Pause() error {
	return m.pause()
}

// Resume implements Component.
func (m *mockTracer) Resume() error {
	return m.resume()
}

// DebugNetworkMaps implements Component.
func (m *mockTracer) DebugNetworkMaps() (*network.Connections, error) {
	return m.debugNetworkMaps()
}

// DebugNetworkState implements Component.
func (m *mockTracer) DebugNetworkState(clientID string) (interface{}, error) {
	return m.debugNetworkState(clientID)
}

// DebugEBPFMaps implements Component.
func (m *mockTracer) DebugEBPFMaps(w io.Writer, maps ...string) error {
	return m.debugEBPFMaps(w, maps...)
}

// DebugCachedConntrack implements Component.
func (m *mockTracer) DebugCachedConntrack(ctx context.Context, w io.Writer, maxSize int) error {
	return m.debugCachedConntrack(ctx, w, maxSize)
}

// DebugHostConntrack implements Component.
func (m *mockTracer) DebugHostConntrack(ctx context.Context, w io.Writer, maxSize int) error {
	return m.debugHostConntrack(ctx, w, maxSize)
}

// DebugDumpProcessCache implements Component.
func (m *mockTracer) DebugDumpProcessCache(ctx context.Context) (interface{}, error) {
	return m.debugDumpProcessCache(ctx)
}

// IsSupported implements Component.
func (m *mockTracer) IsSupported() bool {
	return m.isSupported()
}
