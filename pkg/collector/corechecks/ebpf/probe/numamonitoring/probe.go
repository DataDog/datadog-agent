// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/numa-monitoring-kern.c pkg/ebpf/bytecode/build/runtime/numa-monitoring.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/numa-monitoring.c pkg/ebpf/bytecode/runtime/numa-monitoring.go runtime

package numamonitoring

import (
	"fmt"
	"os"
	"path/filepath"
	gostdruntime "runtime"
	"slices"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/numamonitoring/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	bytecoderuntime "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const rotationInterval = 60 * time.Second

// 4.10 for bpf_get_numa_node_id, 4.18 for bpf_get_current_cgroup_id
var minimumKernelVersion = kernel.VersionCode(4, 18, 0)

// Probe owns the scheduler program and the procfs/resctrl collectors.
type Probe struct {
	mu sync.Mutex

	mgr         *ddebpf.Manager
	reader      *cgroups.Reader
	resctrl     *resctrlManager
	procRoot    string
	numaNodes   []int
	maxGroups   int
	lastRotate  time.Time
	window      map[uint64]map[int]uint64
	selected    map[uint64]struct{}
	readFailure uint64
	now         func() time.Time
}

// NewProbe starts scheduler instrumentation and discovers host capabilities.
func NewProbe(cfg *ddebpf.Config, maxGroups int) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %s", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	if maxGroups < 1 {
		maxGroups = 1
	}
	probe := &Probe{
		procRoot:  cfg.ProcRoot,
		maxGroups: maxGroups,
		window:    make(map[uint64]map[int]uint64),
		selected:  make(map[uint64]struct{}),
		now:       time.Now,
	}

	load := func(asset bytecode.AssetReader, options manager.Options) error {
		probe.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{}, "numa_monitoring", &ebpftelemetry.ErrorsTelemetryModifier{})
		probe.mgr.Probes = []*manager.Probe{{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tracepoint_numa_sched_switch", UID: "numa"}}}
		probe.mgr.Maps = []*manager.Map{{Name: "numa_runtime"}, {Name: "last_switch_ns"}}
		if err := probe.mgr.InitWithOptions(asset, &options); err != nil {
			return fmt.Errorf("initialize NUMA monitoring eBPF manager: %w", err)
		}
		return nil
	}

	var loadErr error
	if cfg.EnableCORE {
		filename := "numa-monitoring.o"
		if cfg.BPFDebug {
			filename = "numa-monitoring-debug.o"
		}
		loadErr = ddebpf.LoadCOREAsset(filename, load)
		if loadErr != nil && !cfg.AllowRuntimeCompiledFallback {
			return nil, fmt.Errorf("load CO-RE NUMA monitoring probe: %w", loadErr)
		}
	}
	if !cfg.EnableCORE || loadErr != nil {
		if loadErr != nil {
			log.Warnf("CO-RE NUMA monitoring probe failed, falling back to runtime compilation: %v", loadErr)
		}
		flags := []string{"-g"}
		if cfg.BPFDebug {
			flags = append(flags, "-DDEBUG=1")
		}
		compiled, err := bytecoderuntime.NumaMonitoring.Compile(cfg, flags)
		if err != nil {
			return nil, fmt.Errorf("runtime compile NUMA monitoring probe: %w", err)
		}
		loadErr = load(compiled, manager.Options{})
		_ = compiled.Close()
		if loadErr != nil {
			return nil, loadErr
		}
	}
	if err := probe.mgr.Start(); err != nil {
		return nil, fmt.Errorf("start NUMA monitoring eBPF manager: %w", err)
	}
	ddebpf.AddNameMappings(probe.mgr.Manager, "numa_monitoring")

	reader, err := cgroups.NewReader(cgroups.WithProcPath(cfg.ProcRoot), cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		probe.Close()
		return nil, fmt.Errorf("initialize NUMA cgroup reader: %w", err)
	}
	probe.reader = reader

	nodesValue, err := os.ReadFile(kernel.HostSys("devices/system/node/online"))
	if err == nil {
		probe.numaNodes, _ = parseRangeList(string(nodesValue))
	}
	probe.resctrl = newResctrlManager(kernel.HostSys("fs/resctrl"), maxGroups)
	return probe, nil
}

