// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"fmt"
	"runtime"
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

// Probe is the eBPF side of the noisy neighbor check
type Probe struct {
	mgr    *ddebpf.Manager
	pmuFDs []int
}

// NewProbe creates a [Probe]
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %s", err)
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
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "runq_enqueued"},
			{Name: "task_oncpu_pmu"},
			{Name: "cgroup_agg_stats"},
			{Name: "cycles_pmu"},
			{Name: "instructions_pmu"},
		}
		if err := p.mgr.InitWithOptions(buf, &opts); err != nil {
			return fmt.Errorf("failed to init ebpf manager: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = p.mgr.Start()
	if err != nil {
		return nil, err
	}
	ddebpf.AddNameMappings(p.mgr.Manager, "noisy_neighbor")
	p.attachPMU()
	return p, nil
}

// attachPMU opens per-CPU hardware perf events for cycles and instructions,
// inserting their file descriptors into the corresponding BPF perf-event-array
// maps. On hosts where the PMU is unavailable (some virtualized environments,
// restrictive perf_event_paranoid, missing CAP_PERF_MON) the perf event opens
// will fail; we log once and proceed. The eBPF program checks the read-value
// helper return code and skips CPI accumulation for those CPUs.
func (p *Probe) attachPMU() {
	cyclesMap, _, err := p.mgr.GetMap("cycles_pmu")
	if err != nil || cyclesMap == nil {
		log.Warnf("noisy_neighbor: cycles_pmu map missing, CPI metrics disabled: %v", err)
		return
	}
	instMap, _, err := p.mgr.GetMap("instructions_pmu")
	if err != nil || instMap == nil {
		log.Warnf("noisy_neighbor: instructions_pmu map missing, CPI metrics disabled: %v", err)
		return
	}

	numCPU := runtime.NumCPU()
	var loggedPMU bool
	for cpu := 0; cpu < numCPU; cpu++ {
		cycFD, err := openHardwarePerfEvent(cpu, unix.PERF_COUNT_HW_CPU_CYCLES)
		if err != nil {
			if !loggedPMU {
				log.Warnf("noisy_neighbor: PMU unavailable, cycles/instructions metrics will be zero: %v", err)
				loggedPMU = true
			}
			continue
		}
		insFD, err := openHardwarePerfEvent(cpu, unix.PERF_COUNT_HW_INSTRUCTIONS)
		if err != nil {
			unix.Close(cycFD)
			if !loggedPMU {
				log.Warnf("noisy_neighbor: PMU unavailable, cycles/instructions metrics will be zero: %v", err)
				loggedPMU = true
			}
			continue
		}
		key := uint32(cpu)
		if err := cyclesMap.Put(key, uint32(cycFD)); err != nil {
			log.Warnf("noisy_neighbor: failed to register cycles fd on cpu %d: %v", cpu, err)
			unix.Close(cycFD)
			unix.Close(insFD)
			continue
		}
		if err := instMap.Put(key, uint32(insFD)); err != nil {
			log.Warnf("noisy_neighbor: failed to register instructions fd on cpu %d: %v", cpu, err)
			unix.Close(cycFD)
			unix.Close(insFD)
			continue
		}
		p.pmuFDs = append(p.pmuFDs, cycFD, insFD)
	}
}

func openHardwarePerfEvent(cpu int, config uint64) (int, error) {
	attr := &unix.PerfEventAttr{
		Type:   unix.PERF_TYPE_HARDWARE,
		Config: config,
		Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
	}
	fd, err := unix.PerfEventOpen(attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return -1, err
	}
	if err := unix.IoctlSetInt(fd, unix.PERF_EVENT_IOC_ENABLE, 0); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("enable perf event: %w", err)
	}
	return fd, nil
}

// Close releases all associated resources
func (p *Probe) Close() {
	for _, fd := range p.pmuFDs {
		unix.Close(fd)
	}
	p.pmuFDs = nil
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping ebpf manager: %s", err)
		}
	}
}

// GetAndFlush gets the stats
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
		var cgroupLatencies, cgroupEvents, cgroupPreemptions, pidCount uint64
		var cgroupCycles, cgroupInstructions uint64
		for _, cpuStat := range perCPUStats {
			cgroupLatencies += cpuStat.Sum_latencies_ns
			cgroupEvents += cpuStat.Event_count
			cgroupPreemptions += cpuStat.Preemption_count
			cgroupCycles += cpuStat.Sum_cycles
			cgroupInstructions += cpuStat.Sum_instructions
			// pid_count is a global cgroup value (not per-CPU), so take the max rather than summing
			if cpuStat.Pid_count > pidCount {
				pidCount = cpuStat.Pid_count
			}
		}

		cgroupsToDelete = append(cgroupsToDelete, cgroupID)

		if cgroupEvents == 0 {
			continue
		}

		stat := model.NoisyNeighborStats{
			CgroupID:        cgroupID,
			SumLatenciesNs:  cgroupLatencies,
			EventCount:      cgroupEvents,
			PreemptionCount: cgroupPreemptions,
			UniquePidCount:  pidCount,
			SumCycles:       cgroupCycles,
			SumInstructions: cgroupInstructions,
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
