// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpf holds ebpf related files
package ebpf

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
)

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
func NewRuntimeSecurityManager(supportsRingBuffers, useFentry bool) *manager.Manager {
	manager := &manager.Manager{
		Probes: probes.AllProbes(useFentry),
		Maps:   probes.AllMaps(),
	}
	if supportsRingBuffers {
		manager.RingBuffers = probes.AllRingBuffers()
	} else {
		manager.PerfMaps = probes.AllPerfMaps()
	}
	return manager
}
