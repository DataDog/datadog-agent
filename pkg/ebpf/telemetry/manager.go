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
type Manager struct {
	*manager.Manager
}

// NewManager creates a Manager
func NewManager(mgr *manager.Manager) *Manager {
	return &Manager{
		Manager: mgr,
	}
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts manager.Options) error {
	if err := setupForTelemetry(m.Manager, &opts); err != nil {
		return err
	}

	if err := m.Manager.InitWithOptions(bytecode, opts); err != nil {
		return err
	}

	if bpfTelemetry != nil {
		if err := bpfTelemetry.populateMapsWithKeys(m.Manager); err != nil {
			return err
		}
	}
	return nil
}
