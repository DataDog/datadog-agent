// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package filesystem

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Disk gets information about the disk
type Disk struct {
	procGetDiskFreeSpaceExW *windows.LazyProc
}

// NewDisk creates a new instance of Disk
func NewDisk() Disk {
	modkernel32 := windows.NewLazyDLL("kernel32.dll")
	return Disk{
		procGetDiskFreeSpaceExW: modkernel32.NewProc("GetDiskFreeSpaceExW"),
	}
}

// GetUsage gets the disk usage
func (d Disk) GetUsage(path string) (*DiskUsage, error) {
	free := uint64(0)
	total := uint64(0)

	winPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	ret, _, err := d.procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(winPath)),
		uintptr(unsafe.Pointer(&free)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(nil)),
	)

	if ret == 0 {
		return nil, err
	}

	return &DiskUsage{
		Total:     total,
		Available: free,
	}, nil
}
