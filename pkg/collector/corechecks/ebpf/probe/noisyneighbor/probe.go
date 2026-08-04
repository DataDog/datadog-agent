// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// 5.13 for kfuncs, 6.2 for bpf_rcu_read_lock kfunc.
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// Config controls the optional PMU portion of the probe.
type Config struct {
	EventMask         uint64
	MaxTrackedCgroups int
}

// Stats is exported through the system-probe module stats endpoint.
type Stats struct {
	ConfiguredEventMask uint64 `json:"configured_event_mask"`
	EffectiveEventMask  uint64 `json:"effective_event_mask"`
	PerfFDCount         int    `json:"perf_fd_count"`
	OnlineCPUs          int    `json:"online_cpus"`
	WatchlistSize       int    `json:"watchlist_size"`
	EligibleCgroups     int    `json:"eligible_cgroups"`
	AttachErrors        uint64 `json:"attach_errors"`
	ReadErrors          uint64 `json:"read_errors"`
	ScalingErrors       uint64 `json:"scaling_errors"`
	LastRotation        int64  `json:"last_rotation"`
}

// Probe is the eBPF side of the noisy neighbor check.
type Probe struct {
	mgr        *ddebpf.Manager
	config     Config
	loadMask   uint64
	perf       *perfEventSet
	cpus       []uint
	generation uint64
	watchlist  map[uint64]struct{}

	mu     sync.Mutex
	stats  Stats
	closed bool
}

