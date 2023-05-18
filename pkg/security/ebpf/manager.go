// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"math"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
)

// NewDefaultOptions returns a new instance of the default runtime security manager options
func NewDefaultOptions() manager.Options {
	return manager.Options{
		// DefaultKProbeMaxActive is the maximum number of active kretprobe at a given time
		DefaultKProbeMaxActive: 512,

		DefaultPerfRingBufferSize: probes.EventsPerfRingBufferSize,

		VerifierOptions: ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				// LogSize is the size of the log buffer given to the verifier. Give it a big enough (2 * 1024 * 1024)
				// value so that all our programs fit. If the verifier ever outputs a `no space left on device` error,
				// we'll need to increase this value.
				LogSize: 2097152,
			},
		},

		// Extend RLIMIT_MEMLOCK (8) size
		// On some systems, the default for RLIMIT_MEMLOCK may be as low as 64 bytes.
		// This will result in an EPERM (Operation not permitted) error, when trying to create an eBPF map
		// using bpf(2) with BPF_MAP_CREATE.
		//
		// We are setting the limit to infinity until we have a better handle on the true requirements.
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
	}
}

// NewRuntimeSecurityManager returns a new instance of the runtime security module manager
func NewRuntimeSecurityManager(supportsRingBuffers bool) *manager.Manager {
	manager := &manager.Manager{
		Probes: probes.AllProbes(),
		Maps:   probes.AllMaps(),
	}
	if supportsRingBuffers {
		manager.RingBuffers = probes.AllRingBuffers()
	} else {
		manager.PerfMaps = probes.AllPerfMaps()
	}
	return manager
}
