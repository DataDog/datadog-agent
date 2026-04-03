// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"fmt"

	"github.com/cilium/ebpf"
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
	mgr                *ddebpf.Manager
	watchlistActiveMap *ebpf.Map
	watchlistMap       *ebpf.Map
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
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_migrate_task", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "runq_enqueued"},
			{Name: "cgroup_agg_stats"},
			{Name: "watchlist_active"},
			{Name: "watchlist"},
			{Name: "preemptor_stats"},
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

	// Cache map references for direct access
	if m, found, err := p.mgr.GetMap("watchlist_active"); err == nil && found {
		p.watchlistActiveMap = m
	} else {
		p.Close()
		return nil, fmt.Errorf("failed to get watchlist_active map: found=%v err=%v", found, err)
	}
	if m, found, err := p.mgr.GetMap("watchlist"); err == nil && found {
		p.watchlistMap = m
	} else {
		p.Close()
		return nil, fmt.Errorf("failed to get watchlist map: found=%v err=%v", found, err)
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

// UpdateWatchlist replaces the current watchlist with the given cgroup IDs.
// It sets/clears the watchlist_active gate accordingly.
func (p *Probe) UpdateWatchlist(cgroupIDs []uint64) error {
	// Clear existing watchlist entries
	var key uint64
	var val uint8
	iter := p.watchlistMap.Iterate()
	var keysToDelete []uint64
	for iter.Next(&key, &val) {
		keysToDelete = append(keysToDelete, key)
	}
	for _, k := range keysToDelete {
		if err := p.watchlistMap.Delete(&k); err != nil {
			log.Warnf("noisy_neighbor: failed to delete watchlist entry %d: %v", k, err)
		}
	}

	// Insert new entries
	var dummy uint8 = 1
	for _, cgID := range cgroupIDs {
		if err := p.watchlistMap.Put(&cgID, &dummy); err != nil {
			log.Warnf("noisy_neighbor: failed to add watchlist entry %d: %v", cgID, err)
		}
	}

	// Set/clear the global gate
	var zeroKey uint32
	var activeVal uint8
	if len(cgroupIDs) > 0 {
		activeVal = 1
	}
	if err := p.watchlistActiveMap.Put(&zeroKey, &activeVal); err != nil {
		return fmt.Errorf("failed to update watchlist_active: %w", err)
	}

	return nil
}

// GetAndFlush gets the stats and clears the maps
func (p *Probe) GetAndFlush() model.CheckResponse {
	var resp model.CheckResponse

	// Flush cgroup_agg_stats
	aggMap, found, err := p.mgr.GetMap("cgroup_agg_stats")
	if err != nil {
		log.Errorf("failed to get cgroup_agg_stats map: %v", err)
		return resp
	}
	if !found {
		log.Warn("cgroup_agg_stats map not found")
		return resp
	}

	iter := aggMap.Iterate()
	var cgroupID uint64
	var perCPUStats []ebpfCgroupAggStats
	var cgroupsToDelete []uint64

	for iter.Next(&cgroupID, &perCPUStats) {
		var (
			latencies, events, foreignPreemptions, selfPreemptions uint64
			taskCount                                              uint64
			bucketLt100us, bucket100us1ms, bucket1ms10ms           uint64
			bucketGt10ms, migrations                               uint64
		)
		for _, cpuStat := range perCPUStats {
			latencies += cpuStat.Sum_latencies_ns
			events += cpuStat.Event_count
			foreignPreemptions += cpuStat.Foreign_preemption_count
			selfPreemptions += cpuStat.Self_preemption_count
			// task_count is a global cgroup value (not per-CPU), so take the max
			if cpuStat.Task_count > taskCount {
				taskCount = cpuStat.Task_count
			}
			bucketLt100us += cpuStat.Latency_bucket_lt_100us
			bucket100us1ms += cpuStat.Latency_bucket_100us_1ms
			bucket1ms10ms += cpuStat.Latency_bucket_1ms_10ms
			bucketGt10ms += cpuStat.Latency_bucket_gt_10ms
			migrations += cpuStat.Cpu_migrations
		}

		cgroupsToDelete = append(cgroupsToDelete, cgroupID)

		if events == 0 && foreignPreemptions == 0 && selfPreemptions == 0 && migrations == 0 {
			continue
		}

		resp.CgroupStats = append(resp.CgroupStats, model.NoisyNeighborStats{
			CgroupID:               cgroupID,
			SumLatenciesNs:         latencies,
			EventCount:             events,
			ForeignPreemptionCount: foreignPreemptions,
			SelfPreemptionCount:    selfPreemptions,
			TaskCount:              taskCount,
			LatencyBucketLt100us:   bucketLt100us,
			LatencyBucket100us1ms:  bucket100us1ms,
			LatencyBucket1ms10ms:   bucket1ms10ms,
			LatencyBucketGt10ms:    bucketGt10ms,
			CpuMigrations:          migrations,
		})
	}

	if err := iter.Err(); err != nil {
		log.Errorf("error iterating cgroup_agg_stats map: %v", err)
	}

	for _, cgID := range cgroupsToDelete {
		if err := aggMap.Delete(&cgID); err != nil {
			log.Errorf("failed to delete cgroup %d from agg map: %v", cgID, err)
		}
	}

	// Flush preemptor_stats
	preemptorMap, found, err := p.mgr.GetMap("preemptor_stats")
	if err != nil {
		log.Errorf("failed to get preemptor_stats map: %v", err)
		return resp
	}
	if !found {
		return resp
	}

	var pkey ebpfPreemptorKey
	var perCPUPreemptorStats []ebpfPreemptorStats
	var pkeysToDelete []ebpfPreemptorKey

	piter := preemptorMap.Iterate()
	for piter.Next(&pkey, &perCPUPreemptorStats) {
		var totalCount uint64
		for _, cpuStat := range perCPUPreemptorStats {
			totalCount += cpuStat.Count
		}
		pkeysToDelete = append(pkeysToDelete, pkey)

		if totalCount == 0 {
			continue
		}

		resp.PreemptorStats = append(resp.PreemptorStats, model.PreemptorStats{
			VictimCgroupID:    pkey.Victim_cgroup_id,
			PreemptorCgroupID: pkey.Preemptor_cgroup_id,
			Count:             totalCount,
		})
	}

	if err := piter.Err(); err != nil {
		log.Errorf("error iterating preemptor_stats map: %v", err)
	}

	for _, pk := range pkeysToDelete {
		if err := preemptorMap.Delete(&pk); err != nil {
			log.Errorf("failed to delete preemptor entry: %v", err)
		}
	}

	return resp
}
