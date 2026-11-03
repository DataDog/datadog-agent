// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procinfo

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	initialSystemProcessInfoBytes = 256 << 10
	maxSystemProcessInfoBytes     = 32 << 20
	filetimeUnixEpochTicks        = uint64(116_444_736_000_000_000)
	maxDurationTicks              = uint64((1<<63 - 1) / 100)
	actualProcessEntry32Bytes     = uint32(unsafe.Sizeof(windows.ProcessEntry32{}))
)

// Keep the architecture-specific ABI constants synchronized with x/sys.
var (
	_ [sizeofProcessEntry32 - actualProcessEntry32Bytes]byte
	_ [actualProcessEntry32Bytes - sizeofProcessEntry32]byte
)

var (
	kernel32DLL             = windows.NewLazySystemDLL("kernel32.dll")
	globalMemoryStatusExDLL = kernel32DLL.NewProc("GlobalMemoryStatusEx")
)

type windowsProcessMemory struct {
	rssKiB uint64
	vszKiB uint64
}

type systemProcessInfoQuery func(buffer []byte) (returned uint32, err error)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func listAll(ctx context.Context, _ string, metrics Metrics) ([]ProcInfo, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("ps: CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var procs []ProcInfo
	var entry windows.ProcessEntry32
	entry.Size = sizeofProcessEntry32
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("ps: Process32First: %w", err)
	}

	for {
		if ctx.Err() != nil {
			break
		}
		if len(procs) >= MaxProcesses {
			break
		}
		info := processEntryToProc(&entry)
		procs = append(procs, info)

		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err != windows.ERROR_NO_MORE_FILES {
				return procs, fmt.Errorf("ps: Process32Next: %w", err)
			}
			break
		}
	}
	enrichWindowsProcesses(ctx, procs, metrics)
	return procs, nil
}

func getSession(ctx context.Context, procPath string, metrics Metrics) ([]ProcInfo, error) {
	all, err := listAll(ctx, procPath, 0)
	if err != nil {
		return nil, err
	}
	// Walk PPID chain upward from current process.
	byPID := make(map[int]ProcInfo, len(all))
	for _, p := range all {
		byPID[p.PID] = p
	}

	ancestors := collectAncestorPIDs(ctx, byPID, os.Getpid(), 0)

	var result []ProcInfo
	for _, p := range all {
		if ctx.Err() != nil {
			break
		}
		if ancestors[p.PID] {
			result = append(result, p)
		}
	}
	enrichWindowsProcesses(ctx, result, metrics)
	return result, nil
}

func getByPIDs(ctx context.Context, procPath string, pids []int, metrics Metrics) ([]ProcInfo, error) {
	all, err := listAll(ctx, procPath, 0)
	if err != nil {
		return nil, err
	}
	wanted := make(map[int]bool, len(pids))
	for _, pid := range pids {
		wanted[pid] = true
	}
	var result []ProcInfo
	for _, p := range all {
		if ctx.Err() != nil {
			break
		}
		if wanted[p.PID] {
			result = append(result, p)
		}
	}
	enrichWindowsProcesses(ctx, result, metrics)
	return result, nil
}

func processEntryToProc(e *windows.ProcessEntry32) ProcInfo {
	pid := int(e.ProcessID)
	ppid := int(e.ParentProcessID)

	cmd := windows.UTF16ToString(e.ExeFile[:])

	return ProcInfo{
		PID:   pid,
		PPID:  ppid,
		UID:   "?",
		State: "?",
		TTY:   "?",
		CPU:   0,
		STime: "-",
		Time:  "-",
		Cmd:   truncateCmdName(cmd),
	}
}

func enrichWindowsProcesses(ctx context.Context, procs []ProcInfo, metrics Metrics) {
	if metrics == 0 || len(procs) == 0 {
		return
	}

	const memoryMetrics = MetricRSS | MetricVSZ | MetricPMem
	const timeMetrics = MetricStartTime | MetricCPUTime | MetricElapsed | MetricPCPU

	var memoryByPID map[uint32]windowsProcessMemory
	if metrics&memoryMetrics != 0 {
		memoryByPID, _ = querySystemProcessMemory()
	}

	var totalPhys uint64
	var totalPhysAvailable bool
	if metrics.Has(MetricPMem) {
		totalPhys, totalPhysAvailable = queryTotalPhysicalMemory()
	}

	now := time.Now()
	for i := range procs {
		if ctx.Err() != nil {
			return
		}

		if pid, ok := windowsPID(procs[i].PID); ok {
			if memory, found := memoryByPID[pid]; found {
				applyWindowsMemory(&procs[i], metrics, memory, totalPhys, totalPhysAvailable)
			}
		}
		if metrics&timeMetrics != 0 {
			queryWindowsProcessTimes(&procs[i], metrics, now)
		}
	}
}

func windowsPID(pid int) (uint32, bool) {
	if pid < 0 || uint64(pid) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(pid), true
}

func applyWindowsMemory(
	info *ProcInfo,
	metrics Metrics,
	memory windowsProcessMemory,
	totalPhys uint64,
	totalPhysAvailable bool,
) {
	if metrics.Has(MetricRSS) {
		info.RSSKiB = memory.rssKiB
		info.Available |= MetricRSS
	}
	if metrics.Has(MetricVSZ) {
		info.VSZKiB = memory.vszKiB
		info.Available |= MetricVSZ
	}
	if metrics.Has(MetricPMem) && totalPhysAvailable && totalPhys > 0 {
		info.PMem = float64(memory.rssKiB) * 1024 / float64(totalPhys) * 100
		info.Available |= MetricPMem
	}
}

