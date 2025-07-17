// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	"math"
	"os"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	minPathnamesEntries = 64000 // ~27 MB
	maxPathnamesEntries = 96000

	minProcEntries = 16384
	maxProcEntries = 131072
)

var (
	// EventsPerfRingBufferSize is the buffer size of the perf buffers used for events.
	// PLEASE NOTE: for the perf ring buffer usage metrics to be accurate, the provided value must have the
	// following form: (1 + 2^n) * pages. Checkout https://github.com/DataDog/ebpf for more.
	EventsPerfRingBufferSize = 256 * os.Getpagesize()
)

// see kernel definitions
func tailCallFnc(name string) string {
	return "tail_call_" + name
}

func tailCallTracepointFnc(name string) string {
	return "tail_call_tracepoint_" + name
}

func tailCallClassifierFnc(name string) string {
	return "tail_call_classifier_" + name
}

func appendSyscallProbes(probes []*manager.Probe, fentry bool, flag int, compat bool, syscalls ...string) []*manager.Probe {
	for _, syscall := range syscalls {
		probes = append(probes,
			ExpandSyscallProbes(&manager.Probe{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					UID: SecurityAgentUID,
				},
				SyscallFuncName: syscall,
			}, fentry, flag, compat)...)
	}

	return probes
}

// computeDefaultEventsRingBufferSize is the default buffer size of the ring buffers for events.
// Must be a power of 2 and a multiple of the page size
func computeDefaultEventsRingBufferSize() uint32 {
	numCPU, err := utils.NumCPU()
	if err != nil {
		numCPU = 1
	}

	if numCPU <= 16 {
		return uint32(8 * 256 * os.Getpagesize())
	}

	return uint32(16 * 256 * os.Getpagesize())
}

// AllProbes returns the list of all the probes of the runtime security module
func AllProbes(fentry bool, cgroup2MountPoint string) []*manager.Probe {
	var allProbes []*manager.Probe
	allProbes = append(allProbes, getAttrProbes(fentry)...)
	allProbes = append(allProbes, getExecProbes(fentry)...)
	allProbes = append(allProbes, getLinkProbe(fentry)...)
	allProbes = append(allProbes, getMkdirProbes(fentry)...)
	allProbes = append(allProbes, getMountProbes(fentry)...)
	allProbes = append(allProbes, getOpenProbes(fentry)...)
	allProbes = append(allProbes, getRenameProbes(fentry)...)
	allProbes = append(allProbes, getRmdirProbe(fentry)...)
	allProbes = append(allProbes, getSharedProbes()...)
	allProbes = append(allProbes, getIouringProbes()...)
	allProbes = append(allProbes, getUnlinkProbes(fentry)...)
	allProbes = append(allProbes, getXattrProbes(fentry)...)
	allProbes = append(allProbes, getIoctlProbes()...)
	allProbes = append(allProbes, getSELinuxProbes()...)
	allProbes = append(allProbes, getBPFProbes(fentry)...)
	allProbes = append(allProbes, getPTraceProbes(fentry)...)
	allProbes = append(allProbes, getMMapProbes()...)
	allProbes = append(allProbes, getMProtectProbes(fentry)...)
	allProbes = append(allProbes, getModuleProbes(fentry)...)
	allProbes = append(allProbes, getSignalProbes(fentry)...)
	allProbes = append(allProbes, getSpliceProbes(fentry)...)
	allProbes = append(allProbes, getFlowProbes()...)
	allProbes = append(allProbes, getNetDeviceProbes()...)
	allProbes = append(allProbes, GetTCProbes(true, true)...)
	allProbes = append(allProbes, getBindProbes(fentry)...)
	allProbes = append(allProbes, getAcceptProbes()...)
	allProbes = append(allProbes, getConnectProbes(fentry)...)
	allProbes = append(allProbes, getSyscallMonitorProbes()...)
	allProbes = append(allProbes, getChdirProbes(fentry)...)
	allProbes = append(allProbes, GetOnDemandProbes()...)
	allProbes = append(allProbes, GetPerfEventProbes()...)
	allProbes = append(allProbes, getSysCtlProbes(cgroup2MountPoint)...)
	allProbes = append(allProbes, getSetSockOptProbe(fentry)...)
	allProbes = append(allProbes, getSetrlimitProbes(fentry)...)

	allProbes = append(allProbes,
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "sys_exit",
			},
		},
	)

	// procfs fallback, used to get mount_id
	allProbes = append(allProbes,
		&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_inode_getattr",
			},
		},
	)
	allProbes = appendSyscallProbes(allProbes, fentry, EntryAndExit, false, "newfstatat")

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
		{Name: "inode_disc_revisions"},
		{Name: "basename_approvers"},
		// Dentry resolver table
		{Name: "pathnames"},
		// Procfs fallback table
		{Name: "inode_file"},
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
		// Syscall stats monitor (inflight syscall)
		{Name: "syscalls_stats_enabled"},
		{Name: "kill_list"},
		// used by raw packet filters
		{Name: "raw_packet_event"},
	}
}

