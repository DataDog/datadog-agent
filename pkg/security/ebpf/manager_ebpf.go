// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

// Package ebpf holds ebpf related files
package ebpf

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
)

// Manager is a wrapper type for pkg/ebpf/manager.Manager used when the linux_bpf build tag is set
type Manager struct {
	*ddebpf.Manager
}

// Get returns the ebpf-manager instance
func (m *Manager) Get() *manager.Manager {
	return m.Manager.Manager
}

// InitWithOptions wraps the call to corresponding ebpf-manager method
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts manager.Options) error {
	return m.Manager.InitWithOptions(bytecode, &opts)
}

// NewDefaultOptions returns a new instance of the default runtime security manager options
func NewDefaultOptions() manager.Options {
	return manager.Options{
		// DefaultKProbeMaxActive is the maximum number of active kretprobe at a given time
		DefaultKProbeMaxActive: 512,

		DefaultPerfRingBufferSize: probes.EventsPerfRingBufferSize,

		RemoveRlimit: true,
	}
}

// NewRuntimeSecurityManager returns a new instance of the runtime security module manager
func NewRuntimeSecurityManager(supportsRingBuffers bool, supportsTaskStorage bool) ManagerInterface {
	manager := &manager.Manager{
		Maps: probes.AllMaps(),
	}
	if supportsRingBuffers {
		manager.RingBuffers = probes.AllRingBuffers()
	} else {
		manager.PerfMaps = probes.AllPerfMaps()
	}
	var modifiers []ddebpf.Modifier
	if !supportsTaskStorage {
		modifiers = append(modifiers, ddebpf.NewHelperCallRemover(asm.FnTaskStorageGet, asm.FnTaskStorageDelete, asm.FnGetCurrentTaskBtf))
	}

	return &Manager{
		Manager: ddebpf.NewManagerWithDefault(manager, "cws", modifiers...),
	}
}