// NewProbe creates a Probe. Omitting probeConfig keeps the existing scheduling-only behavior.
func NewProbe(cfg *ddebpf.Config, probeConfig ...Config) (_ *Probe, retErr error) {
	pc := Config{MaxTrackedCgroups: 64}
	if len(probeConfig) > 0 {
		pc = probeConfig[0]
	}
	if pc.MaxTrackedCgroups < 1 || pc.MaxTrackedCgroups > maxTrackedCgroups {
		return nil, fmt.Errorf("max tracked cgroups must be between 1 and %d", maxTrackedCgroups)
	}

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %s", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	cpus, err := readOnlineCPUs()
	if err != nil {
		return nil, fmt.Errorf("read online CPUs: %w", err)
	}
	perfEvents, effectiveHardwareMask, openErrors := openPerfEvents(pc.EventMask, cpus, systemPerfEventOpener{})
	p := &Probe{
		config:    pc,
		loadMask:  effectiveHardwareMask | pc.EventMask&model.EventCPUMigrations,
		perf:      perfEvents,
		cpus:      cpus,
		watchlist: make(map[uint64]struct{}),
		stats: Stats{
			ConfiguredEventMask: pc.EventMask,
			EffectiveEventMask:  effectiveHardwareMask | pc.EventMask&model.EventCPUMigrations,
			PerfFDCount:         perfEvents.fdCount(),
			OnlineCPUs:          len(cpus),
			AttachErrors:        openErrors,
		},
	}
	defer func() {
		if retErr != nil {
			p.perf.close()
		}
	}()

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
		p.mgr.Maps = []*manager.Map{{Name: "runq_enqueued"}, {Name: "cgroup_agg_stats"}}
		if p.loadMask != 0 {
			if p.loadMask&model.EventCPUMigrations != 0 {
				p.mgr.Probes = append(p.mgr.Probes, &manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_migrate_task", UID: uid}})
			}
			for _, name := range []string{"pmu_task_state", "pmu_cgroup_stats", "pmu_watchlist", "pmu_config", "pmu_error_stats", "pmu_cycles", "pmu_instructions", "pmu_cache_misses", "pmu_cache_references", "pmu_itlb_misses", "pmu_branch_misses"} {
				p.mgr.Maps = append(p.mgr.Maps, &manager.Map{Name: name})
			}
			opts.ConstantEditors = append(opts.ConstantEditors, manager.ConstantEditor{Name: "pmu_event_mask", Value: p.loadMask, BTFGlobalConstant: true, FailOnMissing: true})
			possibleCPUs, err := kernel.PossibleCPUs()
			if err != nil {
				return fmt.Errorf("read possible CPUs: %w", err)
			}
			if opts.MapSpecEditors == nil {
				opts.MapSpecEditors = make(map[string]manager.MapSpecEditor)
			}
			for _, event := range hardwareEvents {
				opts.MapSpecEditors[event.mapName] = manager.MapSpecEditor{MaxEntries: uint32(possibleCPUs), EditorFlag: manager.EditMaxEntries}
			}
		}
		if err := p.mgr.InitWithOptions(buf, &opts); err != nil {
			return fmt.Errorf("failed to init ebpf manager: %w", err)
		}
		if p.loadMask != 0 {
			if err := p.populatePerfEventMaps(p.perf); err != nil {
				return err
			}
			if err := p.writePMUConfig(false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err = p.mgr.Start(); err != nil {
		return nil, err
	}
	ddebpf.AddNameMappings(p.mgr.Manager, "noisy_neighbor")
	return p, nil
}

func (p *Probe) populatePerfEventMaps(events *perfEventSet) error {
	for _, event := range hardwareEvents {
		fds, enabled := events.byMask[event.mask]
		if !enabled {
			continue
		}
		perfMap, found, err := p.mgr.GetMap(event.mapName)
		if err != nil {
			return fmt.Errorf("get %s map: %w", event.mapName, err)
		}
		if !found {
			return fmt.Errorf("get %s map: map not found", event.mapName)
		}
		for cpu, fd := range fds {
			key, value := uint32(cpu), uint32(fd)
			if err := perfMap.Update(&key, &value, ebpf.UpdateAny); err != nil {
				return fmt.Errorf("populate %s for CPU %d: %w", event.mapName, cpu, err)
			}
		}
	}
	return nil
}

func (p *Probe) writePMUConfig(active bool) error {
	configMap, found, err := p.mgr.GetMap("pmu_config")
	if err != nil {
		return fmt.Errorf("get pmu_config map: %w", err)
	}
	if !found {
		return errors.New("get pmu_config map: map not found")
	}
	activeValue := uint32(0)
	if active {
		activeValue = 1
	}
	key := uint32(0)
	value := ebpfPmuConfig{Active: activeValue, Generation: p.generation, Effective_event_mask: p.stats.EffectiveEventMask}
	return configMap.Update(&key, &value, ebpf.UpdateAny)
}

func (p *Probe) advanceWatchlistGeneration() error {
	if len(p.watchlist) == 0 {
		p.generation++
		return nil
	}
	watchlistMap, found, err := p.mgr.GetMap("pmu_watchlist")
	if err != nil {
		return fmt.Errorf("get pmu_watchlist map: %w", err)
	}
	if !found {
		return errors.New("get pmu_watchlist map: map not found")
	}
	p.generation++
	for id := range p.watchlist {
		generation := p.generation
		if err := watchlistMap.Update(&id, &generation, ebpf.UpdateExist); err != nil {
			return fmt.Errorf("refresh PMU watchlist generation: %w", err)
		}
	}
	return nil
}

func (p *Probe) clearWatchlistState() {
	p.watchlist = nil
	p.stats.WatchlistSize = 0
}

// ReplaceWatchlist disables collection while atomically replacing the cgroup set.
func (p *Probe) ReplaceWatchlist(request model.WatchlistRequest) (model.WatchlistResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loadMask == 0 {
		return model.WatchlistResponse{}, nil
	}
	if len(request.CgroupIDs) > p.config.MaxTrackedCgroups || len(request.CgroupIDs) > maxTrackedCgroups {
		return model.WatchlistResponse{}, fmt.Errorf("watchlist has %d cgroups, maximum is %d", len(request.CgroupIDs), p.config.MaxTrackedCgroups)
	}
	unique := make(map[uint64]struct{}, len(request.CgroupIDs))
	for _, id := range request.CgroupIDs {
		if id == 0 {
			return model.WatchlistResponse{}, errors.New("watchlist contains zero cgroup ID")
		}
		if _, exists := unique[id]; exists {
			return model.WatchlistResponse{}, fmt.Errorf("watchlist contains duplicate cgroup ID %d", id)
		}
		unique[id] = struct{}{}
	}

	if err := p.writePMUConfig(false); err != nil {
		p.stats.AttachErrors++
		return model.WatchlistResponse{}, fmt.Errorf("disable PMU gate: %w", err)
	}
	watchlistMap, found, err := p.mgr.GetMap("pmu_watchlist")
	if err != nil {
		p.stats.AttachErrors++
		p.clearWatchlistState()
		return model.WatchlistResponse{}, fmt.Errorf("get pmu_watchlist map: %w", err)
	}
	if !found {
		p.stats.AttachErrors++
		p.clearWatchlistState()
		return model.WatchlistResponse{}, errors.New("get pmu_watchlist map: map not found")
	}
	var key, value uint64
	iterator := watchlistMap.Iterate()
	var oldKeys []uint64
	for iterator.Next(&key, &value) {
		oldKeys = append(oldKeys, key)
	}
	if err := iterator.Err(); err != nil {
		p.stats.AttachErrors++
		p.clearWatchlistState()
		return model.WatchlistResponse{}, fmt.Errorf("iterate PMU watchlist: %w", err)
	}
	for _, oldKey := range oldKeys {
		if err := watchlistMap.Delete(&oldKey); err != nil {
			p.stats.AttachErrors++
			p.clearWatchlistState()
			return model.WatchlistResponse{}, fmt.Errorf("clear PMU watchlist: %w", err)
		}
	}

	p.generation++
	for id := range unique {
		generation := p.generation
		if err := watchlistMap.Update(&id, &generation, ebpf.UpdateNoExist); err != nil {
			p.stats.AttachErrors++
			p.clearWatchlistState()
			return model.WatchlistResponse{}, fmt.Errorf("update PMU watchlist: %w", err)
		}
	}
	p.watchlist = unique
	p.stats.WatchlistSize = len(unique)
	p.stats.EligibleCgroups = request.EligibleCgroups
	p.stats.LastRotation = time.Now().Unix()
	if err := p.writePMUConfig(len(unique) > 0 && p.stats.EffectiveEventMask != 0); err != nil {
		p.stats.AttachErrors++
		p.clearWatchlistState()
		return model.WatchlistResponse{}, fmt.Errorf("enable PMU gate: %w", err)
	}
	return model.WatchlistResponse{Generation: p.generation, Size: len(unique)}, nil
}

// GetStats returns a consistent snapshot of PMU module statistics.
func (p *Probe) GetStats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// Close releases all associated resources.
func (p *Probe) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping ebpf manager: %s", err)
		}
	}
	// Stop detaches all handlers before their perf-event backing FDs are closed.
	p.perf.close()
}

