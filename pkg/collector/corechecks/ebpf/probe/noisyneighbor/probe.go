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
	"os"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// 5.13 for kfuncs
// 6.2 for bpf_rcu_read_lock kfunc
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// Probe is the eBPF side of the noisy neighbor check
type Probe struct {
	mgr      *ddebpf.Manager
	runqPool *ddsync.TypedPool[runqEvent]

	// cgroupID -> latest event
	mtx     sync.Mutex
	cgIDMap map[uint64]runqEvent
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

	p := &Probe{
		cgIDMap:  make(map[uint64]runqEvent),
		runqPool: ddsync.NewDefaultTypedPool[runqEvent](),
		mtx:      sync.Mutex{},
	}
	// TODO noisy: figure out what you want these sizes to be. ringbuf size must be power of 2
	ringbufSize := 8 * os.Getpagesize()
	chanSize := 100
	handler := func(data []byte) {
		if len(data) == 0 {
			return
		}
		e := p.runqPool.Get()
		if err := e.UnmarshalBinary(data); err != nil {
			p.runqPool.Put(e)
			log.Debugf("failed to unmarshal runq event: %v", err)
			return
		}
		p.handleEvent(e)
	}
	eventHandler, err := perf.NewEventHandler("runq_events", handler,
		perf.UseRingBuffers(ringbufSize, chanSize),
		perf.SendTelemetry(cfg.InternalTelemetryEnabled),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new event handler: %v", err)
	}

	filename := "noisy-neighbor.o"
	if cfg.BPFDebug {
		filename = "noisy-neighbor-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		p.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{}, "noisy_neighbor", &ebpftelemetry.ErrorsTelemetryModifier{}, eventHandler)
		const uid = "noisy"
		p.mgr.Probes = []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_wakeup", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_wakeup_new", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_switch", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "runq_enqueued"},
			{Name: "cgroup_id_to_last_event_ts"},
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
	p.mtx.Lock()
	defer p.mtx.Unlock()

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

		uniquePidCount := p.countUniquePidsForCgroup(cgroupID)

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
		// Cgroup name now comes from aggregation map (reliable, not rate-limited)
		stat := model.NoisyNeighborStats{
			CgroupID:        cgroupID,
			CgroupName:      cgroupName,
			SumLatenciesNs:  cgroupLatencies,
			EventCount:      cgroupEvents,
			PreemptionCount: cgroupPreemptions,
			UniquePidCount:  uniquePidCount,
		}

		// Populate legacy fields from ringbuffer events (if available)
		// These are rate-limited individual samples kept for backwards compatibility
		// Only used for distribution metrics and context switch tracking
		if event, ok := p.cgIDMap[cgroupID]; ok {
			stat.PrevCgroupName = event.PrevCgroupName
			stat.PrevCgroupID = event.PrevCgroupID
			stat.RunqLatencyNs = event.RunqLatency
			stat.TimestampNs = event.Timestamp
			stat.Pid = event.Pid
			stat.PrevPid = event.PrevPid
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

	// Clear the event map (used for cgroup names)
	clear(p.cgIDMap)

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

func (p *Probe) countUniquePidsForCgroup(cgroupID uint64) uint64 {
	pidMap, found, err := p.mgr.GetMap("cgroup_pids")
	if err != nil || !found {
		return 0
	}

	var pidCount uint64
	iter := pidMap.Iterate()
	var key ebpfPidKey
	var val uint8

	for iter.Next(&key, &val) {
		if key.Id == cgroupID {
			pidCount++
		}
	}

	return pidCount
}

func (p *Probe) deletePidsForCgroup(cgroupID uint64) {
	pidMap, found, err := p.mgr.GetMap("cgroup_pids")
	if err != nil || !found {
		return
	}

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

func (p *Probe) handleEvent(e *runqEvent) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	defer p.runqPool.Put(e)
	// TODO noisy: handle ebpf data here, this is just an example
	p.cgIDMap[e.CgroupID] = *e
}
