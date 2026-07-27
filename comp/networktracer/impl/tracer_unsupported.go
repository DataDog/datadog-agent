// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf && !(windows && npm)

// Package networktracerimpl implements the networktracer component.
package networktracerimpl

import (
	"context"
	"errors"
	"io"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	networktracer "github.com/DataDog/datadog-agent/comp/networktracer/def"
	"github.com/DataDog/datadog-agent/pkg/network"
)

// ErrNotSupported is returned by the stub component on platforms where the
// eBPF-based network tracer is not available.
var ErrNotSupported = errors.New("network tracer is not supported on this platform")

// Requires defines the fx dependencies for the networktracer stub component.
type Requires struct {
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the networktracer stub component.
type Provides struct {
	Comp networktracer.Component
}

// unsupportedTracer is a no-op implementation returned on unsupported platforms.
type unsupportedTracer struct{}

// NewComponent creates a stub networktracer component that returns ErrNotSupported on all calls.
func NewComponent(_ Requires) (Provides, error) {
	return Provides{Comp: &unsupportedTracer{}}, nil
}

// GetActiveConnections always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) GetActiveConnections(_ string) (*network.Connections, func(), error) {
	return nil, nil, ErrNotSupported
}

// RegisterClient always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) RegisterClient(_ string) error {
	return ErrNotSupported
}

// GetStats always returns nil on unsupported platforms.
func (u *unsupportedTracer) GetStats() map[string]interface{} {
	return nil
}

// Pause always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) Pause() error {
	return ErrNotSupported
}

// Resume always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) Resume() error {
	return ErrNotSupported
}

// DebugNetworkMaps always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, ErrNotSupported
}

// DebugNetworkState always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) DebugNetworkState(_ string) (interface{}, error) {
	return nil, ErrNotSupported
}

// DebugEBPFMaps always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) DebugEBPFMaps(_ io.Writer, _ ...string) error {
	return ErrNotSupported
}

// DebugCachedConntrack always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) DebugCachedConntrack(_ context.Context, _ io.Writer, _ int) error {
	return ErrNotSupported
}

// DebugHostConntrack always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) DebugHostConntrack(_ context.Context, _ io.Writer, _ int) error {
	return ErrNotSupported
}

// DebugDumpProcessCache always returns ErrNotSupported on unsupported platforms.
func (u *unsupportedTracer) DebugDumpProcessCache(_ context.Context) (interface{}, error) {
	return nil, ErrNotSupported
}

// IsSupported always returns false on unsupported platforms.
func (u *unsupportedTracer) IsSupported() bool { return false }
