// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import (
	"math"
	"os"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	minPathnamesEntries = 64000 // ~27 MB
	maxPathnamesEntries = 96000

	minProcEntries = 16394
	maxProcEntries = 131072
)

var (
	// allProbes contain the list of all the probes of the runtime security module
	allProbes []*manager.Probe
	// EventsPerfRingBufferSize is the buffer size of the perf buffers used for events.
	// PLEASE NOTE: for the perf ring buffer usage metrics to be accurate, the provided value must have the
	// following form: (1 + 2^n) * pages. Checkout https://github.com/DataDog/ebpf for more.
	EventsPerfRingBufferSize = 256 * os.Getpagesize()
)

// computeDefaultEventsRingBufferSize is the default buffer size of the ring buffers for events.
// Must be a power of 2 and a multiple of the page size
func computeDefaultEventsRingBufferSize() uint32 {
	numCPU, err := utils.NumCPU()
	if err != nil {
		numCPU = 1
	}

	if numCPU <= 16 {
		return uint32(8 * 256 * os.Getpagesize())
	} else if numCPU <= 64 {
		return uint32(16 * 256 * os.Getpagesize())
	}

	return uint32(32 * 256 * os.Getpagesize())
}

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
	allProbes = append(allProbes, iouringProbes...)
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
	allProbes = append(allProbes, getFlowProbes()...)
	allProbes = append(allProbes, getNetDeviceProbes()...)
	allProbes = append(allProbes, GetTCProbes()...)
	allProbes = append(allProbes, getBindProbes()...)
	allProbes = append(allProbes, getSyscallMonitorProbes()...)
	allProbes = append(allProbes, getPipeProbes()...)

	allProbes = append(allProbes,
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "sys_exit",
			},
		},
		// Snapshot probe
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "kprobe_security_inode_getattr",
			},
		},
	)

	return allProbes
}

// AllMaps returns the list of maps of the runtime security module
func AllMaps() []*manager.Map {
	return []*manager.Map{
		// Syscall table map
		getSyscallTableMap(),
		// Filters
		{Name: "filter_policy"},
		{Name: "inode_discarders"},
		{Name: "pid_discarders"},
		{Name: "inode_disc_revisions"},
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
		// Enabled event mask
		{Name: "enabled_events"},
	}
}

func getMaxEntries(numCPU int, min int, max int) uint32 {
	maxEntries := int(math.Min(float64(max), float64(min*numCPU)/4))
	if maxEntries < min {
		maxEntries = min
	}

	return uint32(maxEntries)
}

// MapSpecEditorOpts defines some options of the map spec editor
type MapSpecEditorOpts struct {
	TracedCgroupSize        int
	UseMmapableMaps         bool
	UseRingBuffers          bool
	RingBufferSize          uint32
	PathResolutionEnabled   bool
	SecurityProfileMaxCount int
}

// AllMapSpecEditors returns the list of map editors
func AllMapSpecEditors(numCPU int, opts MapSpecEditorOpts) map[string]manager.MapSpecEditor {
	editors := map[string]manager.MapSpecEditor{
		"proc_cache": {
			MaxEntries: getMaxEntries(numCPU, minProcEntries, maxProcEntries),
			EditorFlag: manager.EditMaxEntries,
		},
		"pid_cache": {
			MaxEntries: getMaxEntries(numCPU, minProcEntries, maxProcEntries),
			EditorFlag: manager.EditMaxEntries,
		},

		"activity_dumps_config": {
			MaxEntries: model.MaxTracedCgroupsCount,
			EditorFlag: manager.EditMaxEntries,
		},
		"activity_dump_rate_limiters": {
			MaxEntries: model.MaxTracedCgroupsCount,
			EditorFlag: manager.EditMaxEntries,
		},
		"cgroup_wait_list": {
			MaxEntries: model.MaxTracedCgroupsCount,
			EditorFlag: manager.EditMaxEntries,
		},
		"security_profiles": {
			MaxEntries: uint32(opts.SecurityProfileMaxCount),
			EditorFlag: manager.EditMaxEntries,
		},
		"secprofs_syscalls": {
			MaxEntries: uint32(opts.SecurityProfileMaxCount),
			EditorFlag: manager.EditMaxEntries,
		},
	}

	if opts.PathResolutionEnabled {
		editors["pathnames"] = manager.MapSpecEditor{
			MaxEntries: getMaxEntries(numCPU, minPathnamesEntries, maxPathnamesEntries),
			EditorFlag: manager.EditMaxEntries,
		}
	}

	if opts.TracedCgroupSize > 0 {
		editors["traced_cgroups"] = manager.MapSpecEditor{
			MaxEntries: uint32(opts.TracedCgroupSize),
			EditorFlag: manager.EditMaxEntries,
		}
	}

	if opts.UseMmapableMaps {
		editors["dr_erpc_buffer"] = manager.MapSpecEditor{
			Flags:      unix.BPF_F_MMAPABLE,
			EditorFlag: manager.EditFlags,
		}
	}
	if opts.UseRingBuffers {
		if opts.RingBufferSize == 0 {
			opts.RingBufferSize = computeDefaultEventsRingBufferSize()
		}
		editors["events"] = manager.MapSpecEditor{
			MaxEntries: opts.RingBufferSize,
			Type:       ebpf.RingBuf,
			EditorFlag: manager.EditMaxEntries | manager.EditType | manager.EditKeyValue,
		}
	}
	return editors
}

// AllPerfMaps returns the list of perf maps of the runtime security module
func AllPerfMaps() []*manager.PerfMap {
	return []*manager.PerfMap{
		{
			Map: manager.Map{Name: "events"},
		},
	}
}

// AllRingBuffers returns the list of ring buffers of the runtime security module
func AllRingBuffers() []*manager.RingBuffer {
	return []*manager.RingBuffer{
		{
			Map: manager.Map{Name: "events"},
		},
	}
}

// AllTailRoutes returns the list of all the tail call routes
func AllTailRoutes(ERPCDentryResolutionEnabled, networkEnabled, supportMmapableMaps bool) []manager.TailCallRoute {
	var routes []manager.TailCallRoute

	routes = append(routes, getExecTailCallRoutes()...)
	routes = append(routes, getDentryResolverTailCallRoutes(ERPCDentryResolutionEnabled, supportMmapableMaps)...)
	routes = append(routes, getSysExitTailCallRoutes()...)
	if networkEnabled {
		routes = append(routes, getTCTailCallRoutes()...)
	}

	return routes
}

// AllBPFProbeWriteUserProgramFunctions returns the list of program functions that use the bpf_probe_write_user helper
func AllBPFProbeWriteUserProgramFunctions() []string {
	return []string{
		"kprobe_dentry_resolver_erpc_write_user",
		"kprobe_dentry_resolver_parent_erpc_write_user",
		"kprobe_dentry_resolver_segment_erpc_write_user",
	}
}

// GetPerfBufferStatisticsMaps returns the list of maps used to monitor the performances of each perf buffers
func GetPerfBufferStatisticsMaps() map[string]string {
	return map[string]string{
		"events": "events_stats",
	}
}

// GetRingBufferStatisticsMaps returns the list of maps used to monitor the performances of each ring buffer
func GetRingBufferStatisticsMaps() map[string]string {
	return map[string]string{
		"events": "events_ringbuf_stats",
	}
}
