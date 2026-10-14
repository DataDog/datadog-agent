// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"cmp"
	"slices"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory/model"
)

// clock is a reading of the monotonic clock of bpf_ktime_get_ns() and of the
// wall clock taken at the same time, used to convert the eBPF timestamps.
type clock struct {
	monotonicNs int64
	wallNs      int64
}

// ktimeToWall converts a bpf_ktime_get_ns() timestamp to nanoseconds since
// the Unix epoch. 0 (no timestamp) is kept as is.
func (c clock) ktimeToWall(ktime uint64) int64 {
	if ktime == 0 {
		return 0
	}
	return c.wallNs - (c.monotonicNs - int64(ktime))
}

// buildStats converts the entries of the eBPF maps to one JVMStats per JVM,
// sorted by PID, with the pools sorted by name.
//
// keep filters the JVMs (e.g. the alive ones), containerID resolves the
// container of a JVM.
func buildStats(pools map[ebpfPoolKey]ebpfPoolValue, gcs map[uint32]ebpfGCStats, c clock, keep func(pid uint32) bool, containerID func(pid uint32) string) []model.JVMStats {
	jvms := make(map[uint32]*model.JVMStats)
	get := func(pid uint32) *model.JVMStats {
		if s, ok := jvms[pid]; ok {
			return s
		}
		if !keep(pid) {
			return nil
		}
		s := &model.JVMStats{PID: pid, Pools: []model.PoolStats{}}
		jvms[pid] = s
		return s
	}

	for key, value := range pools {
		s := get(key.Tgid)
		if s == nil {
			continue
		}
		s.Pools = append(s.Pools, model.PoolStats{
			Name:       unix.ByteSliceToString(key.Name[:]),
			Init:       value.Init,
			Used:       value.Used,
			Committed:  value.Committed,
			Max:        value.Max,
			LastUpdate: c.ktimeToWall(value.Timestamp),
		})
	}

	for tgid, gc := range gcs {
		s := get(tgid)
		if s == nil {
			continue
		}
		s.GC = model.GCStats{
			Pauses:       gc.Pauses,
			FullPauses:   gc.Full_pauses,
			PauseTimeNs:  gc.Pause_time_ns,
			LastPauseEnd: c.ktimeToWall(gc.Last_pause_end_ns),
		}
	}

	stats := make([]model.JVMStats, 0, len(jvms))
	for _, s := range jvms {
		slices.SortFunc(s.Pools, func(a, b model.PoolStats) int { return cmp.Compare(a.Name, b.Name) })
		s.ContainerID = containerID(s.PID)
		stats = append(stats, *s)
	}
	slices.SortFunc(stats, func(a, b model.JVMStats) int { return cmp.Compare(a.PID, b.PID) })
	return stats
}
