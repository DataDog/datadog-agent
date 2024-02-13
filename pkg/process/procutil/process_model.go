// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procutil

import (
	"github.com/DataDog/gopsutil/cpu"
	// using process.FilledProcess
	"github.com/DataDog/gopsutil/process"
)

// Process holds all relevant metadata and metrics for a process
type Process struct {
	Pid      int32
	Ppid     int32
	NsPid    int32 // process namespaced PID
	Name     string
	Cwd      string
	Exe      string
	Comm     string
	Cmdline  []string
	Username string // (Windows only)
	Uids     []int32
	Gids     []int32
	Stats    *Stats
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p *Process) GetPid() int32 {
	return p.Pid
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p *Process) GetCommand() string {
	return p.Comm
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p *Process) GetCmdline() []string {
	return p.Cmdline
}

// DeepCopy creates a deep copy of Process
func (p *Process) DeepCopy() *Process {
	//nolint:revive // TODO(PROC) Fix revive linter
	copy := &Process{
		Pid:      p.Pid,
		Ppid:     p.Ppid,
		NsPid:    p.NsPid,
		Name:     p.Name,
		Cwd:      p.Cwd,
		Exe:      p.Exe,
		Username: p.Username,
	}
	copy.Cmdline = make([]string, len(p.Cmdline))
	for i := range p.Cmdline {
		copy.Cmdline[i] = p.Cmdline[i]
	}
	copy.Uids = make([]int32, len(p.Uids))
	for i := range p.Uids {
		copy.Uids[i] = p.Uids[i]
	}
	copy.Gids = make([]int32, len(p.Gids))
	for i := range p.Gids {
		copy.Gids[i] = p.Gids[i]
	}
	if p.Stats != nil {
		copy.Stats = p.Stats.DeepCopy()
	}
	return copy
}

// Stats holds all relevant stats metrics of a process
type Stats struct {
	CreateTime int64
	// Status returns the process status.
	// Return value could be one of these.
	// R: Running S: Sleep T: Stop I: Idle
	// Z: Zombie W: Wait L: Lock
	// The character is the same within all supported platforms.
	Status      string
	Nice        int32
	OpenFdCount int32
	NumThreads  int32
	CPUPercent  *CPUPercentStat
	CPUTime     *CPUTimesStat
	MemInfo     *MemoryInfoStat
	MemInfoEx   *MemoryInfoExStat
	IOStat      *IOCountersStat
	IORateStat  *IOCountersRateStat
	CtxSwitches *NumCtxSwitchesStat
}

// DeepCopy creates a deep copy of Stats
func (s *Stats) DeepCopy() *Stats {
	//nolint:revive // TODO(PROC) Fix revive linter
	copy := &Stats{
		CreateTime:  s.CreateTime,
		Status:      s.Status,
		Nice:        s.Nice,
		OpenFdCount: s.OpenFdCount,
		NumThreads:  s.NumThreads,
	}
	if s.CPUTime != nil {
		copy.CPUTime = &CPUTimesStat{}
		*copy.CPUTime = *s.CPUTime
	}
	if s.CPUPercent != nil {
		copy.CPUPercent = &CPUPercentStat{}
		*copy.CPUPercent = *s.CPUPercent
	}
	if s.MemInfo != nil {
		copy.MemInfo = &MemoryInfoStat{}
		*copy.MemInfo = *s.MemInfo
	}
	if s.MemInfoEx != nil {
		copy.MemInfoEx = &MemoryInfoExStat{}
		*copy.MemInfoEx = *s.MemInfoEx
	}
	if s.IOStat != nil {
		copy.IOStat = &IOCountersStat{}
		*copy.IOStat = *s.IOStat
	}
	if s.IORateStat != nil {
		copy.IORateStat = &IOCountersRateStat{}
		*copy.IORateStat = *s.IORateStat
	}
	if s.CtxSwitches != nil {
		copy.CtxSwitches = &NumCtxSwitchesStat{}
		*copy.CtxSwitches = *s.CtxSwitches
	}
	return copy
}

// StatsWithPerm is a collection of stats that require elevated permission to collect in linux
type StatsWithPerm struct {
	OpenFdCount int32
	IOStat      *IOCountersStat
}

// CPUTimesStat holds CPU stat metrics of a process
type CPUTimesStat struct {
	User      float64
	System    float64
	Idle      float64
	Nice      float64
	Iowait    float64
	Irq       float64
	Softirq   float64
	Steal     float64
	Guest     float64
	GuestNice float64
	Stolen    float64
	Timestamp int64
}

// Total returns the total number of seconds in a CPUTimesStat
func (c *CPUTimesStat) Total() float64 {
	total := c.User + c.System + c.Nice + c.Iowait + c.Irq + c.Softirq + c.Steal + c.Guest + c.GuestNice + c.Idle + c.Stolen
	return total
}

// CPUPercentStat holds CPU stat metrics of a process as CPU usage percent
type CPUPercentStat struct {
	UserPct   float64
	SystemPct float64
}

// MemoryInfoStat holds commonly used memory metrics for a process
type MemoryInfoStat struct {
	RSS  uint64 // bytes
	VMS  uint64 // bytes
	Swap uint64 // bytes
}

// MemoryInfoExStat holds all memory metrics for a process
type MemoryInfoExStat struct {
	RSS    uint64 // bytes
	VMS    uint64 // bytes
	Shared uint64 // bytes
	Text   uint64 // bytes
	Lib    uint64 // bytes
	Data   uint64 // bytes
	Dirty  uint64 // bytes
}

// IOCountersStat holds IO metrics for a process
type IOCountersStat struct {
	ReadCount  int64
	WriteCount int64
	ReadBytes  int64
	WriteBytes int64
}

// IsZeroValue checks whether all fields are 0 in value for IOCountersStat
func (i *IOCountersStat) IsZeroValue() bool {
	return i.ReadCount == 0 && i.WriteCount == 0 && i.ReadBytes == 0 && i.WriteBytes == 0
}

// IOCountersRateStat holds IO metrics for a process represented as rates (/sec)
type IOCountersRateStat struct {
	ReadRate       float64
	WriteRate      float64
	ReadBytesRate  float64
	WriteBytesRate float64
}

// NumCtxSwitchesStat holds context switch metrics for a process
type NumCtxSwitchesStat struct {
	Voluntary   int64
	Involuntary int64
}

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
