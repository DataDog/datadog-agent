// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !linux_bpf

// Package kernel holds kernel related files
package kernel

// SupportCORE returns is CORE is supported
// here it's not, since we are built without eBPF support
func (k *Version) SupportCORE() bool {
	return false
}

// HasSKStorage returns true if the kernel supports SK_STORAGE maps
// here it's not, since we are built without eBPF support
func (k *Version) HasSKStorage() bool {
	return false
}

// HaveRingBuffers returns whether the kernel supports ring buffer.
// here it's not, since we are built without eBPF support
func (k *Version) HaveRingBuffers() bool {
	return false
}

// HaveFentrySupport returns whether the kernel supports fentry probes
// here it's not, since we are built without eBPF support
func (k *Version) HaveFentrySupport() bool {
	return false
}

// HaveFentrySupportWithStructArgs returns whether the kernel supports fentry probes with struct arguments
// here it's not, since we are built without eBPF support
func (k *Version) HaveFentrySupportWithStructArgs() bool {
	return false
}

// HaveFentryNoDuplicatedWeakSymbols returns whether the kernel supports fentry probes with struct arguments
// here it's not, since we are built without eBPF support
func (k *Version) HaveFentryNoDuplicatedWeakSymbols() bool {
	return false
}

// HasBPFForEachMapElemHelper returns true if the kernel support the bpf_for_each_map_elem helper
// here it's not, since we are built without eBPF support
func (k *Version) HasBPFForEachMapElemHelper() bool {
	return false
}

// HaveMmapableMaps returns whether the kernel supports mmapable maps.
// here it's not, since we are built without eBPF support
func (k *Version) HaveMmapableMaps() bool {
	return false
}

// HasCgroupSysctlSupportWithRingbuf returns true if cgroup/sysctl programs are available with access to ringbuffer
// here it's not, since we are built without eBPF support
func (k *Version) HasCgroupSysctlSupportWithRingbuf() bool {
	return false
}

// HasSKStorageInTracingPrograms returns true if the kernel supports SK_STORAGE maps in tracing programs
// here it's not, since we are built without eBPF support
func (k *Version) HasSKStorageInTracingPrograms() bool {
	return false
}

// HasTracingHelpersInCgroupSysctlPrograms returns true if basic tracing helpers are available in cgroup/sysctl programs
// here it's not, since we are built without eBPF support
func (k *Version) HasTracingHelpersInCgroupSysctlPrograms() bool {
	return false
}

// HasBpfGetCurrentPidTgidForSchedCLS returns true if the kernel supports bpf_get_current_pid_tgid for Sched CLS program type
// https://github.com/torvalds/linux/commit/eb166e522c77699fc19bfa705652327a1e51a117
func (k *Version) HasBpfGetCurrentPidTgidForSchedCLS() bool {
	return false
}