// GetAndFlush gets the scheduling and PMU stats.
func (p *Probe) GetAndFlush() []model.NoisyNeighborStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.reconcileOnlineCPUs()

	byCgroup := make(map[uint64]*model.NoisyNeighborStats)
	p.readSchedulingStats(byCgroup)
	p.readPMUErrors()
	p.readPMUStats(byCgroup)
	ids := make([]uint64, 0, len(byCgroup))
	for id := range byCgroup {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	results := make([]model.NoisyNeighborStats, 0, len(ids))
	for _, id := range ids {
		results = append(results, *byCgroup[id])
	}
	return results
}

func (p *Probe) readPMUErrors() {
	if p.loadMask&model.HardwareEventMask == 0 {
		return
	}
	errorMap, found, err := p.mgr.GetMap("pmu_error_stats")
	if err != nil || !found {
		p.stats.ReadErrors++
		return
	}
	key := uint32(0)
	var perCPU []ebpfPmuErrorStats
	if err := errorMap.Lookup(&key, &perCPU); err != nil {
		p.stats.ReadErrors++
		return
	}
	for i := range perCPU {
		if math.MaxUint64-p.stats.ReadErrors < perCPU[i].Read_errors {
			p.stats.ReadErrors = math.MaxUint64
		} else {
			p.stats.ReadErrors += perCPU[i].Read_errors
		}
		perCPU[i].Read_errors = 0
	}
	if err := errorMap.Update(&key, perCPU, ebpf.UpdateAny); err != nil {
		p.stats.ReadErrors++
	}
}

func (p *Probe) readSchedulingStats(byCgroup map[uint64]*model.NoisyNeighborStats) {
	aggMap, found, err := p.mgr.GetMap("cgroup_agg_stats")
	if err != nil || !found {
		p.stats.ReadErrors++
		log.Errorf("failed to get cgroup_agg_stats map: found=%t: %v", found, err)
		return
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
			stat.UniquePidCount = max(stat.UniquePidCount, cpuStat.Pid_count)
		}
		cgroupsToDelete = append(cgroupsToDelete, cgroupID)
		if stat.EventCount != 0 {
			copy := stat
			byCgroup[cgroupID] = &copy
		}
	}
	if err := iter.Err(); err != nil {
		p.stats.ReadErrors++
		log.Errorf("error iterating cgroup_agg_stats map: %v", err)
	}
	for _, id := range cgroupsToDelete {
		if err := aggMap.Delete(&id); err != nil {
			p.stats.ReadErrors++
			log.Errorf("failed to delete cgroup %d from agg map: %v", id, err)
		}
	}
}

