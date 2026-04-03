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

// Probe is the eBPF side of the noisy neighbor check
type Probe struct {
	mgr *ddebpf.Manager
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
			{Name: "cgroup_agg_stats"},
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
	return p, nil
}

// Close releases all associated resources
func (p *Probe) Close() {
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
		for _, cpuStat := range perCPUStats {
			cgroupLatencies += cpuStat.Sum_latencies_ns
			cgroupEvents += cpuStat.Event_count
			cgroupPreemptions += cpuStat.Preemption_count
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