// Close releases eBPF resources and removes only Agent-owned resctrl groups.
func (probe *Probe) Close() {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.resctrl != nil {
		probe.resctrl.close()
	}
	if probe.mgr != nil {
		ddebpf.RemoveNameMappings(probe.mgr.Manager)
		if err := probe.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping NUMA monitoring eBPF manager: %v", err)
		}
		probe.mgr = nil
	}
}

func (probe *Probe) flushRuntime() map[uint64]map[int]uint64 {
	result := make(map[uint64]map[int]uint64)
	runtimeMap, found, err := probe.mgr.GetMap("numa_runtime")
	if err != nil || !found {
		probe.readFailure++
		return result
	}

	iterator := runtimeMap.Iterate()
	var key ebpfRuntimeKey
	var perCPU []ebpfRuntimeValue
	var deleteKeys []ebpfRuntimeKey
	for iterator.Next(&key, &perCPU) {
		if key.Numa_node == ^uint32(0) {
			deleteKeys = append(deleteKeys, key)
			continue
		}
		var runtimeNS uint64
		for _, value := range perCPU {
			runtimeNS += value.Ns
		}
		if runtimeNS > 0 {
			nodes := result[key.Cgroup_id]
			if nodes == nil {
				nodes = make(map[int]uint64)
				result[key.Cgroup_id] = nodes
			}
			nodes[int(key.Numa_node)] += runtimeNS
		}
		deleteKeys = append(deleteKeys, key)
	}
	if iterator.Err() != nil {
		probe.readFailure++
	}
	for index := range deleteKeys {
		if err := runtimeMap.Delete(&deleteKeys[index]); err != nil {
			probe.readFailure++
		}
	}
	return result
}

func (probe *Probe) addWindow(runtimeByCgroup map[uint64]map[int]uint64) {
	for cgroupID, values := range runtimeByCgroup {
		window := probe.window[cgroupID]
		if window == nil {
			window = make(map[int]uint64)
			probe.window[cgroupID] = window
		}
		for node, value := range values {
			window[node] += value
		}
	}
}

func (probe *Probe) selectTop() []uint64 {
	type candidate struct {
		id      uint64
		runtime uint64
	}
	values := make([]candidate, 0, len(probe.window))
	for id, nodes := range probe.window {
		if probe.reader.GetCgroupByInode(id) == nil {
			continue
		}
		var total uint64
		for _, value := range nodes {
			total += value
		}
		values = append(values, candidate{id: id, runtime: total})
	}
	slices.SortFunc(values, func(left, right candidate) int {
		if left.runtime > right.runtime {
			return -1
		}
		if left.runtime < right.runtime {
			return 1
		}
		if left.id < right.id {
			return -1
		}
		if left.id > right.id {
			return 1
		}
		return 0
	})
	if len(values) > probe.maxGroups {
		values = values[:probe.maxGroups]
	}
	result := make([]uint64, 0, len(values))
	for _, value := range values {
		result = append(result, value.id)
	}
	return result
}

func (probe *Probe) cgroupTasks(cgroupID uint64) ([]int, bool) {
	cgroup := probe.reader.GetCgroupByInode(cgroupID)
	if cgroup == nil {
		return nil, false
	}
	pids, err := cgroup.GetPIDs(0)
	if err != nil {
		probe.readFailure++
		return nil, false
	}
	tids := make(map[int]struct{})
	for _, pid := range pids {
		entries, err := os.ReadDir(filepath.Join(probe.procRoot, fmt.Sprintf("%d/task", pid)))
		if err != nil {
			probe.readFailure++
			continue
		}
		for _, entry := range entries {
			var tid int
			if _, err := fmt.Sscanf(entry.Name(), "%d", &tid); err == nil {
				tids[tid] = struct{}{}
			}
		}
	}
	result := make([]int, 0, len(tids))
	for tid := range tids {
		result = append(result, tid)
	}
	slices.Sort(result)
	return result, len(result) > 0
}

