// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package syscalllatency

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/syscalllatency/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Raw tracepoints are available from kernel 4.17, no BTF required —
// broader coverage than lock contention (which needs 5.14 + BTF).
const minimumKernelMajor = 4
const minimumKernelMinor = 17

// syscallNames maps SYSCALL_SLOT_* enum values to metric tag strings.
// Order must match the enum in syscall-latency-kern-user.h.
var syscallNames = []string{
	"read",        // 0
	"write",       // 1
	"pread64",     // 2
	"pwrite64",    // 3
	"poll",        // 4
	"select",      // 5
	"mmap",        // 6
	"munmap",      // 7
	"connect",     // 8
	"accept",      // 9
	"accept4",     // 10
	"futex",       // 11
	"epoll_wait",  // 12
	"epoll_pwait", // 13
	"clone",       // 14
	"execve",      // 15
	"io_uring",    // 16
}

const syscallSlotMax = 17 // must match SYSCALL_SLOT_MAX in the C header

// Probe is the eBPF side of the syscall latency check.
type Probe struct {
	mgr *ddebpf.Manager
}

// NewProbe creates and starts the eBPF probe.
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %w", err)
	}
	major, minor := kv.Major(), kv.Minor()
	if major < minimumKernelMajor || (major == minimumKernelMajor && minor < minimumKernelMinor) {
		return nil, fmt.Errorf("syscall latency requires kernel >= %d.%d, got %d.%d",
			minimumKernelMajor, minimumKernelMinor, major, minor)
	}

	p := &Probe{}

	filename := "syscall-latency.o"
	if cfg.BPFDebug {
		filename = "syscall-latency-debug.o"
	}

	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		p.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{}, "syscall_latency", &ebpftelemetry.ErrorsTelemetryModifier{})
		const uid = "syslat"
		p.mgr.Probes = []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "raw_tp__sys_enter", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "raw_tp__sys_exit", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "tid_entry"},
			{Name: "syscall_stats"},
		}
		if err := p.mgr.InitWithOptions(buf, &opts); err != nil {
			return fmt.Errorf("failed to init eBPF manager: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := p.mgr.Start(); err != nil {
		return nil, err
	}

	ddebpf.AddNameMappings(p.mgr.Manager, "syscall_latency")
	ddebpf.AddProbeFDMappings(p.mgr.Manager)
	return p, nil
}

// Close releases all resources held by the probe.
func (p *Probe) Close() {
	if p.mgr != nil {
		ddebpf.RemoveNameMappings(p.mgr.Manager)
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping syscall latency eBPF manager: %s", err)
		}
	}
}

// GetAndFlush reads per-(container, slot) stats from the per-CPU hash map,
// aggregates across CPUs, resets max_time_ns for the next interval, and
// returns the results. Entries with zero count are skipped.
func (p *Probe) GetAndFlush() []model.SyscallLatencyStats {
	statsMap, found, err := p.mgr.GetMap("syscall_stats")
	if err != nil {
		log.Errorf("failed to get syscall_stats map: %v", err)
		return nil
	}
	if !found {
		log.Warn("syscall_stats map not found")
		return nil
	}

	var result []model.SyscallLatencyStats

	var key ebpfCgroupStatsKey
	var perCPU []ebpfSyscallStats
	it := statsMap.Iterate()
	for it.Next(&key, &perCPU) {
		slot := key.Slot
		if int(slot) >= syscallSlotMax {
			continue
		}

		var totalTime, count, maxTime, slowCount uint64
		for _, s := range perCPU {
			totalTime += s.Total_time_ns
			count += s.Count
			slowCount += s.Slow_count
			if s.Max_time_ns > maxTime {
				maxTime = s.Max_time_ns
			}
		}

		if count == 0 {
			continue
		}

		cgroupName := unix.ByteSliceToString(key.Cgroup_name[:])
		containerID, _ := cgroups.ContainerFilter("", cgroupName)

		result = append(result, model.SyscallLatencyStats{
			Syscall:     syscallNames[slot],
			CgroupName:  cgroupName,
			ContainerID: containerID,
			TotalTimeNs: totalTime,
			Count:       count,
			MaxTimeNs:   maxTime,
			SlowCount:   slowCount,
		})

		// Reset max_time_ns for per-interval gauge semantics.
		// Same race trade-off as lock contention: a concurrent update between
		// Iterate and Put may be lost; it will appear in the next interval.
		for i := range perCPU {
			perCPU[i].Max_time_ns = 0
		}
		keyCopy := key
		if err := statsMap.Put(&keyCopy, perCPU); err != nil {
			log.Warnf("failed to reset max_time_ns for slot %d (%s) cgroup %s: %v",
				slot, syscallNames[slot], cgroupName, err)
		}
	}
	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate syscall_stats map: %v", err)
	}

	return result
}
