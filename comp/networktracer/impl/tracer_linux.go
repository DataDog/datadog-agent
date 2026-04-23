// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package networktracerimpl implements the networktracer component.
package networktracerimpl

import (
	"context"
	"fmt"
	"io"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networktracer "github.com/DataDog/datadog-agent/comp/networktracer/def"
	"github.com/DataDog/datadog-agent/pkg/network"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
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

// tracerImpl is the private implementation of the networktracer component.
type tracerImpl struct {
	t *tracer.Tracer
}

// NewComponent creates a new networktracer component backed by the eBPF tracer.
// If the current OS or kernel version does not support the tracer, a no-op component
// is returned instead of an error so that the fx graph can still start; the module
// factory will detect the unsupported case and skip module registration gracefully.
func NewComponent(reqs Requires) (Provides, error) {
	ncfg := networkconfig.New()

	if supported, reason := tracer.IsTracerSupportedByOS(ncfg.ExcludedBPFLinuxVersions); !supported {
		return Provides{Comp: &unsupportedKernelTracer{reason: reason}}, nil
	}

	t, err := tracer.NewTracer(ncfg, reqs.Telemetry, reqs.Statsd)
	if err != nil {
		return Provides{}, fmt.Errorf("could not create network tracer: %w", err)
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

// Pause suspends eBPF-based connection tracking.
func (ti *tracerImpl) Pause() error {
	return ti.t.Pause()
}

// Resume resumes eBPF-based connection tracking.
func (ti *tracerImpl) Resume() error {
	return ti.t.Resume()
}

// DebugNetworkMaps returns all active connections for debugging purposes.
func (ti *tracerImpl) DebugNetworkMaps() (*network.Connections, error) {
	return ti.t.DebugNetworkMaps()
}

// DebugNetworkState returns the internal connection state for the given client.
func (ti *tracerImpl) DebugNetworkState(clientID string) (interface{}, error) {
	return ti.t.DebugNetworkState(clientID)
}

// DebugEBPFMaps writes the content of all eBPF maps (or the subset named by maps) to w.
func (ti *tracerImpl) DebugEBPFMaps(w io.Writer, maps ...string) error {
	return ti.t.DebugEBPFMaps(w, maps...)
}

// DebugCachedConntrack writes the cached conntrack table to w, limited to maxSize entries.
func (ti *tracerImpl) DebugCachedConntrack(ctx context.Context, w io.Writer, maxSize int) error {
	table, err := ti.t.DebugCachedConntrack(ctx)
	if err != nil {
		return err
	}
	return table.WriteTo(w, maxSize)
}

// DebugHostConntrack writes the host conntrack table to w, limited to maxSize entries.
func (ti *tracerImpl) DebugHostConntrack(ctx context.Context, w io.Writer, maxSize int) error {
	table, err := ti.t.DebugHostConntrack(ctx)
	if err != nil {
		return err
	}
	return table.WriteTo(w, maxSize)
}

// DebugDumpProcessCache returns the content of the process cache.
func (ti *tracerImpl) DebugDumpProcessCache(ctx context.Context) (interface{}, error) {
	return ti.t.DebugDumpProcessCache(ctx)
}

// IsSupported returns true since this implementation wraps a live eBPF tracer.
func (ti *tracerImpl) IsSupported() bool { return true }

// unsupportedKernelTracer is returned when the current kernel does not satisfy
// the tracer's minimum version requirements. It is a no-op so that the fx graph
// can start successfully; the module factory detects the unsupported condition
// independently and skips module registration.
type unsupportedKernelTracer struct {
	reason error
}

func (u *unsupportedKernelTracer) GetActiveConnections(_ string) (*network.Connections, func(), error) {
	return nil, nil, fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) RegisterClient(_ string) error {
	return fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) GetStats() map[string]interface{} { return nil }

func (u *unsupportedKernelTracer) Pause() error {
	return fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) Resume() error {
	return fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) DebugNetworkState(_ string) (interface{}, error) {
	return nil, fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) DebugEBPFMaps(_ io.Writer, _ ...string) error {
	return fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) DebugCachedConntrack(_ context.Context, _ io.Writer, _ int) error {
	return fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) DebugHostConntrack(_ context.Context, _ io.Writer, _ int) error {
	return fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) DebugDumpProcessCache(_ context.Context) (interface{}, error) {
	return nil, fmt.Errorf("network tracer not supported: %w", u.reason)
}

func (u *unsupportedKernelTracer) IsSupported() bool { return false }