func (probe *Probe) residency(cgroupID uint64) map[int]uint64 {
	cgroup := probe.reader.GetCgroupByInode(cgroupID)
	if cgroup == nil {
		return nil
	}
	pids, err := cgroup.GetPIDs(0)
	if err != nil {
		probe.readFailure++
		return nil
	}
	result := make(map[int]uint64)
	pageSize := uint64(os.Getpagesize())
	for _, pid := range pids {
		file, err := os.Open(filepath.Join(probe.procRoot, fmt.Sprintf("%d/numa_maps", pid)))
		if err != nil {
			probe.readFailure++
			continue
		}
		values, parseErr := parseNUMAMaps(file, pageSize)
		_ = file.Close()
		if parseErr != nil {
			probe.readFailure++
			continue
		}
		for node, value := range values {
			result[node] += value
		}
	}
	return result
}

// GetAndFlush returns one polling interval and performs top-cgroup rotation at
// most once per minute.
func (probe *Probe) GetAndFlush() model.Response {
	probe.mu.Lock()
	defer probe.mu.Unlock()

	probe.readFailure = 0
	probe.resctrl.readErrors = 0
	now := probe.now()
	runtimeByCgroup := probe.flushRuntime()
	probe.addWindow(runtimeByCgroup)
	if err := probe.reader.RefreshCgroups(0); err != nil {
		probe.readFailure++
	}

	if probe.lastRotate.IsZero() || now.Sub(probe.lastRotate) >= rotationInterval {
		selectedTasks := make(map[uint64][]int)
		clear(probe.selected)
		for _, cgroupID := range probe.selectTop() {
			if tasks, ok := probe.cgroupTasks(cgroupID); ok {
				probe.selected[cgroupID] = struct{}{}
				selectedTasks[cgroupID] = tasks
			}
		}
		probe.resctrl.rotate(selectedTasks)
		clear(probe.window)
		probe.lastRotate = now
	}

	response := model.Response{}
	for cgroupID := range probe.selected {
		stats := model.ContainerStats{CgroupID: cgroupID}
		stats.RuntimeShares = distribution(runtimeByCgroup[cgroupID])
		stats.ResidentBytes = probe.residency(cgroupID)
		if mismatch, ok := placementMismatch(stats.RuntimeShares, distribution(stats.ResidentBytes)); ok {
			stats.PlacementMismatch = &mismatch
		}
		stats.Domains = probe.resctrl.read(cgroupID, now)
		var total, local float64
		var haveRemoteAttribution bool
		for _, domain := range stats.Domains {
			if domain.TotalBandwidth != nil && domain.LocalBandwidth != nil {
				total += *domain.TotalBandwidth
				local += *domain.LocalBandwidth
				haveRemoteAttribution = true
			}
		}
		if haveRemoteAttribution {
			_, ratio, ok := remoteRatio(total, local)
			if ok {
				stats.RemoteRatio = &ratio
			}
		}
		if stats.PlacementMismatch != nil {
			badness := badnessScore(*stats.PlacementMismatch, stats.RemoteRatio)
			stats.BadnessScore = &badness
		} else if stats.RemoteRatio != nil {
			badness := *stats.RemoteRatio
			stats.BadnessScore = &badness
		}
		response.Containers = append(response.Containers, stats)
	}
	slices.SortFunc(response.Containers, func(left, right model.ContainerStats) int {
		if left.CgroupID < right.CgroupID {
			return -1
		}
		if left.CgroupID > right.CgroupID {
			return 1
		}
		return 0
	})
	response.Status = probe.statusLocked()
	return response
}

// Status returns a snapshot without changing module health or liveness.
func (probe *Probe) Status() model.Status {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	return probe.statusLocked()
}

func (probe *Probe) statusLocked() model.Status {
	status := model.Status{
		Architecture:         gostdruntime.GOARCH,
		NUMANodes:            slices.Clone(probe.numaNodes),
		MonitorFeatures:      slices.Clone(probe.resctrl.features),
		ActiveGroups:         len(probe.resctrl.groups),
		Capacity:             probe.maxGroups,
		ForeignTaskConflicts: probe.resctrl.conflicts,
		ReadFailures:         probe.readFailure + probe.resctrl.readErrors,
	}
	switch {
	case len(status.NUMANodes) == 0:
		status.State = model.StateUnsupported
		status.Message = "host NUMA topology is not exposed"
	case !probe.resctrl.supported():
		status.State = model.StatePartial
		status.Message = "scheduler and residency monitoring active; resctrl monitoring unavailable"
	case status.ForeignTaskConflicts > 0 || status.ReadFailures > 0:
		status.State = model.StatePartial
	default:
		status.State = model.StateActive
	}
	return status
}
