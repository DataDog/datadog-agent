// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

// Package networktracerimpl implements the networktracer component.
package networktracerimpl

import (
	"context"
	"fmt"
	"io"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networktracer "github.com/DataDog/datadog-agent/comp/networktracer/def"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the fx dependencies for the networktracer component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Telemetry telemetry.Component
	Statsd    ddgostatsd.ClientInterface
}

// Provides defines the output of the networktracer component.
type Provides struct {
	Comp networktracer.Component
}

// tracerImpl is the private implementation of the networktracer component on Windows.
type tracerImpl struct {
	t *tracer.Tracer
}

// unavailableTracer is returned when the Windows NPM driver cannot be started. It
// satisfies the networktracer.Component interface with IsSupported() == false so that
// the module factory can detect the failure and skip module registration gracefully,
// allowing system-probe to continue starting normally.
type unavailableTracer struct {
	reason error
}

// NewComponent creates a new networktracer component backed by the Windows NPM tracer.
// If the NPM driver fails to start (e.g. it is not installed or is currently unavailable),
// a no-op component is returned instead of propagating the error, so the fx graph can
// still start; the module factory detects the unsupported condition via IsSupported() and
// skips module registration gracefully.
func NewComponent(reqs Requires) (Provides, error) {
	// driver.Init must be called before tracer.NewTracer so that the driver
	// reference counter is ready before driver.Start() is invoked inside NewTracer.
	// In the legacy module-factory path this was done by loader_windows.go's
	// preRegister hook; the component constructor runs earlier (during fx
	// dependency resolution), so we must initialize the driver here instead.
	if err := driver.Init(); err != nil {
		log.Warnf("could not initialize Windows NPM driver subsystem, NPM will be disabled: %v", err)
		return Provides{Comp: &unavailableTracer{reason: err}}, nil
	}

	ncfg := networkconfig.New()

	t, err := tracer.NewTracer(ncfg, reqs.Telemetry, reqs.Statsd)
	if err != nil {
		log.Warnf("could not create Windows network tracer, NPM will be disabled: %v", err)
		return Provides{Comp: &unavailableTracer{reason: err}}, nil
	}

	impl := &tracerImpl{t: t}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error { return nil },
		OnStop: func(context.Context) error {
			impl.t.Stop()
			return nil
		},
	})

	return Provides{Comp: impl}, nil
}

func (u *unavailableTracer) GetActiveConnections(_ string) (*network.Connections, func(), error) {
	return nil, nil, fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) RegisterClient(_ string) error {
	return fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) GetStats() map[string]interface{} { return nil }

func (u *unavailableTracer) Pause() error {
	return fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) Resume() error {
	return fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) DebugNetworkState(_ string) (interface{}, error) {
	return nil, fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) DebugEBPFMaps(_ io.Writer, _ ...string) error {
	return fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) DebugCachedConntrack(_ context.Context, _ io.Writer, _ int) error {
	return fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) DebugHostConntrack(_ context.Context, _ io.Writer, _ int) error {
	return fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) DebugDumpProcessCache(_ context.Context) (interface{}, error) {
	return nil, fmt.Errorf("network tracer unavailable: %w", u.reason)
}

func (u *unavailableTracer) IsSupported() bool { return false }

// GetActiveConnections returns active network connections for the given clientID.
func (ti *tracerImpl) GetActiveConnections(clientID string) (*network.Connections, func(), error) {
	return ti.t.GetActiveConnections(clientID)
}

// RegisterClient registers a new client for connection delta tracking.
func (ti *tracerImpl) RegisterClient(clientID string) error {
	return ti.t.RegisterClient(clientID)
}

// GetStats returns internal tracer statistics.
func (ti *tracerImpl) GetStats() map[string]interface{} {
	stats, _ := ti.t.GetStats()
	return stats
}

// Pause is not implemented on Windows.
func (ti *tracerImpl) Pause() error {
	return ebpf.ErrNotImplemented
}

// Resume is not implemented on Windows.
func (ti *tracerImpl) Resume() error {
	return ebpf.ErrNotImplemented
}

// DebugNetworkMaps is not implemented on Windows.
func (ti *tracerImpl) DebugNetworkMaps() (*network.Connections, error) {
	return ti.t.DebugNetworkMaps()
}

// DebugNetworkState is not implemented on Windows.
func (ti *tracerImpl) DebugNetworkState(clientID string) (interface{}, error) {
	result, err := ti.t.DebugNetworkState(clientID)
	return result, err
}

// DebugEBPFMaps is not implemented on Windows.
func (ti *tracerImpl) DebugEBPFMaps(w io.Writer, maps ...string) error {
	return ti.t.DebugEBPFMaps(w, maps...)
}

// DebugCachedConntrack is not implemented on Windows.
func (ti *tracerImpl) DebugCachedConntrack(_ context.Context, _ io.Writer, _ int) error {
	return ebpf.ErrNotImplemented
}

// DebugHostConntrack is not implemented on Windows.
func (ti *tracerImpl) DebugHostConntrack(_ context.Context, _ io.Writer, _ int) error {
	return ebpf.ErrNotImplemented
}

// DebugDumpProcessCache returns the content of the process cache.
func (ti *tracerImpl) DebugDumpProcessCache(ctx context.Context) (interface{}, error) {
	return ti.t.DebugDumpProcessCache(ctx)
}

// IsSupported returns true since this implementation wraps a live Windows NPM tracer.
func (ti *tracerImpl) IsSupported() bool { return true }
