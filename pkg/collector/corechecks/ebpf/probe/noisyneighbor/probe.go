// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
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

// minimumKernelVersion is 6.2.0, the first kernel with bpf_rcu_read_lock,
// which the probe uses to walk task_struct pointers safely.
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// ErrKernelTooOld is returned by NewProbe when the host kernel is below
// minimumKernelVersion. Callers can use errors.Is to skip the module silently
// on unsupported hosts.
var ErrKernelTooOld = errors.New("kernel version below minimum")

// Probe is the eBPF side of the noisy neighbor check. Construct only via
// NewProbe — the zero value is not usable.
type Probe struct {
	// mgr and closeFn are set once during NewProbe (before *Probe is
	// published to other goroutines) and never reassigned afterward; safe
	// to read without holding mu.
	mgr     *ddebpf.Manager
	closeFn func()

	// mu serializes GetAndFlush against doClose and guards pmuFds against
	// concurrent shutdown. pmuFds is written by attachPMU (pre-publication,
	// no lock required) and read/cleared in doClose under mu.
	mu     sync.Mutex
	pmuFds []int
}

// NewProbe loads the noisy_neighbor eBPF asset, attaches its tracepoints,
// and opens per-CPU PMU counters. Returns an error wrapping ErrKernelTooOld
// on unsupported hosts (callers can use errors.Is). PMU events that fail to
// open are logged and skipped — the returned Probe is still usable, but the
// affected counters report zero. On any error after manager initialization,
// resources are released before the error is returned. The caller must
// invoke Close to release the probe on the success path.
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	if err := checkKernelVersion(); err != nil {
		return nil, err
	}

	p := &Probe{}
	// Wire close before any resource-acquiring step so cleanup paths can
	// rely on p.closeFn() to release everything.
	p.closeFn = sync.OnceFunc(p.doClose)

	if err := p.loadManager(cfg); err != nil {
		return nil, err
	}
	if err := p.attachProbes(); err != nil {
		p.closeFn()
		return nil, err
	}
	ddebpf.AddNameMappings(p.mgr.Manager, "noisy_neighbor")
	p.attachPMU()
	return p, nil
}

func checkKernelVersion() error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return fmt.Errorf("kernel version: %w", err)
	}
	if kv < minimumKernelVersion {
		return fmt.Errorf("%w: minimum %s, got %s", ErrKernelTooOld, minimumKernelVersion, kv)
	}
	return nil
}

