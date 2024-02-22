// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"
)

// Manager wraps ebpf-manager.Manager to transparently handle eBPF telemetry
// Deprecated: The telemetry manager wrapper should no longer be used. Instead, use ebpf/manager.Manager instead with the ErrorsTelemetryModifier
type Manager struct {
	*manager.Manager
	bpfTelemetry *EBPFTelemetry
}

// NewManager creates a Manager
// Deprecated: The telemetry manager wrapper should no longer be used. Instead, use ebpf/manager.Manager instead with the ErrorsTelemetryModifier
func NewManager(mgr *manager.Manager, bt *EBPFTelemetry) *Manager {
	return &Manager{
		Manager:      mgr,
		bpfTelemetry: bt,
	}
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
// Deprecated: The telemetry manager wrapper should no longer be used. Instead, use ebpf/manager.Manager instead with the ErrorsTelemetryModifier
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts manager.Options) error {
	if err := setupForTelemetry(m.Manager, &opts, m.bpfTelemetry); err != nil {
		return err
	}

	if err := m.Manager.InitWithOptions(bytecode, opts); err != nil {
		return err
	}

	if m.bpfTelemetry != nil {
		if err := m.bpfTelemetry.populateMapsWithKeys(m.Manager); err != nil {
			return err
		}
	}
	return nil
}

// ErrorsTelemetryModifier is a modifier that sets up the manager to handle eBPF telemetry.
type ErrorsTelemetryModifier struct{}

// String returns the name of the modifier.
func (t *ErrorsTelemetryModifier) String() string {
	return "ErrorsTelemetryModifier"
}

// BeforeInit sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
func (t *ErrorsTelemetryModifier) BeforeInit(m *manager.Manager, opts *manager.Options) error {
	return setupForTelemetry(m, opts, errorsTelemetry)
}

// AfterInit pre-populates the telemetry maps with entries corresponding to the ebpf program of the manager.
func (t *ErrorsTelemetryModifier) AfterInit(m *manager.Manager, _ *manager.Options) error {
	if errorsTelemetry != nil {
		if err := errorsTelemetry.populateMapsWithKeys(m); err != nil {
			return err
		}
	}
	return nil
}
