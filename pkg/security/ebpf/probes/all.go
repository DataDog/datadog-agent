// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	"math"

	"github.com/DataDog/ebpf/manager"
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

	allProbes = append(allProbes,
		// Syscall monitor
		&manager.Probe{
			UID:     SecurityAgentUID,
			Section: "tracepoint/raw_syscalls/sys_enter",
		},
		&manager.Probe{
			UID:     SecurityAgentUID,
			Section: "tracepoint/raw_syscalls/sys_exit",
		},
		&manager.Probe{
			UID:     SecurityAgentUID,
			Section: "tracepoint/sched/sched_process_exec",
		},
		// Snapshot probe
		&manager.Probe{
			UID:     SecurityAgentUID,
			Section: "kprobe/security_inode_getattr",
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

// AllMapSpecEditors returns the list of map editors
func AllMapSpecEditors(numCPU int) map[string]manager.MapSpecEditor {
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
			// max 600,000 | min 64,000 entrie => max ~180 MB | min ~18 MB
			MaxEntries: uint32(math.Max(math.Min(640000, float64(64000*numCPU/4)), 64000)),
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
func AllTailRoutes() []manager.TailCallRoute {
	var routes []manager.TailCallRoute

	routes = append(routes, getExecTailCallRoutes()...)
	routes = append(routes, getDentryResolverTailCallRoutes()...)
	routes = append(routes, getSysExitTailCallRoutes()...)

	return routes
}

// GetPerfBufferStatisticsMaps returns the list of maps used to monitor the performances of each perf buffers
func GetPerfBufferStatisticsMaps() map[string]string {
	return map[string]string{
		"events": "events_stats",
	}
}
