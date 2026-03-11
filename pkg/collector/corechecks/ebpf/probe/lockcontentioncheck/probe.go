// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package lockcontentioncheck is the system-probe side of the lock contention check.
package lockcontentioncheck

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/lockcontentioncheck/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// contention_begin/contention_end tracepoints available since kernel 5.14
var minimumKernelVersion = kernel.VersionCode(5, 14, 0)

// lockTypeNames maps lock_type_key_t enum values to human-readable names
var lockTypeNames = []string{
	"spinlock",
	"mutex",
	"rwsem_read",
	"rwsem_write",
	"rwlock_read",
	"rwlock_write",
	"rt_mutex",
	"pcpu_spinlock",
	"other",
}

const lockTypeMax = 9 // must match LOCK_TYPE_MAX in the eBPF C code

// Probe is the eBPF side of the lock contention check
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

	filename := "lock-contention-check.o"
	if cfg.BPFDebug {
		filename = "lock-contention-check-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		p.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{}, "lock_contention_check", &ebpftelemetry.ErrorsTelemetryModifier{})
		const uid = "lockcon"
		p.mgr.Probes = []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tracepoint__contention_begin", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tracepoint__contention_end", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "tstamp"},
			{Name: "tstamp_cpu"},
			{Name: "lock_contention_stats"},
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
	ddebpf.AddNameMappings(p.mgr.Manager, "lock_contention_check")
	return p, nil
}

// Close releases all associated resources
func (p *Probe) Close() {
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping lock contention ebpf manager: %s", err)
		}
	}
}

// GetAndFlush reads the lock contention stats from the per-CPU array map,
// aggregates across CPUs, resets max_time_ns, and returns the results.
func (p *Probe) GetAndFlush() []model.LockContentionStats {
	statsMap, found, err := p.mgr.GetMap("lock_contention_stats")
	if err != nil {
		log.Errorf("failed to get lock_contention_stats map: %v", err)
		return nil
	}
	if !found {
		log.Warn("lock_contention_stats map not found")
		return nil
	}

	var result []model.LockContentionStats

	for i := uint32(0); i < lockTypeMax; i++ {
		key := i
		var perCPUStats []ebpfLockContentionStats
		if err := statsMap.Lookup(&key, &perCPUStats); err != nil {
			continue
		}

		var totalTime, count, maxTime uint64
		for _, cpuStat := range perCPUStats {
			totalTime += cpuStat.Total_time_ns
			count += cpuStat.Count
			if cpuStat.Max_time_ns > maxTime {
				maxTime = cpuStat.Max_time_ns
			}
		}

		if count == 0 {
			continue
		}

		result = append(result, model.LockContentionStats{
			LockType:    lockTypeNames[i],
			TotalTimeNs: totalTime,
			Count:       count,
			MaxTimeNs:   maxTime,
		})

		// Reset max_time_ns for per-interval semantics.
		// We zero the entire per-CPU entry's max field by writing back zeros.
		// total_time_ns and count are monotonic — the agent computes deltas.
		for j := range perCPUStats {
			perCPUStats[j].Max_time_ns = 0
		}
		if err := statsMap.Put(&key, perCPUStats); err != nil {
			log.Warnf("failed to reset max_time_ns for lock type %d: %v", i, err)
		}
	}

	return result
}
