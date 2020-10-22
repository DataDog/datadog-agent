package procutil

import (
	"os"
	"path/filepath"

	cpu "github.com/DataDog/gopsutil/cpu"
	process "github.com/DataDog/gopsutil/process"
)

func HostProc(combineWith ...string) string {
	return GetEnv("HOST_PROC", "/proc", combineWith...)
}

func HostSys(combineWith ...string) string {
	return GetEnv("HOST_SYS", "/sys", combineWith...)
}

func HostEtc(combineWith ...string) string {
	return GetEnv("HOST_ETC", "/etc", combineWith...)
}

//GetEnv retrieves the environment variable key. If it does not exist it returns the default.
func GetEnv(key string, fallback string, combineWith ...string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}

	switch len(combineWith) {
	case 0:
		return value
	case 1:
		return filepath.Join(value, combineWith[0])
	default:
		all := make([]string, len(combineWith)+1)
		all[0] = value
		copy(all[1:], combineWith)
		return filepath.Join(all...)
	}
}

func DoesDirExist(filepath string) bool {
	// TODO (SK): There's probably a better way of checking if a directory exists...
	file, err := os.Open(filepath)
	file.Close()
	return err == nil
}

// AssignCPUStat converts gopsutil TimeStat object to CPUTimesStat in procutil
func AssignCPUStat(s cpu.TimesStat) *CPUTimesStat {
	return &CPUTimesStat{
		CPU:       s.CPU,
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

// AssignMemInfo converts gopsutil MemoryInfoStat object to MemoryInfoStat in procutil
func AssignMemInfo(s *process.MemoryInfoStat) *MemoryInfoStat {
	return &MemoryInfoStat{
		RSS:  s.RSS,
		VMS:  s.VMS,
		Swap: s.Swap,
	}
}

// AssignMemInfoEx converts gopsutil MemoryInfoExStat object to MemoryInfoExStat in procutil
func AssignMemInfoEx(s *process.MemoryInfoExStat) *MemoryInfoExStat {
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

// AssignIOStats converts gopsutil IOCounterStat object to IOCountersStat in procutil
func AssignIOStats(s *process.IOCountersStat) *IOCountersStat {
	return &IOCountersStat{
		ReadCount:  s.ReadCount,
		WriteCount: s.WriteCount,
		ReadBytes:  s.ReadBytes,
		WriteBytes: s.WriteBytes,
	}
}

// AssignCtxSwitches converts gopsutil NumCtxSwitchesStat object to NumCtxSwitchesStat in procutil
func AssignCtxSwitches(s *process.NumCtxSwitchesStat) *NumCtxSwitchesStat {
	return &NumCtxSwitchesStat{
		Voluntary:   s.Voluntary,
		Involuntary: s.Involuntary,
	}
}
