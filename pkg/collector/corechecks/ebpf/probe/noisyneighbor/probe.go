// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// 5.13 for kfuncs, 6.2 for bpf_rcu_read_lock kfunc
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// defaultCgroupRoot is where the host's cgroup v2 hierarchy is visible from
// inside the system-probe container. The system-probe container itself only
// has a cgroup-namespaced view of /sys/fs/cgroup (rooted at its own cgroup);
// the host's full tree is reachable through /host/proc/1/root because the
// /host/proc mount is the host proc filesystem.
const defaultCgroupRoot = "/host/proc/1/root/sys/fs/cgroup"

// Probe is the eBPF side of the noisy neighbor check
type Probe struct {
	mgr    *ddebpf.Manager
	pmuMgr *cgroupPMUManager
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
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_softirq_entry", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_softirq_exit", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_block_rq_issue", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "runq_enqueued"},
			{Name: "cgroup_agg_stats"},
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

	err = p.mgr.Start()
	if err != nil {
		return nil, err
	}
	ddebpf.AddNameMappings(p.mgr.Manager, "noisy_neighbor")

	p.pmuMgr = newCgroupPMUManager(defaultCgroupRoot)
	if err := p.pmuMgr.Refresh(); err != nil {
		// A failure here is informational — perf_event_open may be denied on
		// some hosts (paranoid kernel.perf_event_paranoid, missing
		// CAP_PERF_MON) but BPF-side metrics still work.
		log.Warnf("noisy_neighbor: initial cgroup PMU refresh failed: %v", err)
	}
	return p, nil
}

// Close releases all associated resources
func (p *Probe) Close() {
	if p.pmuMgr != nil {
		p.pmuMgr.Close()
	}
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping ebpf manager: %s", err)
		}
	}
}

// GetAndFlush refreshes the user-space PMU fd set against the current cgroup
// tree, reads counters, drains the BPF cgroup_agg_stats map, and returns the
// merged per-cgroup statistics. A row is returned whenever either source has
// non-empty data for that cgroup; missing fields are zero. The BPF map is
// reset after every read; PMU counters are u64-monotonic and tracked
// per-cgroup as deltas internally.
func (p *Probe) GetAndFlush() []model.NoisyNeighborStats {
	if p.pmuMgr != nil {
		if err := p.pmuMgr.Refresh(); err != nil {
			log.Debugf("noisy_neighbor: cgroup PMU refresh: %v", err)
		}
	}

	merged := make(map[uint64]*model.NoisyNeighborStats)
	if p.pmuMgr != nil {
		for cgID, ps := range p.pmuMgr.ReadAll() {
			merged[cgID] = &model.NoisyNeighborStats{
				CgroupID: cgID,
				PMU:      ps,
			}
		}
	}

	aggMap, found, err := p.mgr.GetMap("cgroup_agg_stats")
	if err != nil {
		log.Errorf("failed to get cgroup_agg_stats map: %v", err)
	} else if !found {
		log.Warn("cgroup_agg_stats map not found")
	} else {
		iter := aggMap.Iterate()
		var cgroupID uint64
		var perCPUStats []ebpfCgroupAggStats
		var cgroupsToDelete []uint64

		for iter.Next(&cgroupID, &perCPUStats) {
			var sumLatencies, eventCount, preemptionCount, pidCount uint64
			var wakeupCount, sumSoftirqNs, blockIORequests uint64
			for _, cpuStat := range perCPUStats {
				sumLatencies += cpuStat.Sum_latencies_ns
				eventCount += cpuStat.Event_count
				preemptionCount += cpuStat.Preemption_count
				wakeupCount += cpuStat.Wakeup_count
				sumSoftirqNs += cpuStat.Sum_softirq_ns
				blockIORequests += cpuStat.Block_io_requests
				// pid_count is a global cgroup value (not per-CPU), so take the max rather than summing
				if cpuStat.Pid_count > pidCount {
					pidCount = cpuStat.Pid_count
				}
			}

			cgroupsToDelete = append(cgroupsToDelete, cgroupID)

			// Skip cgroups with no scheduling activity AND no other counters
			// to keep the metric stream from emitting rows for cgroups the
			// kernel never scheduled in the interval. PMU-only rows are kept
			// because the PMU manager populated them already.
			if eventCount == 0 && wakeupCount == 0 && sumSoftirqNs == 0 && blockIORequests == 0 {
				continue
			}

			entry, ok := merged[cgroupID]
			if !ok {
				entry = &model.NoisyNeighborStats{CgroupID: cgroupID}
				merged[cgroupID] = entry
			}
			entry.SumLatenciesNs = sumLatencies
			entry.EventCount = eventCount
			entry.PreemptionCount = preemptionCount
			entry.UniquePidCount = pidCount
			entry.WakeupCount = wakeupCount
			entry.SumSoftirqNs = sumSoftirqNs
			entry.BlockIORequests = blockIORequests
		}

		if err := iter.Err(); err != nil {
			log.Errorf("error iterating cgroup_agg_stats map: %v", err)
		}

		for _, cgID := range cgroupsToDelete {
			if err := aggMap.Delete(&cgID); err != nil {
				log.Errorf("failed to delete cgroup %d from agg map: %v", cgID, err)
			}
		}
	}

	nnstats := make([]model.NoisyNeighborStats, 0, len(merged))
	for _, s := range merged {
		nnstats = append(nnstats, *s)
	}
	return nnstats
}