// loadManager selects the noisy-neighbor BPF asset (debug or release) and
// initializes the ebpf-manager with the tracepoints and maps the kernel
// program expects.
func (p *Probe) loadManager(cfg *ddebpf.Config) error {
	filename := "noisy-neighbor.o"
	if cfg.BPFDebug {
		filename = "noisy-neighbor-debug.o"
	}
	err := ddebpf.LoadCOREAsset(filename, func(r bytecode.AssetReader, opts manager.Options) error {
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
			{Name: "llc_misses_pmu"},
			{Name: "itlb_misses_pmu"},
			{Name: "branch_misses_pmu"},
			{Name: "cpu_migrations_pmu"},
			{Name: "cache_references_pmu"},
			{Name: "softirq_start_ns"},
		}
		if err := p.mgr.InitWithOptions(r, &opts); err != nil {
			return fmt.Errorf("init ebpf manager: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("load CO-RE asset %s: %w", filename, err)
	}
	return nil
}

// attachProbes attaches each manager probe sequentially. On failure the
// caller (NewProbe) is expected to invoke p.closeFn() so the partially-
// loaded manager and any prior attachments are torn down by doClose.
//
// We skip mgr.Start() and attach probes manually: Start() runs
// cleanupTraceFS which tries to remove ALL orphaned kprobe events on the
// host and fails fatally if any are pinned (common on dev machines where
// a previous agent left tail-called kprobes behind). This module only
// uses tracepoints, so we don't need the perf/ring reader startup that
// Start() also handles. Stop() requires state >= initialized, which
// InitWithOptions already set.
func (p *Probe) attachProbes() error {
	for _, probe := range p.mgr.Probes {
		if err := probe.Attach(); err != nil {
			return fmt.Errorf("attach probe %s: %w", probe.EBPFFuncName, err)
		}
	}
	return nil
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

// attachPMU opens per-CPU perf events for every counter we track and
// populates the corresponding BPF perf-event-array maps. Called only from
// NewProbe before the Probe is published to other goroutines, so writes to
// p.pmuFds need no synchronization. See attachPMUEvent for per-event
// semantics and failure modes.
func (p *Probe) attachPMU() {
	events := []pmuEvent{
		{mapName: "cycles_pmu", humanLabel: "cycles", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_CPU_CYCLES},
		{mapName: "instructions_pmu", humanLabel: "instructions", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_INSTRUCTIONS},
		{mapName: "llc_misses_pmu", humanLabel: "LLC misses", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_CACHE_MISSES},
		{mapName: "itlb_misses_pmu", humanLabel: "iTLB misses", perfType: unix.PERF_TYPE_HW_CACHE, perfConfig: itlbLoadMissesConfig},
		{mapName: "branch_misses_pmu", humanLabel: "branch misses", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_BRANCH_MISSES},
		// CPU migrations is a software counter — doesn't compete for hardware
		// PMU counters and is always available regardless of µarch.
		{mapName: "cpu_migrations_pmu", humanLabel: "CPU migrations", perfType: unix.PERF_TYPE_SOFTWARE, perfConfig: unix.PERF_COUNT_SW_CPU_MIGRATIONS},
		// Cache references pairs with llc_misses to give LLC hit-rate: under
		// cache thrashing the rate moves even when absolute miss count stays
		// flat for memory-bound workloads already at floor miss rate.
		{mapName: "cache_references_pmu", humanLabel: "cache references", perfType: unix.PERF_TYPE_HARDWARE, perfConfig: unix.PERF_COUNT_HW_CACHE_REFERENCES},
	}
	numCPU := runtime.NumCPU()
	p.pmuFds = make([]int, 0, len(events)*numCPU)
	for _, ev := range events {
		p.attachPMUEvent(ev, numCPU)
	}
}

// attachPMUEvent opens one perf event per CPU and inserts the fds into the
// associated BPF perf-event-array map. On hosts where the event isn't
// supported (virtualized envs, restrictive perf_event_paranoid, missing
// CAP_PERF_MON, or µarch lacking the event) opens fail and we log once per
// event type. Other events still attach independently, and the eBPF program
// checks each read-value return code and skips just that counter's deltas.
func (p *Probe) attachPMUEvent(ev pmuEvent, numCPU int) {
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
	for cpu := range numCPU {
		fd, err := openPerfEvent(ev.perfType, ev.perfConfig, cpu)
		if err != nil {
			if !logged {
				log.Warnf("noisy_neighbor: %s PMU event unavailable, metric will be zero: %v", ev.humanLabel, err)
				logged = true
			}
			continue
		}
		if err := m.Put(uint32(cpu), uint32(fd)); err != nil {
			log.Warnf("noisy_neighbor: failed to register %s fd on cpu %d: %v", ev.humanLabel, cpu, err)
			if cerr := unix.Close(fd); cerr != nil {
				log.Warnf("noisy_neighbor: failed to close orphaned %s fd on cpu %d: %v", ev.humanLabel, cpu, cerr)
			}
			continue
		}
		p.pmuFds = append(p.pmuFds, fd)
	}
}

// openPerfEvent opens a single perf event of any type (hardware, software,
// or hw_cache) on the given CPU and returns the fd. unsafe.Sizeof is
// required by the kernel ABI for perf_event_attr.size.
func openPerfEvent(typ uint32, config uint64, cpu int) (int, error) {
	attr := &unix.PerfEventAttr{
		Type:   typ,
		Config: config,
		Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
	}
	fd, err := unix.PerfEventOpen(attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return -1, fmt.Errorf("perf_event_open(type=%d, config=%#x, cpu=%d): %w", typ, config, cpu, err)
	}
	if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_ENABLE, 0); err != nil {
		if cerr := unix.Close(fd); cerr != nil {
			return -1, fmt.Errorf("enable perf event: %w (also close failed: %v)", err, cerr)
		}
		return -1, fmt.Errorf("enable perf event: %w", err)
	}
	return fd, nil
}

// Close releases all associated resources. Safe to call multiple times.
func (p *Probe) Close() {
	p.closeFn()
}

// doClose is the unguarded shutdown path. It runs exactly once, gated by
// p.closeFn = sync.OnceFunc(p.doClose).
func (p *Probe) doClose() {
	p.mu.Lock()
	defer p.mu.Unlock()
	var errs []error
	for _, fd := range p.pmuFds {
		if err := unix.Close(fd); err != nil {
			errs = append(errs, fmt.Errorf("close PMU fd %d: %w", fd, err))
		}
	}
	p.pmuFds = nil
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			// Manager stop failure is operationally critical: orphan kprobes
			// can pin the next startup.
			errs = append(errs, fmt.Errorf("stop ebpf manager: %w", err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		log.Errorf("noisy_neighbor: shutdown errors: %v", err)
	}
}

// GetAndFlush returns aggregated per-cgroup stats for the current window
// and atomically deletes each returned cgroup's entry from the BPF map.
// Cgroups with zero observable activity (no events, softirq, block-IO,
// or wakeups) are filtered out of the return slice but still flushed.
//
// A non-nil error means the returned slice is nil and callers must not
// emit metrics — the next cycle will double-count any cgroups whose Delete
// failed. ctx cancellation aborts iteration between map operations.
//
// GetAndFlush holds p.mu for the entire iteration; Close blocks for the
// same duration. Callers serialize at a higher layer (the system-probe
// HTTP handler uses WithConcurrencyLimit(1)).
func (p *Probe) GetAndFlush(ctx context.Context) ([]model.Stats, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	aggMap, found, err := p.mgr.GetMap("cgroup_agg_stats")
	if err != nil {
		return nil, fmt.Errorf("get cgroup_agg_stats map: %w", err)
	}
	if !found {
		return nil, errors.New("cgroup_agg_stats map not found")
	}

	// No capacity hint: the BPF map is sized for worst case (4096) but
	// typical hosts carry tens to low hundreds of active cgroups, so
	// pre-allocating would waste hundreds of KB per call.
	var stats []model.Stats
	var cgroupsToDelete []uint64

	iter := aggMap.Iterate()
	var cgroupID uint64
	perCPUStats := make([]ebpfCgroupAggStats, runtime.NumCPU())

	for iter.Next(&cgroupID, &perCPUStats) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cgroupsToDelete = append(cgroupsToDelete, cgroupID)
		var agg model.Stats
		aggregatePerCPU(&agg, cgroupID, perCPUStats)
		if !hasActivity(&agg) {
			continue
		}
		stats = append(stats, agg)
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("iterate cgroup_agg_stats: %w", err)
	}

	var deleteErrs []error
	for _, cgroupID := range cgroupsToDelete {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := aggMap.Delete(&cgroupID); err != nil {
			if errors.Is(err, unix.ENOENT) {
				// The entry got deleted between Iterate and Delete (concurrent
				// removal by the eBPF side). Benign.
				continue
			}
			deleteErrs = append(deleteErrs, fmt.Errorf("delete cgroup %d: %w", cgroupID, err))
		}
	}
	if err := errors.Join(deleteErrs...); err != nil {
		return nil, fmt.Errorf("flush cgroup_agg_stats: %w", err)
	}

	return stats, nil
}

// aggregatePerCPU sums per-CPU counters for one cgroup into dst. Range by
// index to avoid copying the 112-byte ebpfCgroupAggStats per iteration.
func aggregatePerCPU(dst *model.Stats, cgroupID uint64, perCPU []ebpfCgroupAggStats) {
	dst.CgroupID = cgroupID
	for i := range perCPU {
		cpuStat := &perCPU[i]
		dst.SumLatenciesNs += cpuStat.Sum_latencies_ns
		dst.EventCount += cpuStat.Event_count
		dst.PreemptionCount += cpuStat.Preemption_count
		dst.SumCycles += cpuStat.Sum_cycles
		dst.SumInstructions += cpuStat.Sum_instructions
		dst.SumLLCMisses += cpuStat.Sum_llc_misses
		dst.SumITLBMisses += cpuStat.Sum_itlb_misses
		dst.SumSoftirqNs += cpuStat.Sum_softirq_ns
		dst.BlockIORequests += cpuStat.Block_io_requests
		dst.SumBranchMisses += cpuStat.Sum_branch_misses
		dst.SumCPUMigrations += cpuStat.Sum_cpu_migrations
		dst.WakeupCount += cpuStat.Wakeup_count
		dst.SumCacheReferences += cpuStat.Sum_cache_references
		// pid_count is a global cgroup value (not per-CPU): take max, not sum.
		dst.UniquePidCount = max(dst.UniquePidCount, cpuStat.Pid_count)
	}
}

// hasActivity reports whether the cgroup had any observable scheduling,
// softirq, block-IO, or wakeup activity in the window.
func hasActivity(s *model.Stats) bool {
	return s.EventCount != 0 || s.SumSoftirqNs != 0 || s.BlockIORequests != 0 || s.WakeupCount != 0
}
