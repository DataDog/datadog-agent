package procutil

import (
	"github.com/DataDog/gopsutil/cpu"
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
	Cmdline  []string
	Username string // (Windows only)
	Uids     []int32
	Gids     []int32

	Stats *Stats
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
	CPUTime     *CPUTimesStat
	MemInfo     *MemoryInfoStat
	MemInfoEx   *MemoryInfoExStat
	IOStat      *IOCountersStat
	CtxSwitches *NumCtxSwitchesStat
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
	ReadCount  uint64
	WriteCount uint64
	ReadBytes  uint64
	WriteBytes uint64
}

// NumCtxSwitchesStat holds context switch metrics for a process
type NumCtxSwitchesStat struct {
	Voluntary   int64
	Involuntary int64
}

// ConvertAllProcesses takes a group of Process objects and convert them into FilledProcess
func ConvertAllProcesses(processes map[int32]*Process) map[int32]*process.FilledProcess {
	result := make(map[int32]*process.FilledProcess, len(processes))
	for pid, p := range processes {
		result[pid] = ConvertToFilledProcess(p)
	}
	return result
}

// ConvertToFilledProcess takes a Process object and convert it into FilledProcess
func ConvertToFilledProcess(p *Process) *process.FilledProcess {
	return &process.FilledProcess{
		Pid:         p.Pid,
		Ppid:        p.Ppid,
		NsPid:       p.NsPid,
		Cmdline:     p.Cmdline,
		CpuTime:     *ConvertCPUStat(p.Stats.CPUTime),
		Nice:        p.Stats.Nice,
		CreateTime:  p.Stats.CreateTime,
		OpenFdCount: p.Stats.OpenFdCount,
		Name:        p.Name,
		Status:      p.Stats.Status,
		Uids:        p.Uids,
		Gids:        p.Gids,
		NumThreads:  p.Stats.NumThreads,
		CtxSwitches: ConvertCtxSwitches(p.Stats.CtxSwitches),
		MemInfo:     ConvertMemInfo(p.Stats.MemInfo),
		MemInfoEx:   ConvertMemInfoEx(p.Stats.MemInfoEx),
		Cwd:         p.Cwd,
		Exe:         p.Exe,
		IOStat:      ConvertIOStats(p.Stats.IOStat),
		Username:    p.Username,
	}
}

// ConvertCPUStat converts procutil CPUTimeStat object to TimesStat in gopsutil
func ConvertCPUStat(s *CPUTimesStat) *cpu.TimesStat {
	return &cpu.TimesStat{
		CPU:       "cpu",
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

// ConvertMemInfo converts procutil MemoryInfoStat object to MemoryInfoStat in gopsutil
func ConvertMemInfo(s *MemoryInfoStat) *process.MemoryInfoStat {
	return &process.MemoryInfoStat{
		RSS:  s.RSS,
		VMS:  s.VMS,
		Swap: s.Swap,
	}
}

// ConvertMemInfoEx converts procutil MemoryInfoExStat object to MemoryInfoExStat in gopsutil
func ConvertMemInfoEx(s *MemoryInfoExStat) *process.MemoryInfoExStat {
	return &process.MemoryInfoExStat{
		RSS:    s.RSS,
		VMS:    s.VMS,
		Shared: s.Shared,
		Text:   s.Text,
		Lib:    s.Lib,
		Data:   s.Data,
		Dirty:  s.Dirty,
	}
}

// ConvertIOStats converts procutil IOCountersStat object to IOCounterStat in gopsutil
func ConvertIOStats(s *IOCountersStat) *process.IOCountersStat {
	return &process.IOCountersStat{
		ReadCount:  s.ReadCount,
		WriteCount: s.WriteCount,
		ReadBytes:  s.ReadBytes,
		WriteBytes: s.WriteBytes,
	}
}

// ConvertCtxSwitches converts procutil NumCtxSwitchesStat object to NumCtxSwitchesStat in gopsutil
func ConvertCtxSwitches(s *NumCtxSwitchesStat) *process.NumCtxSwitchesStat {
	return &process.NumCtxSwitchesStat{
		Voluntary:   s.Voluntary,
		Involuntary: s.Involuntary,
	}
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
		ReadCount:  s.ReadCount,
		WriteCount: s.WriteCount,
		ReadBytes:  s.ReadBytes,
		WriteBytes: s.WriteBytes,
	}
}

// ConvertFromCtxSwitches converts gopsutil NumCtxSwitchesStat object to NumCtxSwitchesStat in procutil
func ConvertFromCtxSwitches(s *process.NumCtxSwitchesStat) *NumCtxSwitchesStat {
	return &NumCtxSwitchesStat{
		Voluntary:   s.Voluntary,
		Involuntary: s.Involuntary,
	}
}