func querySystemProcessMemory() (map[uint32]windowsProcessMemory, bool) {
	return querySystemProcessMemoryWith(func(buffer []byte) (uint32, error) {
		var returned uint32
		err := windows.NtQuerySystemInformation(
			windows.SystemProcessInformation,
			unsafe.Pointer(&buffer[0]),
			uint32(len(buffer)),
			&returned,
		)
		runtime.KeepAlive(buffer)
		return returned, err
	})
}

func querySystemProcessMemoryWith(query systemProcessInfoQuery) (map[uint32]windowsProcessMemory, bool) {
	size := uint32(initialSystemProcessInfoBytes)
	for size <= maxSystemProcessInfoBytes {
		buffer := make([]byte, size)
		returned, err := query(buffer)
		if err == nil {
			if returned > 0 {
				if returned > uint32(len(buffer)) {
					return nil, false
				}
				buffer = buffer[:returned]
			}
			return parseSystemProcessMemory(buffer)
		}
		if err != windows.STATUS_INFO_LENGTH_MISMATCH {
			return nil, false
		}

		next, ok := nextSystemProcessInfoSize(size, returned)
		if !ok {
			return nil, false
		}
		size = next
	}
	return nil, false
}

func nextSystemProcessInfoSize(current, returned uint32) (uint32, bool) {
	if current >= maxSystemProcessInfoBytes {
		return 0, false
	}
	next := uint64(current) * 2
	if uint64(returned) > next {
		next = uint64(returned)
	}
	if next > maxSystemProcessInfoBytes {
		next = maxSystemProcessInfoBytes
	}
	if next <= uint64(current) {
		return 0, false
	}
	return uint32(next), true
}

func parseSystemProcessMemory(buffer []byte) (map[uint32]windowsProcessMemory, bool) {
	recordSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	recordAlignment := int(unsafe.Alignof(windows.SYSTEM_PROCESS_INFORMATION{}))
	if len(buffer) < recordSize {
		return nil, false
	}

	result := make(map[uint32]windowsProcessMemory)
	offset := 0
	for count := 0; count < MaxProcesses; count++ {
		if offset < 0 || offset > len(buffer)-recordSize {
			return nil, false
		}

		record := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[offset]))
		result[uint32(record.UniqueProcessID)] = windowsProcessMemory{
			rssKiB: uint64(record.WorkingSetSize) / 1024,
			vszKiB: uint64(record.VirtualSize) / 1024,
		}

		next := int(record.NextEntryOffset)
		if next == 0 {
			return result, true
		}
		if next < recordSize || next > len(buffer)-offset || next%recordAlignment != 0 {
			return nil, false
		}
		offset += next
	}

	// The public process list is capped at MaxProcesses. A larger kernel
	// snapshot is valid, but additional records can never be selected.
	return result, true
}

func queryTotalPhysicalMemory() (uint64, bool) {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	ok, _, _ := globalMemoryStatusExDLL.Call(uintptr(unsafe.Pointer(&status)))
	runtime.KeepAlive(&status)
	if ok == 0 || status.TotalPhys == 0 {
		return 0, false
	}
	return status.TotalPhys, true
}

func queryWindowsProcessTimes(info *ProcInfo, metrics Metrics, now time.Time) {
	pid, ok := windowsPID(info.PID)
	if !ok || pid == 0 {
		return
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)

	var creationTime, exitTime, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(handle, &creationTime, &exitTime, &kernelTime, &userTime); err != nil {
		return
	}
	applyWindowsProcessTimes(info, metrics, now, creationTime, kernelTime, userTime)
}

func applyWindowsProcessTimes(
	info *ProcInfo,
	metrics Metrics,
	now time.Time,
	creationTime windows.Filetime,
	kernelTime windows.Filetime,
	userTime windows.Filetime,
) {
	start, startAvailable := windowsFiletimeToTime(creationTime)
	cpuTime, cpuAvailable := windowsCPUTime(kernelTime, userTime)

	if metrics.Has(MetricStartTime) && startAvailable {
		info.StartTime = start
		info.STime = formatStartTime(start, now)
		info.Available |= MetricStartTime
	}
	if metrics.Has(MetricCPUTime) && cpuAvailable {
		info.CPUTime = cpuTime
		info.Time = formatCPUTime(cpuTime)
		info.Available |= MetricCPUTime
	}

	var elapsed time.Duration
	elapsedAvailable := startAvailable && !start.After(now)
	if elapsedAvailable {
		elapsed = now.Sub(start)
	}
	if metrics.Has(MetricElapsed) && elapsedAvailable {
		info.Elapsed = elapsed
		info.Available |= MetricElapsed
	}
	if metrics.Has(MetricPCPU) && cpuAvailable && elapsedAvailable && elapsed > 0 {
		info.PCPU = float64(cpuTime) / float64(elapsed) * 100
		info.CPU = boundedCPUInteger(info.PCPU)
		info.Available |= MetricPCPU
	}
}

func windowsFiletimeTicks(value windows.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}

func windowsFiletimeToTime(value windows.Filetime) (time.Time, bool) {
	ticks := windowsFiletimeTicks(value)
	if ticks < filetimeUnixEpochTicks {
		return time.Time{}, false
	}
	unixTicks := ticks - filetimeUnixEpochTicks
	if unixTicks > maxDurationTicks {
		return time.Time{}, false
	}
	return time.Unix(0, int64(unixTicks*100)).Local(), true
}

func windowsCPUTime(kernelTime, userTime windows.Filetime) (time.Duration, bool) {
	kernelTicks := windowsFiletimeTicks(kernelTime)
	userTicks := windowsFiletimeTicks(userTime)
	if userTicks > ^uint64(0)-kernelTicks {
		return 0, false
	}
	totalTicks := kernelTicks + userTicks
	if totalTicks > maxDurationTicks {
		return 0, false
	}
	return time.Duration(totalTicks * 100), true
}
