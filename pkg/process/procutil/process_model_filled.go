// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (!linux && !windows) || test

package procutil

import (
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/process"
)

// ConvertAllFilledProcesses takes a group of FilledProcess objects and convert them into Process
func ConvertAllFilledProcesses(processes map[int32]*process.FilledProcess) map[int32]*Process {
	result := make(map[int32]*Process, len(processes))
	for pid, p := range processes {
		result[pid] = ConvertFromFilledProcess(p)
	}
	return result
}

// ConvertAllFilledProcessesToStats takes a group of FilledProcess objects and convert them into Stats
func ConvertAllFilledProcessesToStats(processes map[int32]*process.FilledProcess) map[int32]*Stats {
	stats := make(map[int32]*Stats, len(processes))
	for pid, p := range processes {
		stats[pid] = ConvertFilledProcessesToStats(p)
	}
	return stats
}

// ConvertFilledProcessesToStats takes a group of FilledProcess objects and convert them into Stats
func ConvertFilledProcessesToStats(p *process.FilledProcess) *Stats {
	return &Stats{
		CreateTime:  p.CreateTime,
		Status:      p.Status,
		Nice:        p.Nice,
		OpenFdCount: p.OpenFdCount,
		NumThreads:  p.NumThreads,
		CPUTime:     ConvertFromCPUStat(p.CpuTime),
		MemInfo:     ConvertFromMemInfo(p.MemInfo),
		MemInfoEx:   ConvertFromMemInfoEx(p.MemInfoEx),
		IOStat:      ConvertFromIOStats(p.IOStat),
		CtxSwitches: ConvertFromCtxSwitches(p.CtxSwitches),
	}
}

// ConvertFromFilledProcess takes a FilledProcess object and convert it into Process
func ConvertFromFilledProcess(p *process.FilledProcess) *Process {
	return &Process{
		Pid:      p.Pid,
		Ppid:     p.Ppid,
		NsPid:    p.NsPid,
		Name:     p.Name,
		Cwd:      p.Cwd,
		Exe:      p.Exe,
		Cmdline:  p.Cmdline,
		Username: p.Username,
		Uids:     p.Uids,
		Gids:     p.Gids,
		Stats:    ConvertFilledProcessesToStats(p),
	}
}

// ConvertFromCPUStat converts gopsutil TimesStat object to CPUTimesStat in procutil
func ConvertFromCPUStat(s cpu.TimesStat) *CPUTimesStat {
	return &CPUTimesStat{
		User:      s.User,
		System:    s.System,
		Idle:      s.Idle,
		Nice:      s.Nice,
		Iowait:    s.Iowait,
		Irq:       s.Irq,
		Softirq:   s.Softirq,
		Steal:     s.Steal,
		Guest:     s.Guest,
		GuestNice: s.GuestNice,
		Stolen:    s.Stolen,
		Timestamp: s.Timestamp,
	}
}

// ConvertFromMemInfo converts gopsutil MemoryInfoStat object to MemoryInfoStat in procutil
func ConvertFromMemInfo(s *process.MemoryInfoStat) *MemoryInfoStat {
	return &MemoryInfoStat{
		RSS:  s.RSS,
		VMS:  s.VMS,
		Swap: s.Swap,
	}
}

// ConvertFromMemInfoEx converts gopsutil MemoryInfoExStat object to MemoryInfoExStat in procutil
func ConvertFromMemInfoEx(s *process.MemoryInfoExStat) *MemoryInfoExStat {
	return &MemoryInfoExStat{
		RSS:    s.RSS,
		VMS:    s.VMS,
		Shared: s.Shared,
		Text:   s.Text,
		Lib:    s.Lib,
		Data:   s.Data,
		Dirty:  s.Dirty,
	}
}

// ConvertFromIOStats converts gopsutil IOCountersStat object to IOCounterStat in procutil
func ConvertFromIOStats(s *process.IOCountersStat) *IOCountersStat {
	return &IOCountersStat{
		ReadCount:  int64(s.ReadCount),
		WriteCount: int64(s.WriteCount),
		ReadBytes:  int64(s.ReadBytes),
		WriteBytes: int64(s.WriteBytes),
	}
}

// ConvertFromCtxSwitches converts gopsutil NumCtxSwitchesStat object to NumCtxSwitchesStat in procutil
func ConvertFromCtxSwitches(s *process.NumCtxSwitchesStat) *NumCtxSwitchesStat {
	return &NumCtxSwitchesStat{
		Voluntary:   s.Voluntary,
		Involuntary: s.Involuntary,
	}
}
