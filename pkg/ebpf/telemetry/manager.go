// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Manager wraps ebpf-manager.Manager to transparently handle eBPF telemetry
type Manager struct {
	*manager.Manager
	bpfTelemetry *EBPFTelemetry
}

// NewManager creates a Manager
func NewManager(mgr *manager.Manager, bt *EBPFTelemetry) *Manager {
	return &Manager{
		Manager:      mgr,
		bpfTelemetry: bt,
	}
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts manager.Options) error {
	if err := setupForTelemetry(m.Manager, &opts, m.bpfTelemetry); err != nil {
		return err
	}

	log.Debugf("adamk Initializing eBPF manager with manager properties: %+v", m.RingBuffers)
	log.Debugf("adamk Initializing eBPF manager with options: %+v", opts)
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
