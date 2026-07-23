// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"fmt"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// 5.13 for kfuncs, 6.2 for bpf_rcu_read_lock kfunc
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// Config holds noisy_neighbor-specific runtime knobs read from system-probe
// config. PMUMetrics keys are the BPF perf-event-array map names (e.g.
// "cycles_pmu"); a missing key or a false value disables the corresponding
// counter: no perf fds are opened for it, the eBPF program reads -ENOENT
// and skips the deltas for that event.
type Config struct {
	PMUMetrics map[string]bool
}

// Probe is the eBPF side of the noisy neighbor check
type Probe struct {
	mgr    *ddebpf.Manager
	pmuFDs []int
}

// NewProbe creates a [Probe]
func NewProbe(cfg *ddebpf.Config, modCfg Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %w", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	p := &Probe{}

	filename := "noisy-neighbor.o"
	if cfg.BPFDebug {
		filename = "noisy-neighbor-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		p.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{}, "noisy_neighbor", &ebpftelemetry.ErrorsTelemetryModifier{})
		const uid = "noisy"
		p.mgr.Probes = []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_wakeup", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_wakeup_new", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_switch", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_softirq_entry", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_softirq_exit", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_block_rq_issue", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "runq_enqueued"},
			{Name: "task_oncpu_pmu"},
			{Name: "cgroup_agg_stats"},
			{Name: "cycles_pmu"},
			{Name: "instructions_pmu"},
			{Name: "cache_misses_pmu"},
			{Name: "itlb_misses_pmu"},
			{Name: "branch_misses_pmu"},
			{Name: "cpu_migrations_pmu"},
			{Name: "cache_references_pmu"},
			{Name: "softirq_start_ns"},
		}
		if err := p.mgr.InitWithOptions(buf, &opts); err != nil {
			return fmt.Errorf("failed to init ebpf manager: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Manually attach probes instead of calling p.mgr.Start().
	// Start() runs cleanupTraceFS() which tries to remove ALL orphaned kprobe
	// events (not just ours) and fails fatally if any are pinned by a dead
	// process. Since this module only uses tracepoints—no kprobes—we skip
	// Start() and attach directly. Init already loaded programs and created maps.
	for _, probe := range p.mgr.Probes {
		if err := probe.Attach(); err != nil {
			return nil, fmt.Errorf("failed to attach probe %s: %w", probe.EBPFFuncName, err)
		}
	}
	ddebpf.AddNameMappings(p.mgr.Manager, "noisy_neighbor")
	p.attachPMU(modCfg.PMUMetrics)
	return p, nil
}

// pmuEvent describes one perf event (hardware or software counter) that we
// open per-CPU and expose to BPF via a perf-event-array map.
type pmuEvent struct {
	mapName    string
	humanLabel string
	perfType   uint32
	perfConfig uint64
}

// iTLB-load-misses encoded for PERF_TYPE_HW_CACHE: cache_id | (op << 8) | (result << 16).
const itlbLoadMissesConfig = uint64(unix.PERF_COUNT_HW_CACHE_ITLB) |
	(uint64(unix.PERF_COUNT_HW_CACHE_OP_READ) << 8) |
	(uint64(unix.PERF_COUNT_HW_CACHE_RESULT_MISS) << 16)

// attachPMU opens per-CPU perf events for every counter enabled via
// pmuEnabled (keyed by BPF map name) and populates the corresponding BPF
// perf-event-array maps. Events absent from the map or set to false are
// skipped — no perf fds are opened, the eBPF read returns -ENOENT, and
// deltas for that event are dropped. See attachPMUEvent for per-event
// semantics and failure modes.
//
// The CPU list comes from kernel.OnlineCPUs() rather than runtime.NumCPU().
// runtime.NumCPU() returns the size of the caller's sched_getaffinity mask,
// so on a cpuset-restricted system-probe (e.g. a Guaranteed-QoS pod under
// the kubelet static CPU manager) it can be smaller than the host CPU id
// range. Tracepoints fire on every online CPU regardless of the loader's
// affinity, so any host CPU missing from the perf-event-array reads -ENOENT
// on the BPF side and silently undercounts.
//
// CPU hotplug is not handled: the online set is snapshotted once here. A
// CPU brought online after init will have an empty map slot and its PMU
// samples will be dropped until the probe is reloaded.
func (p *Probe) attachPMU(pmuEnabled map[string]bool) {
	cpus, err := kernel.OnlineCPUs()
	if err != nil {
		log.Warnf("noisy_neighbor: cannot enumerate online CPUs (%v); PMU counters disabled", err)
		return
	}
	events := []pmuEvent{
		{mapName: "cycles_pmu", humanLabel: "cycles", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_CPU_CYCLES},
		{mapName: "instructions_pmu", humanLabel: "instructions", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_INSTRUCTIONS},
		{mapName: "cache_misses_pmu", humanLabel: "cache misses", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_CACHE_MISSES},
		{mapName: "itlb_misses_pmu", humanLabel: "iTLB misses", perfType: unix.PERF_TYPE_HW_CACHE, perfConfig: itlbLoadMissesConfig},
		{mapName: "branch_misses_pmu", humanLabel: "branch misses", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_BRANCH_MISSES},
		// CPU migrations is a software counter — doesn't compete for hardware
		// PMU counters and is always available regardless of µarch.
		{mapName: "cpu_migrations_pmu", humanLabel: "CPU migrations", perfType: unix.PERF_TYPE_SOFTWARE, perfConfig: unix.PERF_COUNT_SW_CPU_MIGRATIONS},
		// Cache references pairs with cache_misses to give cache hit-rate: under
		// cache thrashing the rate moves even when absolute miss count stays
		// flat for memory-bound workloads already at floor miss rate.
		{mapName: "cache_references_pmu", humanLabel: "cache references", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_CACHE_REFERENCES},
	}
	for _, ev := range events {
		if !pmuEnabled[ev.mapName] {
			log.Debugf("noisy_neighbor: %s PMU event disabled by config, skipping", ev.humanLabel)
			continue
		}
		p.attachPMUEvent(ev, cpus)
	}
}

// attachPMUEvent opens one perf event per online CPU and inserts the fds
// into the associated BPF perf-event-array map at the host CPU id (matching
// BPF_F_CURRENT_CPU on the reader side). On hosts where the event isn't
// supported (virtualized envs, restrictive perf_event_paranoid, missing
// CAP_PERF_MON, or µarch lacking the event) opens fail and we log once per
// event type. Other events still attach independently, and the eBPF program
// checks each read-value return code and skips just that counter's deltas.
func (p *Probe) attachPMUEvent(ev pmuEvent, cpus []uint) {
	m, _, err := p.mgr.GetMap(ev.mapName)
	if err != nil {
		log.Warnf("noisy_neighbor: %s map (%s) lookup failed: %v", ev.humanLabel, ev.mapName, err)
		return
	}
	if m == nil {
		log.Warnf("noisy_neighbor: %s map (%s) not registered in manager", ev.humanLabel, ev.mapName)
		return
	}
	var logged bool
	for _, cpu := range cpus {
		fd, err := openHardwarePerfEvent(ev.perfType, ev.perfConfig, int(cpu))
		if err != nil {
			if !logged {
				log.Warnf("noisy_neighbor: %s PMU event unavailable on cpu %d, metric will be zero for that CPU: %v", ev.humanLabel, cpu, err)
				logged = true
			}
			continue
		}
		if err := m.Put(uint32(cpu), uint32(fd)); err != nil {
			log.Warnf("noisy_neighbor: failed to register %s fd on cpu %d: %v", ev.humanLabel, cpu, err)
			_ = unix.Close(fd)
			continue
		}
		p.pmuFDs = append(p.pmuFDs, fd)
	}
}

func openHardwarePerfEvent(perfType uint32, config uint64, cpu int) (int, error) {
	attr := &unix.PerfEventAttr{
		Type:   perfType,
		Config: config,
		Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
	}
	fd, err := unix.PerfEventOpen(attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return -1, err
	}
	if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_ENABLE, 0); err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("enable perf event: %w", err)
	}
	return fd, nil
}

