// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
)

// protocol holds the state of the postgres protocol monitoring.
// Currently, it is an empty struct.
type protocol struct{}

// Spec is the protocol spec for the postgres protocol.
var Spec = &protocols.ProtocolSpec{
	Factory: newPostgresProtocol,
}

func newPostgresProtocol(cfg *config.Config) (protocols.Protocol, error) {
	if !cfg.EnablePostgresMonitoring {
		return nil, nil
	}

	return &protocol{}, nil
}

// Name returns the name of the protocol.
func (p *protocol) Name() string {
	return "postgres"
}

// ConfigureOptions add the necessary options for the postgres monitoring to work, to be used by the manager.
// Currently, a no-op.
func (p *protocol) ConfigureOptions(*manager.Manager, *manager.Options) {
}

// PreStart runs setup required before starting the protocol.
// Currently, a no-op.
func (p *protocol) PreStart(*manager.Manager) error {
	return nil
}

// PostStart starts the map cleaner.
// Currently, a no-op.
func (p *protocol) PostStart(*manager.Manager) error {
	return nil
}

// Stop stops all resources associated with the protocol.
// Currently, a no-op.
func (p *protocol) Stop(*manager.Manager) {}

// DumpMaps dumps map contents for debugging.
// Currently, a no-op.
func (p *protocol) DumpMaps(io.Writer, string, *ebpf.Map) {}

// GetStats returns a map of Postgres stats.
// Currently, a no-op.
func (p *protocol) GetStats() *protocols.ProtocolStats {
	return &protocols.ProtocolStats{
		Type:  protocols.Postgres,
		Stats: nil,
	}
}

// IsBuildModeSupported returns always true, as postgres module is supported by all modes.
func (*protocol) IsBuildModeSupported(buildmode.Type) bool {
	return true
}
