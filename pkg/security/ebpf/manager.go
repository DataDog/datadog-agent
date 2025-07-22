// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpf holds ebpf related files
package ebpf

import (
	"sync"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
)

// Manager handle the safeness of the ebpf manager
type Manager struct {
	sync.Mutex
	Manager *manager.Manager
}

// NewDefaultOptions returns a new instance of the default runtime security manager options
func NewDefaultOptions(kretprobeMaxActive int) manager.Options {
	return manager.Options{
		// DefaultKProbeMaxActive is the maximum number of active kretprobe at a given time
		DefaultKProbeMaxActive: kretprobeMaxActive,

		DefaultPerfRingBufferSize: probes.EventsPerfRingBufferSize,

		RemoveRlimit: true,
	}
}

// NewRuntimeSecurityManager returns a new instance of the runtime security module manager
func NewRuntimeSecurityManager(supportsRingBuffers bool) *manager.Manager {
	manager := &manager.Manager{
		Maps: probes.AllMaps(),
	}
	if supportsRingBuffers {
		manager.RingBuffers = probes.AllRingBuffers()
	} else {
		manager.PerfMaps = probes.AllPerfMaps()
	}
	return manager
}
