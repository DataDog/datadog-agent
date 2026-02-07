// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/noisy-neighbor-kern.c pkg/ebpf/bytecode/build/runtime/noisy-neighbor.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/noisy-neighbor.c pkg/ebpf/bytecode/runtime/noisy-neighbor.go runtime

package noisyneighbor

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// 5.13 for kfuncs
// 6.2 for bpf_rcu_read_lock kfunc
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// PERFORMANCE OPTIMIZATION:
// Instead of scanning the cgroup_pids map for each cgroup (O(N×M) complexity),
// we scan it once per GetAndFlush and build counts for all cgroups (O(M) complexity).
//
// Previous approach: For each cgroup, scan ALL entries in the cgroup_pids BPF map
//   - 100 cgroups × 10,000 total PIDs = 1,000,000 iterations per GetAndFlush()
//   - With 1000 cgroups × 100,000 PIDs = 100,000,000 iterations (catastrophic!)
//
// Optimized approach: Scan cgroup_pids map once, build counts for all cgroups
//   - 10,000 PIDs = 10,000 iterations total (not per cgroup!)
//   - Simple, no ringbuffer overhead, no event drops
//   - O(M) where M = total PIDs, amortized to O(1) per cgroup

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
	start, _ := ddebpf.NowNanoseconds()

	var nnstats []model.NoisyNeighborStats
	var totalEvents, totalPreemptions, totalLatencies uint64
	cgroupCount := 0

	// Get the aggregation map for PSL/PSP calculation
	aggMap, found, err := p.mgr.GetMap("cgroup_agg_stats")
	if err != nil {
		log.Errorf("failed to get cgroup_agg_stats map: %v", err)
		return nnstats
	}
	if !found {
		log.Warn("cgroup_agg_stats map not found")
		return nnstats
	}

	// Build PID counts by scanning cgroup_pids map ONCE for all cgroups
	// This is O(M) where M = total PIDs, much better than O(N×M) per-cgroup scanning
	pidCounts := p.buildPidCounts()

	// Iterate through all cgroups in the aggregation map
	iter := aggMap.Iterate()
	var cgroupID uint64
	var perCPUStats []ebpfCgroupAggStats
	var cgroupsToDelete []uint64
	var starvedCgroups int

	for iter.Next(&cgroupID, &perCPUStats) {
		cgroupCount++

		// Aggregate across all CPUs and extract cgroup name from first non-empty entry
		var cgroupLatencies, cgroupEvents, cgroupPreemptions uint64
		var cgroupName string
		for _, cpuStat := range perCPUStats {
			cgroupLatencies += cpuStat.Sum_latencies_ns
			cgroupEvents += cpuStat.Event_count
			cgroupPreemptions += cpuStat.Preemption_count
			// Get cgroup name from first CPU that has it (they should all have the same name)
			if cgroupName == "" && len(cpuStat.Cgroup_name) > 0 {
				cgroupName = unix.ByteSliceToString(cpuStat.Cgroup_name[:])
			}
		}

		// Accumulate totals for logging
		totalLatencies += cgroupLatencies
		totalEvents += cgroupEvents
		totalPreemptions += cgroupPreemptions

		// Collect key for deletion
		cgroupsToDelete = append(cgroupsToDelete, cgroupID)

		// Get unique PID count from our pre-built map
		uniquePidCount := pidCounts[cgroupID]

		// Track cgroups that were preempted but never scheduled (extremely throttled)
		// Include these as special entries so the check can count them
		if cgroupEvents == 0 && cgroupPreemptions > 0 {
			starvedCgroups++
			log.Debugf("[noisy_neighbor] Cgroup %d (%s): %d preemptions but 0 scheduling events (extreme throttling)",
				cgroupID, cgroupName, cgroupPreemptions)

			// Add special entry with zero events but preemptions counted
			// The check will detect this pattern and emit a separate metric
			nnstats = append(nnstats, model.NoisyNeighborStats{
				CgroupID:        cgroupID,
				CgroupName:      cgroupName,
				PreemptionCount: cgroupPreemptions,
				EventCount:      0, // Marker for starved cgroup
			})
			continue
		}

		// Skip cgroups with no events and no preemptions (inactive)
		if cgroupEvents == 0 {
			continue
		}

		// Build stats entry with aggregated data
		// Cgroup name comes from aggregation map (reliable, comprehensive)
		stat := model.NoisyNeighborStats{
			CgroupID:        cgroupID,
			CgroupName:      cgroupName,
			SumLatenciesNs:  cgroupLatencies,
			EventCount:      cgroupEvents,
			PreemptionCount: cgroupPreemptions,
			UniquePidCount:  uniquePidCount,
		}

		nnstats = append(nnstats, stat)
	}

	if err := iter.Err(); err != nil {
		log.Errorf("error iterating cgroup_agg_stats map: %v", err)
	}

	// Clear the aggregation map for next interval
	for _, cgID := range cgroupsToDelete {
		if err := aggMap.Delete(&cgID); err != nil {
			log.Warnf("failed to delete cgroup %d from agg map: %v", cgID, err)
		}
		p.deletePidsForCgroup(cgID)
	}

	// Calculate flush duration
	end, _ := ddebpf.NowNanoseconds()
	flushDurationNs := end - start
	flushDurationMs := float64(flushDurationNs) / 1e6

	// Log performance statistics
	log.Debugf("[noisy_neighbor] Flushed %d cgroups (%d with events, %d starved), %d total events (%d preemptions) in %.2fms",
		cgroupCount, len(nnstats), starvedCgroups, totalEvents, totalPreemptions, flushDurationMs)

	// Warn on performance issues
	if flushDurationMs > 50.0 {
		log.Warnf("[noisy_neighbor] Slow flush detected: %.2fms for %d cgroups", flushDurationMs, cgroupCount)
	}

	// Warn if no data collected
	if totalEvents == 0 && cgroupCount > 0 {
		log.Debugf("[noisy_neighbor] No events collected for %d tracked cgroups in this interval", cgroupCount)
	}

	return nnstats
}

// Note: In this code, "PID" follows kernel convention (task_struct->pid)
// which is actually the Thread ID (TID) in userspace terminology.
// The Linux scheduler operates at thread granularity, not process granularity.

// buildPidCounts scans the cgroup_pids BPF map once and builds a count of unique PIDs per cgroup
// OPTIMIZED: O(M) where M = total PIDs, instead of O(N×M) by scanning once for all cgroups
func (p *Probe) buildPidCounts() map[uint64]uint64 {
	pidCounts := make(map[uint64]uint64)

	pidMap, found, err := p.mgr.GetMap("cgroup_pids")
	if err != nil || !found {
		return pidCounts
	}

	// Single scan of the entire map - O(M) where M = total PIDs
	iter := pidMap.Iterate()
	var key ebpfPidKey
	var val uint8

	for iter.Next(&key, &val) {
		pidCounts[key.Id]++
	}

	return pidCounts
}

// deletePidsForCgroup cleans up PID tracking for a cgroup
// Now simply deletes all PIDs for a cgroup from the BPF map
func (p *Probe) deletePidsForCgroup(cgroupID uint64) {
	pidMap, found, err := p.mgr.GetMap("cgroup_pids")
	if err != nil || !found {
		return
	}

	// We need to iterate to find all PIDs for this cgroup
	// This is still O(M) but only happens during cleanup
	var keysToDelete []ebpfPidKey
	iter := pidMap.Iterate()
	var key ebpfPidKey
	var val uint8

	for iter.Next(&key, &val) {
		if key.Id == cgroupID {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, k := range keysToDelete {
		_ = pidMap.Delete(&k)
	}
}