// Close releases all associated resources
func (p *Probe) Close() {
	for _, fd := range p.pmuFDs {
		if err := unix.Close(fd); err != nil {
			log.Warnf("noisy_neighbor: failed to close PMU fd %d: %v", fd, err)
		}
	}
	p.pmuFDs = nil
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping ebpf manager: %s", err)
		}
	}
}

// GetAndFlush returns aggregated per-cgroup stats for the current window
// and atomically deletes each returned cgroup's entry from the BPF map.
// Cgroups with zero observable activity (no events, softirq, block-IO,
// or wakeups) are filtered out of the return slice but still flushed.
func (p *Probe) GetAndFlush() []model.NoisyNeighborStats {
	var nnstats []model.NoisyNeighborStats

	aggMap, found, err := p.mgr.GetMap("cgroup_agg_stats")
	if err != nil {
		log.Errorf("failed to get cgroup_agg_stats map: %v", err)
		return nnstats
	}
	if !found {
		log.Warn("cgroup_agg_stats map not found")
		return nnstats
	}

	iter := aggMap.Iterate()
	var cgroupID uint64
	var perCPUStats []ebpfCgroupAggStats
	var cgroupsToDelete []uint64

	for iter.Next(&cgroupID, &perCPUStats) {
		stat := model.NoisyNeighborStats{CgroupID: cgroupID}
		for _, cpuStat := range perCPUStats {
			stat.SumLatenciesNs += cpuStat.Sum_latencies_ns
			stat.EventCount += cpuStat.Event_count
			stat.PreemptionCount += cpuStat.Preemption_count
			stat.SumCycles += cpuStat.Sum_cycles
			stat.SumInstructions += cpuStat.Sum_instructions
			stat.SumCacheMisses += cpuStat.Sum_cache_misses
			stat.SumITLBMisses += cpuStat.Sum_itlb_misses
			stat.SumSoftirqNs += cpuStat.Sum_softirq_ns
			stat.BlockIORequests += cpuStat.Block_io_requests
			stat.SumBranchMisses += cpuStat.Sum_branch_misses
			stat.SumCPUMigrations += cpuStat.Sum_cpu_migrations
			stat.WakeupCount += cpuStat.Wakeup_count
			stat.SumCacheReferences += cpuStat.Sum_cache_references
			// pid_count is a global cgroup value (not per-CPU), so take the max rather than summing
			if cpuStat.Pid_count > stat.UniquePidCount {
				stat.UniquePidCount = cpuStat.Pid_count
			}
		}

		cgroupsToDelete = append(cgroupsToDelete, cgroupID)

		// Drop only when there is no observable activity from any source.
		// PMU sums are included so a task that was already on-CPU at window
		// start and gets switched out exactly once (PMU stamp delta accrued,
		// no schedule-on event) isn't silently flushed.
		if stat.EventCount == 0 && stat.SumSoftirqNs == 0 && stat.BlockIORequests == 0 && stat.WakeupCount == 0 &&
			stat.SumCycles == 0 && stat.SumInstructions == 0 && stat.SumCacheMisses == 0 && stat.SumCacheReferences == 0 &&
			stat.SumITLBMisses == 0 && stat.SumBranchMisses == 0 && stat.SumCPUMigrations == 0 {
			continue
		}

		nnstats = append(nnstats, stat)
	}

	if err := iter.Err(); err != nil {
		log.Errorf("error iterating cgroup_agg_stats map: %v", err)
	}

	for _, cgID := range cgroupsToDelete {
		if err := aggMap.Delete(&cgID); err != nil {
			log.Errorf("failed to delete cgroup %d from agg map: %v", cgID, err)
		}
	}

	return nnstats
}
