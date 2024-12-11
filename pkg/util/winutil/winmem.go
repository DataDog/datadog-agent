// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32 = windows.NewLazyDLL("kernel32.dll")
	modPsapi    = windows.NewLazyDLL("psapi.dll")

	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetPerformanceInfo   = modPsapi.NewProc("GetPerformanceInfo")
)

// VirtualMemoryStat contains basic metrics for virtual memory
type VirtualMemoryStat struct {
	// Total amount of RAM on this system
	Total uint64

	// RAM available for programs to allocate
	//
	// This value is computed from the kernel specific values.
	Available uint64

	// RAM used by programs
	//
	// This value is computed from the kernel specific values.
	Used uint64

	// Percentage of RAM used by programs
	//
	// This value is computed from the kernel specific values.
	UsedPercent float64
}

// PagefileStat contains basic metrics for the windows pagefile
type PagefileStat struct {
	// The current committed memory limit for the system or
	// the current process, whichever is smaller, in bytes
	Total uint64

	// The maximum amount of memory the current process can commit, in bytes.
	// This value is equal to or smaller than the system-wide available commit
	// value.
	Available uint64

	// Used is Total - Available
	Used uint64

	// UsedPercent is used as a percentage of the total pagefile
	UsedPercent float64
}

// SwapMemoryStat contains swap statistics
type SwapMemoryStat struct {
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
}

type memoryStatusEx struct {
	cbSize                  uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64 // in bytes
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

// VirtualMemory returns virtual memory metrics for the machine
func VirtualMemory() (*VirtualMemoryStat, error) {
	var memInfo memoryStatusEx
	memInfo.cbSize = uint32(unsafe.Sizeof(memInfo))
	mem, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if mem == 0 {
		return nil, windows.GetLastError()
	}

	ret := &VirtualMemoryStat{
		Total:       memInfo.ullTotalPhys,
		Available:   memInfo.ullAvailPhys,
		Used:        memInfo.ullTotalPhys - memInfo.ullAvailPhys,
		UsedPercent: float64(memInfo.dwMemoryLoad),
	}

	return ret, nil
}

// PagefileMemory returns paging (swap) file metrics
func PagefileMemory() (*PagefileStat, error) {
	var memInfo memoryStatusEx
	memInfo.cbSize = uint32(unsafe.Sizeof(memInfo))
	mem, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if mem == 0 {
		return nil, windows.GetLastError()
	}
	total := memInfo.ullTotalPageFile
	free := memInfo.ullAvailPageFile
	used := total - free
	percent := (float64(used) / float64(total)) * 100
	ret := &PagefileStat{
		Total:       total,
		Available:   free,
		Used:        used,
		UsedPercent: percent,
	}

	return ret, nil
}

// SwapMemory returns swapfile statistics
func SwapMemory() (*SwapMemoryStat, error) {
	var perfInfo performanceInformation
	perfInfo.cb = uint32(unsafe.Sizeof(perfInfo))
	mem, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&perfInfo)), uintptr(perfInfo.cb))
	if mem == 0 {
		return nil, windows.GetLastError()
	}
	tot := uint64(perfInfo.commitLimit * perfInfo.pageSize)
	used := uint64(perfInfo.commitTotal * perfInfo.pageSize)
	free := tot - used
	ret := &SwapMemoryStat{
		Total:       tot,
		Used:        used,
		Free:        free,
		UsedPercent: (float64(used) / float64(tot)) * 100,
	}

	return ret, nil
}
