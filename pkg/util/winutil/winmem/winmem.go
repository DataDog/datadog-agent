// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winmem provides memory usage metrics for Windows
package winmem

/*
#cgo LDFLAGS: -lpsapi

#include "winmem.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"runtime/cgo"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	modkernel32 = windows.NewLazyDLL("kernel32.dll")
	modPsapi    = windows.NewLazyDLL("psapi.dll")

	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetPerformanceInfo   = modPsapi.NewProc("GetPerformanceInfo")
	procEnumPageFilesW       = modPsapi.NewProc("EnumPageFilesW")
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

// PagingFileStat contains statistics for paging files
type PagingFileStat struct {
	// The name of the paging file
	Name string

	// The total size of the paging file
	Total uint64

	// The amount of paging file that is available for use
	Available uint64

	// The amount of paging file that is used
	Used uint64

	// The percentage of paging file that is used
	UsedPercent float64
}

// PageFilesContext is a context for the EnumPageFilesW function
type PageFilesContext struct {
	PageFiles []*PagingFileStat
	PageSize  uint64
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

// enumPageFileInformation contains information about a pagefile.
//
// https://learn.microsoft.com/en-us/windows/win32/api/psapi/ns-psapi-enum_page_file_information
type enumPageFileInformation struct {
	cbSize     uint32 //nolint:unused // Required for Windows API structure layout
	reserved   uint32 //nolint:unused // Required for Windows API structure layout
	totalSize  uint64
	totalInUse uint64
	peakUsage  uint64 //nolint:unused // Required for Windows API structure layout
}

// VirtualMemory returns virtual memory metrics for the machine
func VirtualMemory() (*VirtualMemoryStat, error) {
	var memInfo memoryStatusEx
	memInfo.cbSize = uint32(unsafe.Sizeof(memInfo))
	mem, _, e := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if mem == 0 {
		return nil, e
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
	mem, _, e := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if mem == 0 {
		return nil, e
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
	perfInfo, err := getPerformanceInfo()
	if err != nil {
		return nil, err
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

//export PageFileCallback
func PageFileCallback(
	handle C.GoHandle,
	pInfo C.PENUM_PAGE_FILE_INFORMATION,
	lpFilename C.LPCWSTR,
) C.BOOL {
	if pInfo == nil || lpFilename == nil {
		log.Errorf("Invalid input in callback: pInfo: %v, lpFilename: %v", pInfo, lpFilename)
		return C.BOOL(1)
	}

	// Convert the handle back to a Go context
	h := cgo.Handle(handle)
	value := h.Value()
	ctx, ok := value.(*PageFilesContext)
	if !ok {
		log.Errorf("Invalid context type in callback: %v", value)
		return C.BOOL(0)
	}

	pageFileName := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(lpFilename)))
	if pageFileName == "" {
		log.Errorf("Invalid page file name in callback: %v", lpFilename)
		return C.BOOL(1)
	}
	log.Debugf("Found page file: %v", pageFileName)

	// Calculate metrics in bytes with overflow protection
	// Check for potential overflow before multiplication
	totalSize := uint64(pInfo.TotalSize)
	totalInUse := uint64(pInfo.TotalInUse)
	log.Debugf("Total size: %v, Total in use: %v", totalSize, totalInUse)

	// Check for potential overflow before multiplication
	if ctx.PageSize > 0 && totalSize > (^uint64(0)/ctx.PageSize) {
		log.Debugf("Total size is too large for page file: %v", pageFileName)
		return C.BOOL(1)
	}

	totalBytes := totalSize * ctx.PageSize
	usedBytes := totalInUse * ctx.PageSize
	availableBytes := totalBytes - usedBytes
	log.Debugf("Total bytes: %v, Used bytes: %v, Available bytes: %v", totalBytes, usedBytes, availableBytes)

	var usedPercent float64
	if totalBytes != 0 {
		usedPercent = (float64(usedBytes) / float64(totalBytes)) * 100.0
	} else {
		log.Warnf("Total bytes is 0 for page file: %v", pageFileName)
	}

	// Create page file entry
	pageFile := &PagingFileStat{
		Name:        pageFileName,
		Total:       totalBytes,
		Available:   availableBytes,
		Used:        usedBytes,
		UsedPercent: usedPercent,
	}
	log.Debugf("Created page file entry: %v", pageFile)

	ctx.PageFiles = append(ctx.PageFiles, pageFile)
	log.Debugf("Added page file entry to context: %v", ctx.PageFiles)

	return C.BOOL(1)
}

// EnumPageFilesW enumerates page files
//
// https://learn.microsoft.com/en-us/windows/win32/api/psapi/nf-psapi-enumpagefilesw
func EnumPageFilesW(pageSize uint64) ([]*PagingFileStat, error) {
	ctx := &PageFilesContext{
		PageFiles: make([]*PagingFileStat, 0),
		PageSize:  pageSize,
	}

	handle := cgo.NewHandle(ctx)
	defer handle.Delete()

	ret := windows.Errno(C.enumPageFilesWithHandle(C.GoHandle(handle)))
	if ret != windows.ERROR_SUCCESS {
		return nil, fmt.Errorf("failed to enumerate Page Files: %w", ret)
	}

	return ctx.PageFiles, nil
}

// PagingFileMemory returns paging file metrics
func PagingFileMemory() ([]*PagingFileStat, error) {
	perfInfo, err := getPerformanceInfo()
	if err != nil {
		return nil, err
	}
	pageSize := perfInfo.pageSize
	pageFiles, err := EnumPageFilesW(pageSize)
	if err != nil {
		return nil, err
	}
	return pageFiles, nil
}

func getPerformanceInfo() (*performanceInformation, error) {
	var perfInfo performanceInformation
	perfInfo.cb = uint32(unsafe.Sizeof(perfInfo))
	mem, _, err := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&perfInfo)), uintptr(perfInfo.cb))
	if mem == 0 {
		return nil, err
	}
	return &perfInfo, nil
}
