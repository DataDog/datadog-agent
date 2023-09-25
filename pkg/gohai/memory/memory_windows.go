// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// MEMORYSTATUSEX is the type of the struct expected by GlobalMemoryStatusEx
// https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/ns-sysinfoapi-memorystatusex
//
//nolint:revive
type MEMORYSTATUSEX struct {
	dwLength               uint32 // size of this structure
	dwMemoryLoad           uint32 // number 0-100 estimating %age of memory in use
	ulTotalPhys            uint64 // amount of physical memory
	ulAvailPhys            uint64 // amount of physical memory that can be used w/o flush to disk
	ulTotalPageFile        uint64 // current commit limit for system or process
	ulAvailPageFile        uint64 // amount of memory current process can commit
	ulTotalVirtual         uint64 // size of user-mode portion of VA space
	ulAvailVirtual         uint64 // amount of unreserved/uncommitted memory in ulTotalVirtual
	ulAvailExtendedVirtual uint64 // reserved (always zero)
}

func (info *Info) fillMemoryInfo() {
	var mod = windows.NewLazyDLL("kernel32.dll")
	var getMem = mod.NewProc("GlobalMemoryStatusEx")

	var memStruct MEMORYSTATUSEX
	memStruct.dwLength = uint32(unsafe.Sizeof(memStruct))

	status, _, err := getMem.Call(uintptr(unsafe.Pointer(&memStruct)))
	if status != 0 {
		info.TotalBytes = utils.NewValue(memStruct.ulTotalPhys)
	} else {
		info.TotalBytes = utils.NewErrorValue[uint64](err)
	}

	info.SwapTotalKb = utils.NewErrorValue[uint64](fmt.Errorf("memory.SwapTotalKb: %w", utils.ErrNotCollectable))
}
