// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows && !darwin

package procutil

import (
	"errors"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// NewProcessProbe returns a Probe object
func NewProcessProbe(options ...Option) Probe {
	p := &probe{}
	for _, option := range options {
		option(p)
	}
	return p
}

// probe is an implementation of the process probe for platforms other than Windows, Linux, or macOS
type probe struct {
}

func (p *probe) Close() {}

func (p *probe) StatsForPIDs(_ []int32, _ time.Time) (map[int32]*Stats, error) {
	procs, err := p.ProcessesByPID(time.Now(), false)
	if err != nil {
		return nil, err
	}
	stats := make(map[int32]*Stats, len(procs))
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats, nil
}

func (p *probe) ProcessesByPID(_ time.Time, _ bool) (map[int32]*Process, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}
	result := make(map[int32]*Process, len(pids))
	for _, pid := range pids {
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		result[pid] = convertProcess(proc)
	}
	return result, nil
}

func (p *probe) StatsWithPermByPID(_ []int32) (map[int32]*StatsWithPerm, error) {
	return nil, errors.New("StatsWithPermByPID is not implemented in this environment")
}

// convertProcess builds a procutil.Process from an upstream gopsutil Process.
// Fields that cannot be retrieved (permission errors, unimplemented on this OS) are
// left at their zero values
func convertProcess(p *process.Process) *Process {
	result := &Process{
		Pid: p.Pid,
		// pre-initialize fields to avoid nil dereference in callers
		Stats: &Stats{
			CPUTime:     &CPUTimesStat{},
			CtxSwitches: &NumCtxSwitchesStat{},
			MemInfo:     &MemoryInfoStat{},
		},
	}
	if ppid, err := p.Ppid(); err == nil {
		result.Ppid = ppid
	}
	if name, err := p.Name(); err == nil {
		result.Name = name
	}
	if cmdline, err := p.CmdlineSlice(); err == nil {
		result.Cmdline = cmdline
	}
	if cwd, err := p.Cwd(); err == nil {
		result.Cwd = cwd
	}
	if exe, err := p.Exe(); err == nil {
		result.Exe = exe
	}
	if username, err := p.Username(); err == nil {
		result.Username = username
	}
	// Uids/Gids return []uint32 in gopsutil v4; convert to []int32 used by procutil.
	if uids, err := p.Uids(); err == nil {
		result.Uids = make([]int32, len(uids))
		for i, uid := range uids {
			result.Uids[i] = int32(uid)
		}
	}
	if gids, err := p.Gids(); err == nil {
		result.Gids = make([]int32, len(gids))
		for i, gid := range gids {
			result.Gids[i] = int32(gid)
		}
	}
	if createTime, err := p.CreateTime(); err == nil {
		result.Stats.CreateTime = createTime
	}
	// Status returns []string in gopsutil v4; use the first element.
	if statuses, err := p.Status(); err == nil && len(statuses) > 0 {
		result.Stats.Status = statuses[0]
	}
	if nice, err := p.Nice(); err == nil {
		result.Stats.Nice = nice
	}
	if numFDs, err := p.NumFDs(); err == nil {
		result.Stats.OpenFdCount = numFDs
	}
	if numThreads, err := p.NumThreads(); err == nil {
		result.Stats.NumThreads = numThreads
	}
	if times, err := p.Times(); err == nil {
		result.Stats.CPUTime = &CPUTimesStat{
			User:      times.User,
			System:    times.System,
			Idle:      times.Idle,
			Nice:      times.Nice,
			Iowait:    times.Iowait,
			Irq:       times.Irq,
			Softirq:   times.Softirq,
			Steal:     times.Steal,
			Guest:     times.Guest,
			GuestNice: times.GuestNice,
		}
	}
	if memInfo, err := p.MemoryInfo(); err == nil {
		result.Stats.MemInfo = &MemoryInfoStat{
			RSS:  memInfo.RSS,
			VMS:  memInfo.VMS,
			Swap: memInfo.Swap,
		}
	}
	// IOStat and MemInfoEx are nil when not available; callers already nil-check these.
	if ioStat, err := p.IOCounters(); err == nil {
		result.Stats.IOStat = &IOCountersStat{
			ReadCount:  int64(ioStat.ReadCount),
			WriteCount: int64(ioStat.WriteCount),
			ReadBytes:  int64(ioStat.ReadBytes),
			WriteBytes: int64(ioStat.WriteBytes),
		}
	}
	if ctxSwitches, err := p.NumCtxSwitches(); err == nil {
		result.Stats.CtxSwitches = &NumCtxSwitchesStat{
			Voluntary:   ctxSwitches.Voluntary,
			Involuntary: ctxSwitches.Involuntary,
		}
	}
	return result
}
