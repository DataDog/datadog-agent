// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/shirou/w32"
	"golang.org/x/sys/windows"

	process "github.com/DataDog/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	modpsapi                  = windows.NewLazyDLL("psapi.dll")
	modkernel                 = windows.NewLazyDLL("kernel32.dll")
	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = modkernel.NewProc("GetProcessHandleCount")
	procGetProcessIoCounters  = modkernel.NewProc("GetProcessIoCounters")
)

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

func getProcessMemoryInfo(h windows.Handle, mem *process.PROCESS_MEMORY_COUNTERS) (err error) {
	r1, _, e1 := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(mem)), uintptr(unsafe.Sizeof(*mem)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getProcessHandleCount(h windows.Handle, count *uint32) (err error) {
	r1, _, e1 := procGetProcessHandleCount.Call(uintptr(h), uintptr(unsafe.Pointer(count)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getProcessIoCounters(h windows.Handle, counters *IO_COUNTERS) (err error) {
	r1, _, e1 := procGetProcessIoCounters.Call(uintptr(h), uintptr(unsafe.Pointer(counters)))
	if r1 == 0 {
		return e1
	}
	return nil
}

type windowsToolhelpProbe struct {
	cachedProcesses map[uint32]*cachedProcess
}

// NewWindowsToolhelpProbe provides an implementation of a process probe based on Toolhelp API
func NewWindowsToolhelpProbe() Probe {
	return &windowsToolhelpProbe{
		cachedProcesses: map[uint32]*cachedProcess{},
	}
}

func (p *windowsToolhelpProbe) Close() {}

func (p *windowsToolhelpProbe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	procs, err := p.ProcessesByPID(now, true)
	if err != nil {
		return nil, err
	}
	stats := make(map[int32]*Stats, len(procs))
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats, nil
}

// StatsWithPermByPID is currently not implemented in non-linux environments
func (p *windowsToolhelpProbe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	return nil, fmt.Errorf("windowsToolhelpProbe: StatsWithPermByPID is not implemented")
}

func (p *windowsToolhelpProbe) ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error) {
	// make sure we get the consistent snapshot by using the same OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	allProcsSnap := w32.CreateToolhelp32Snapshot(w32.TH32CS_SNAPPROCESS, 0)
	if allProcsSnap == 0 {
		return nil, windows.GetLastError()
	}
	procs := make(map[int32]*Process)

	defer w32.CloseHandle(allProcsSnap)
	var pe32 w32.PROCESSENTRY32
	pe32.DwSize = uint32(unsafe.Sizeof(pe32))

	knownPids := make(map[uint32]struct{})
	for pid := range p.cachedProcesses {
		knownPids[pid] = struct{}{}
	}

	for success := w32.Process32First(allProcsSnap, &pe32); success; success = w32.Process32Next(allProcsSnap, &pe32) {
		pid := pe32.Th32ProcessID
		ppid := pe32.Th32ParentProcessID

		if pid == 0 {
			// this is the "system idle process".  We'll never be able to open it,
			// which will cause us to thrash WMI once per check, which we don't
			// want to do.
			continue
		}
		cp, ok := p.cachedProcesses[pid]
		if !ok {
			// wasn't already in the map.
			cp = &cachedProcess{}

			if err := cp.fillFromProcEntry(&pe32); err != nil {
				log.Debugf("could not fill Win32 process information for pid %v %v", pid, err)
				continue
			}
			p.cachedProcesses[pid] = cp
		} else {
			var err error
			if cp.procHandle, err = OpenProcessHandle(int32(pe32.Th32ProcessID)); err != nil {
				log.Debugf("Could not reopen process handle for pid %v %v", pid, err)
				continue
			}
		}
		defer cp.close()

		procHandle := cp.procHandle

		// Collect start time
		var CPU windows.Rusage
		if err := windows.GetProcessTimes(procHandle, &CPU.CreationTime, &CPU.ExitTime, &CPU.KernelTime, &CPU.UserTime); err != nil {
			log.Debugf("Could not get process times for %v %v", pid, err)
			continue
		}
		ctime := CPU.CreationTime.Nanoseconds() / 1000000

		var stats *Stats
		if collectStats {
			var handleCount uint32
			if err := getProcessHandleCount(procHandle, &handleCount); err != nil {
				log.Debugf("could not get handle count for %v %v", pid, err)
				continue
			}

			var pmemcounter process.PROCESS_MEMORY_COUNTERS
			if err := getProcessMemoryInfo(procHandle, &pmemcounter); err != nil {
				log.Debugf("could not get memory info for %v %v", pid, err)
				continue
			}

			// shell out to getprocessiocounters for io stats
			var ioCounters IO_COUNTERS
			if err := getProcessIoCounters(procHandle, &ioCounters); err != nil {
				log.Debugf("could not get IO Counters for %v %v", pid, err)
				continue
			}

			utime := float64((int64(CPU.UserTime.HighDateTime) << 32) | int64(CPU.UserTime.LowDateTime))
			stime := float64((int64(CPU.KernelTime.HighDateTime) << 32) | int64(CPU.KernelTime.LowDateTime))

			stats = &Stats{
				CreateTime:  ctime,
				OpenFdCount: int32(handleCount),
				NumThreads:  int32(pe32.CntThreads),
				CPUTime: &CPUTimesStat{
					User:      utime,
					System:    stime,
					Timestamp: time.Now().UnixNano(),
				},
				MemInfo: &MemoryInfoStat{
					RSS:  uint64(pmemcounter.WorkingSetSize),
					VMS:  uint64(pmemcounter.QuotaPagedPoolUsage),
					Swap: 0,
				},
				IOStat: &IOCountersStat{
					ReadCount:  int64(ioCounters.ReadOperationCount),
					WriteCount: int64(ioCounters.WriteOperationCount),
					ReadBytes:  int64(ioCounters.ReadTransferCount),
					WriteBytes: int64(ioCounters.WriteTransferCount),
				},
				CtxSwitches: &NumCtxSwitchesStat{},
			}
		} else {
			stats = &Stats{CreateTime: ctime}
		}

		delete(knownPids, pid)
		procs[int32(pid)] = &Process{
			Pid:      int32(pid),
			Ppid:     int32(ppid),
			Cmdline:  cp.parsedArgs,
			Stats:    stats,
			Exe:      cp.executablePath,
			Username: cp.userName,
		}
	}
	for pid := range knownPids {
		cp := p.cachedProcesses[pid]
		log.Debugf("removing process %v %v", pid, cp.executablePath)
		delete(p.cachedProcesses, pid)
	}

	return procs, nil
}

type cachedProcess struct {
	userName       string
	executablePath string
	commandLine    string
	procHandle     windows.Handle
	parsedArgs     []string
}

func (cp *cachedProcess) fillFromProcEntry(pe32 *w32.PROCESSENTRY32) (err error) {
	cp.procHandle, err = OpenProcessHandle(int32(pe32.Th32ProcessID))
	if err != nil {
		return err
	}
	var usererr error
	cp.userName, usererr = GetUsernameForProcess(cp.procHandle)
	if usererr != nil {
		log.Debugf("Couldn't get process username %v %v", pe32.Th32ProcessID, err)
	}
	var cmderr error
	cp.executablePath = winutil.ConvertWindowsString16(pe32.SzExeFile[:])
	commandParams, cmderr := winutil.GetCommandParamsForProcess(cp.procHandle, false)
	if cmderr != nil {
		log.Debugf("Error retrieving full command line %v", cmderr)
		cp.commandLine = cp.executablePath
	} else {
		cp.commandLine = commandParams.CmdLine
	}

	cp.parsedArgs = ParseCmdLineArgs(cp.commandLine)
	if len(cp.commandLine) > 0 && len(cp.parsedArgs) == 0 {
		log.Warnf("Failed to parse the cmdline:%s for pid:%d", cp.commandLine, pe32.Th32ProcessID)
	}

	return
}

func (cp *cachedProcess) close() {
	if cp.procHandle != windows.Handle(0) {
		windows.CloseHandle(cp.procHandle)
		cp.procHandle = windows.Handle(0)
	}
}

// GetParentPid looks up the parent process given a pid
func GetParentPid(pid uint32) (uint32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var pe32 w32.PROCESSENTRY32
	pe32.DwSize = uint32(unsafe.Sizeof(pe32))

	allProcsSnap := w32.CreateToolhelp32Snapshot(w32.TH32CS_SNAPPROCESS, 0)
	if allProcsSnap == 0 {
		return 0, windows.GetLastError()
	}
	defer w32.CloseHandle(allProcsSnap)
	for success := w32.Process32First(allProcsSnap, &pe32); success; success = w32.Process32Next(allProcsSnap, &pe32) {
		if pid == pe32.Th32ProcessID {
			return pe32.Th32ParentProcessID, nil
		}
	}
	return 0, nil
}
