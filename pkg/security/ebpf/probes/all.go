// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import (
	"math"

	manager "github.com/DataDog/ebpf-manager"
)

// allProbes contain the list of all the probes of the runtime security module
var allProbes []*manager.Probe

// AllProbes returns the list of all the probes of the runtime security module
func AllProbes() []*manager.Probe {
	if len(allProbes) > 0 {
		return allProbes
	}

	allProbes = append(allProbes, getAttrProbes()...)
	allProbes = append(allProbes, getExecProbes()...)
	allProbes = append(allProbes, getLinkProbe()...)
	allProbes = append(allProbes, getMkdirProbes()...)
	allProbes = append(allProbes, getMountProbes()...)
	allProbes = append(allProbes, getOpenProbes()...)
	allProbes = append(allProbes, getRenameProbes()...)
	allProbes = append(allProbes, getRmdirProbe()...)
	allProbes = append(allProbes, sharedProbes...)
	allProbes = append(allProbes, getUnlinkProbes()...)
	allProbes = append(allProbes, getXattrProbes()...)
	allProbes = append(allProbes, getIoctlProbes()...)
	allProbes = append(allProbes, getSELinuxProbes()...)
	allProbes = append(allProbes, getBPFProbes()...)
	allProbes = append(allProbes, getPTraceProbes()...)
	allProbes = append(allProbes, getMMapProbes()...)
	allProbes = append(allProbes, getMProtectProbes()...)
	allProbes = append(allProbes, getModuleProbes()...)
	allProbes = append(allProbes, getSignalProbes()...)
	allProbes = append(allProbes, getSpliceProbes()...)

	allProbes = append(allProbes,
		// Syscall monitor
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFSection:  "tracepoint/raw_syscalls/sys_enter",
				EBPFFuncName: "sys_enter",
			},
		},
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFSection:  "tracepoint/raw_syscalls/sys_exit",
				EBPFFuncName: "sys_exit",
			},
		},
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFSection:  "tracepoint/sched/sched_process_exec",
				EBPFFuncName: "sched_process_exec",
			},
		},
		// Snapshot probe
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFSection:  "kprobe/security_inode_getattr",
				EBPFFuncName: "kprobe_security_inode_getattr",
			},
		},
	)

	return allProbes
}

// AllMaps returns the list of maps of the runtime security module
func AllMaps() []*manager.Map {
	return []*manager.Map{
		// Filters
		{Name: "filter_policy"},
		{Name: "inode_discarders"},
		{Name: "pid_discarders"},
		{Name: "discarder_revisions"},
		{Name: "basename_approvers"},
		// Dentry resolver table
		{Name: "pathnames"},
		// Snapshot table
		{Name: "exec_file_cache"},
		// Open tables
		{Name: "open_flags_approvers"},
		// Exec tables
		{Name: "proc_cache"},
		{Name: "pid_cache"},
		{Name: "str_array_buffers"},
		// SELinux tables
		{Name: "selinux_write_buffer"},
		{Name: "selinux_enforce_status"},
		// Syscall monitor tables
		{Name: "buffer_selector"},
		{Name: "noisy_processes_fb"},
		{Name: "noisy_processes_bb"},
		// Flushing discarders boolean
		{Name: "flushing_discarders"},
		// Enabled event mask
		{Name: "enabled_events"},
	}
}

const (
	// MaxTracedCgroupsCount hard limit for the count of traced cgroups
	MaxTracedCgroupsCount = 1000
)

// AllMapSpecEditors returns the list of map editors
func AllMapSpecEditors(numCPU int, tracedCgroupsCount int, cgroupsWaitListSize int) map[string]manager.MapSpecEditor {
	if tracedCgroupsCount <= 0 || tracedCgroupsCount > MaxTracedCgroupsCount {
		tracedCgroupsCount = MaxTracedCgroupsCount
	}
	if cgroupsWaitListSize <= 0 || cgroupsWaitListSize > MaxTracedCgroupsCount {
		tracedCgroupsCount = MaxTracedCgroupsCount
	}
	return map[string]manager.MapSpecEditor{
		"proc_cache": {
			MaxEntries: uint32(4096 * numCPU),
			EditorFlag: manager.EditMaxEntries,
		},
		"pid_cache": {
			MaxEntries: uint32(4096 * numCPU),
			EditorFlag: manager.EditMaxEntries,
		},
		"pathnames": {
			// max 600,000 | min 64,000 entrie => max ~180 MB | min ~27 MB
			MaxEntries: uint32(math.Max(math.Min(640000, float64(64000*numCPU/4)), 96000)),
			EditorFlag: manager.EditMaxEntries,
		},
		"traced_cgroups": {
			MaxEntries: uint32(tracedCgroupsCount),
			EditorFlag: manager.EditMaxEntries,
		},
		"cgroups_wait_list": {
			MaxEntries: uint32(cgroupsWaitListSize),
			EditorFlag: manager.EditMaxEntries,
		},
	}
}

// AllPerfMaps returns the list of perf maps of the runtime security module
func AllPerfMaps() []*manager.PerfMap {
	return []*manager.PerfMap{
		{
			Map: manager.Map{Name: "events"},
		},
	}
}

// AllTailRoutes returns the list of all the tail call routes
func AllTailRoutes(ERPCDentryResolutionEnabled bool) []manager.TailCallRoute {
	var routes []manager.TailCallRoute

	routes = append(routes, getExecTailCallRoutes()...)
	routes = append(routes, getDentryResolverTailCallRoutes(ERPCDentryResolutionEnabled)...)
	routes = append(routes, getSysExitTailCallRoutes()...)

	return routes
}

// AllBPFProbeWriteUserProgramFunctions returns the list of program functions that use the bpf_probe_write_user helper
func AllBPFProbeWriteUserProgramFunctions() []string {
	return []string{
		"kprobe_dentry_resolver_erpc",
		"kprobe_dentry_resolver_parent_erpc",
		"kprobe_dentry_resolver_segment_erpc",
	}
}

// GetPerfBufferStatisticsMaps returns the list of maps used to monitor the performances of each perf buffers
func GetPerfBufferStatisticsMaps() map[string]string {
	return map[string]string{
		"events": "events_stats",
	}
}
