// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package winutil

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	Modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	ModPsapi    = syscall.NewLazyDLL("psapi.dll")

	procGlobalMemoryStatusEx = Modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetPerformanceInfo   = ModPsapi.NewProc("GetPerformanceInfo")
)

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
		UsedPercent: float64(memInfo.dwMemoryLoad),
	}

	ret.Used = ret.Total - ret.Available
	return ret, nil
}

type performanceInformation struct {
	cb                uint32
	commitTotal       uint64
	commitLimit       uint64
	commitPeak        uint64
	physicalTotal     uint64
	physicalAvailable uint64
	systemCache       uint64
	kernelTotal       uint64
	kernelPaged       uint64
	kernelNonpaged    uint64
	pageSize          uint64
	handleCount       uint32
	processCount      uint32
	threadCount       uint32
}

func SwapMemory() (*SwapMemoryStat, error) {
	var perfInfo performanceInformation
	perfInfo.cb = uint32(unsafe.Sizeof(perfInfo))
	mem, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&perfInfo)), uintptr(perfInfo.cb))
	if mem == 0 {
		return nil, windows.GetLastError()
	}
	tot := perfInfo.commitLimit * perfInfo.pageSize
	used := perfInfo.commitTotal * perfInfo.pageSize
	free := tot - used
	ret := &SwapMemoryStat{
		Total:       tot,
		Used:        used,
		Free:        free,
		UsedPercent: float64(used / tot),
	}

	return ret, nil
}
