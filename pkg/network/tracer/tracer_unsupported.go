// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && !linux_bpf) || (windows && !npm) || (!linux && !windows)

package tracer

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// Tracer is not implemented
type Tracer struct{}

// NewTracer is not implemented on this OS for Tracer
func NewTracer(_ *config.Config) (*Tracer, error) {
	return nil, ebpf.ErrNotImplemented
}

// Stop is not implemented on this OS for Tracer
func (t *Tracer) Stop() {}

// GetActiveConnections is not implemented on this OS for Tracer
func (t *Tracer) GetActiveConnections(_ string) (*network.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// RegisterClient registers the client
func (t *Tracer) RegisterClient(clientID string) error {
	return ebpf.ErrNotImplemented
}

// GetStats is not implemented on this OS for Tracer
func (t *Tracer) GetStats() (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugNetworkState is not implemented on this OS for Tracer
func (t *Tracer) DebugNetworkState(clientID string) (map[string]interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugNetworkMaps is not implemented on this OS for Tracer
func (t *Tracer) DebugNetworkMaps() (*network.Connections, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugEBPFMaps is not implemented on this OS for Tracer
func (t *Tracer) DebugEBPFMaps(maps ...string) (string, error) {
	return "", ebpf.ErrNotImplemented
}

// DebugCachedConntrack is not implemented on this OS for Tracer
func (t *Tracer) DebugCachedConntrack(ctx context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugHostConntrack is not implemented on this OS for Tracer
func (t *Tracer) DebugHostConntrack(ctx context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}

// DebugDumpProcessCache is not implemented on this OS for Tracer
func (t *Tracer) DebugDumpProcessCache(ctx context.Context) (interface{}, error) {
	return nil, ebpf.ErrNotImplemented
}