// AllBPFForEachMapElemProgramFunctions returns the list of programs that leverage the bpf_for_each_map_elem helper
func AllBPFForEachMapElemProgramFunctions() []string {
	return []string{
		"network_stats_worker",
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
	TracedCgroupSize          int
	UseMmapableMaps           bool
	UseRingBuffers            bool
	RingBufferSize            uint32
	PathResolutionEnabled     bool
	SecurityProfileMaxCount   int
	ReducedProcPidCacheSize   bool
	NetworkFlowMonitorEnabled bool
	NetworkSkStorageEnabled   bool
	SpanTrackMaxCount         int
}

// AllMapSpecEditors returns the list of map editors
func AllMapSpecEditors(numCPU int, opts MapSpecEditorOpts, kv *kernel.Version) map[string]manager.MapSpecEditor {
	var procPidCacheMaxEntries uint32
	if opts.ReducedProcPidCacheSize {
		procPidCacheMaxEntries = getMaxEntries(numCPU, minProcEntries, maxProcEntries/2)
	} else {
		procPidCacheMaxEntries = getMaxEntries(numCPU, minProcEntries, maxProcEntries)
	}

	var activeFlowsMaxEntries, nsFlowToNetworkStats uint32
	if opts.NetworkFlowMonitorEnabled {
		activeFlowsMaxEntries = procPidCacheMaxEntries
		nsFlowToNetworkStats = 4096
	} else {
		activeFlowsMaxEntries = 1
		nsFlowToNetworkStats = 1
	}

	editors := map[string]manager.MapSpecEditor{
		"syscalls": {
			MaxEntries: 8192,
			EditorFlag: manager.EditMaxEntries,
		},
		"proc_cache": {
			MaxEntries: procPidCacheMaxEntries,
			EditorFlag: manager.EditMaxEntries,
		},
		"pid_cache": {
			MaxEntries: procPidCacheMaxEntries,
			EditorFlag: manager.EditMaxEntries,
		},
		"pid_rate_limiters": {
			MaxEntries: procPidCacheMaxEntries,
			EditorFlag: manager.EditMaxEntries,
		},
		"active_flows": {
			MaxEntries: activeFlowsMaxEntries,
			EditorFlag: manager.EditMaxEntries,
		},
		"active_flows_spin_locks": {
			MaxEntries: activeFlowsMaxEntries,
			EditorFlag: manager.EditMaxEntries,
		},
		"ns_flow_to_network_stats": {
			MaxEntries: nsFlowToNetworkStats,
			EditorFlag: manager.EditMaxEntries,
		},
		"inet_bind_args": {
			MaxEntries: procPidCacheMaxEntries,
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
		"span_tls": {
			MaxEntries: uint32(opts.SpanTrackMaxCount),
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

	if opts.NetworkSkStorageEnabled {
		// SK_Storage maps are enabled and available, delete fall back
		editors["sock_meta"] = manager.MapSpecEditor{
			Type:       ebpf.Hash,
			KeySize:    1,
			ValueSize:  1,
			MaxEntries: 1,
			EditorFlag: manager.EditKeyValue | manager.EditType | manager.EditMaxEntries,
		}
	} else {
		// Edit each SK_Storage map and transform them to a basic hash maps so they can be loaded by older kernels.
		// We need this so that the eBPF manager can link the SK_Storage maps in our eBPF programs, even if deadcode
		// elimination will clean up the piece of code that work with them prior to running the verifier.
		editors["sk_storage_meta"] = manager.MapSpecEditor{
			Type:       ebpf.Hash,
			KeySize:    1,
			ValueSize:  1,
			MaxEntries: 1,
			EditorFlag: manager.EditKeyValue | manager.EditType | manager.EditMaxEntries,
		}
	}

	if !kv.HasNoPreallocMapsInPerfEvent() {
		editors["active_flows"] = manager.MapSpecEditor{
			MaxEntries: activeFlowsMaxEntries,
			Flags:      unix.BPF_ANY,
			EditorFlag: manager.EditMaxEntries | manager.EditFlags,
		}
	} else {
		editors["active_flows"] = manager.MapSpecEditor{
			MaxEntries: activeFlowsMaxEntries,
			EditorFlag: manager.EditMaxEntries,
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
func AllTailRoutes(eRPCDentryResolutionEnabled, networkEnabled, networkFlowMonitorEnabled, rawPacketEnabled, supportMmapableMaps bool) []manager.TailCallRoute {
	var routes []manager.TailCallRoute

	routes = append(routes, getOpenTailCallRoutes()...)
	routes = append(routes, getExecTailCallRoutes()...)
	routes = append(routes, getDentryResolverTailCallRoutes(eRPCDentryResolutionEnabled, supportMmapableMaps)...)
	routes = append(routes, getSysExitTailCallRoutes()...)
	if networkEnabled {
		routes = append(routes, getTCTailCallRoutes(rawPacketEnabled)...)
	}
	if networkFlowMonitorEnabled {
		routes = append(routes, getFlushNetworkStatsTailCallRoutes()...)
	}

	return routes
}

// AllBPFProbeWriteUserProgramFunctions returns the list of program functions that use the bpf_probe_write_user helper
func AllBPFProbeWriteUserProgramFunctions() []string {
	return []string{
		tailCallFnc("dentry_resolver_erpc_write_user"),
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
