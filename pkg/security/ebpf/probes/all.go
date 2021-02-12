// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

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
			Section: "kretprobe/get_task_exe_file",
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
		{Name: "inode_info_cache"},
		// Open tables
		{Name: "open_flags_approvers"},
		// Exec tables
		{Name: "proc_cache"},
		{Name: "pid_cache"},
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

// AllPerfMaps returns the list of perf maps of the runtime security module
func AllPerfMaps() []*manager.PerfMap {
	return []*manager.PerfMap{
		{
			Map: manager.Map{Name: "events"},
		},
	}
}

// GetPerfBufferStatisticsMaps returns the list of maps used to monitor the performances of each perf buffers
func GetPerfBufferStatisticsMaps() map[string]string {
	return map[string]string{
		"events": "events_stats",
	}
}