func (p *Probe) readPMUStats(byCgroup map[uint64]*model.NoisyNeighborStats) {
	if p.loadMask == 0 {
		return
	}
	pmuMap, found, err := p.mgr.GetMap("pmu_cgroup_stats")
	if err != nil || !found {
		p.stats.ReadErrors++
		return
	}
	iter := pmuMap.Iterate()
	var cgroupID uint64
	var perCPUStats []ebpfPmuCgroupStats
	var cgroupsToDelete []uint64
	for iter.Next(&cgroupID, &perCPUStats) {
		stat := byCgroup[cgroupID]
		if stat == nil {
			stat = &model.NoisyNeighborStats{CgroupID: cgroupID}
			byCgroup[cgroupID] = stat
		}
		var counters [6]ebpfPmuCounter
		var migrations uint64
		var sampledMask uint64
		var invalidMask uint64
		for _, cpuStats := range perCPUStats {
			for i := range counters {
				bit := uint64(1) << i
				if cpuStats.Sampled_event_mask&bit == 0 {
					continue
				}
				sampledMask |= bit
				if !addCounter(&counters[i], cpuStats.Counters[i]) {
					invalidMask |= bit
					p.stats.ScalingErrors++
				}
			}
			if cpuStats.Sampled_event_mask&model.EventCPUMigrations != 0 {
				sampledMask |= model.EventCPUMigrations
				if math.MaxUint64-migrations < cpuStats.Cpu_migrations {
					invalidMask |= model.EventCPUMigrations
					p.stats.ScalingErrors++
				} else {
					migrations += cpuStats.Cpu_migrations
				}
			}
		}
		validMask := sampledMask &^ invalidMask
		values := []*uint64{&stat.Cycles, &stat.Instructions, &stat.CacheMisses, &stat.CacheReferences, &stat.ITLBMisses, &stat.BranchMisses}
		for i := range counters {
			bit := uint64(1) << i
			if validMask&bit == 0 {
				continue
			}
			value, ok := scaleCounter(counters[i].Value, counters[i].Enabled, counters[i].Running)
			if !ok {
				p.stats.ScalingErrors++
				continue
			}
			*values[i] = value
			stat.SampledEventMask |= bit
		}
		if validMask&model.EventCPUMigrations != 0 {
			stat.CPUMigrations = migrations
			stat.SampledEventMask |= model.EventCPUMigrations
		}
		cgroupsToDelete = append(cgroupsToDelete, cgroupID)
	}
	if err := iter.Err(); err != nil {
		p.stats.ReadErrors++
	}
	for _, id := range cgroupsToDelete {
		if err := pmuMap.Delete(&id); err != nil {
			p.stats.ReadErrors++
		}
	}
}

func addCounter(dst *ebpfPmuCounter, src ebpfPmuCounter) bool {
	if math.MaxUint64-dst.Value < src.Value || math.MaxUint64-dst.Enabled < src.Enabled || math.MaxUint64-dst.Running < src.Running {
		return false
	}
	dst.Value += src.Value
	dst.Enabled += src.Enabled
	dst.Running += src.Running
	return true
}

func (p *Probe) reconcileOnlineCPUs() {
	if p.loadMask&model.HardwareEventMask == 0 {
		return
	}
	cpus, err := readOnlineCPUs()
	if err != nil {
		p.stats.AttachErrors++
		return
	}
	if slices.Equal(cpus, p.cpus) {
		return
	}
	if err := p.writePMUConfig(false); err != nil {
		p.stats.AttachErrors++
		return
	}
	newEvents, effectiveHardwareMask, openErrors := openPerfEvents(p.loadMask, cpus, systemPerfEventOpener{})
	p.stats.AttachErrors += openErrors
	mapCPUs := append(slices.Clone(p.cpus), cpus...)
	slices.Sort(mapCPUs)
	mapCPUs = slices.Compact(mapCPUs)
	if err := p.clearPerfEventMaps(mapCPUs); err != nil {
		p.stats.AttachErrors++
		newEvents.close()
		return
	}
	if err := p.populatePerfEventMaps(newEvents); err != nil {
		p.stats.AttachErrors++
		if clearErr := p.clearPerfEventMaps(mapCPUs); clearErr != nil {
			p.stats.AttachErrors++
		}
		newEvents.close()
		return
	}
	p.perf.close()
	p.perf = newEvents
	p.cpus = cpus
	p.stats.OnlineCPUs = len(cpus)
	p.stats.PerfFDCount = newEvents.fdCount()
	p.stats.EffectiveEventMask = effectiveHardwareMask | p.loadMask&model.EventCPUMigrations
	if err := p.advanceWatchlistGeneration(); err != nil {
		p.stats.AttachErrors++
		p.clearWatchlistState()
		return
	}
	if err := p.writePMUConfig(len(p.watchlist) > 0 && p.stats.EffectiveEventMask != 0); err != nil {
		p.stats.AttachErrors++
		p.clearWatchlistState()
	}
}

func (p *Probe) clearPerfEventMaps(cpus []uint) error {
	for _, event := range hardwareEvents {
		perfMap, found, err := p.mgr.GetMap(event.mapName)
		if err != nil {
			return fmt.Errorf("get %s map: %w", event.mapName, err)
		}
		if !found {
			return fmt.Errorf("get %s map: map not found", event.mapName)
		}
		for _, cpu := range cpus {
			key := uint32(cpu)
			if err := perfMap.Delete(&key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
				return fmt.Errorf("clear %s for CPU %d: %w", event.mapName, cpu, err)
			}
		}
	}
	return nil
}
