// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// msStoreEntry matches the MSStoreEntry struct from msstoreapps.h
type msStoreEntry struct {
	displayName *byte
	version     *byte
	installDate *byte
	is64bit     uint8
	_           [7]byte // padding to keep 8-byte alignment
	publisher   *byte
	productCode *byte
}

var (
	mod              = syscall.NewLazyDLL("MSStoreApps.dll")
	listStoreEntries = mod.NewProc("ListStoreEntries")
	freeStoreEntries = mod.NewProc("FreeStoreEntries")
)

// MSStoreAppsCollector implements Collector for Windows Store apps
type msStoreAppsCollector struct{}

// Collect retrieves Windows Store apps from MSStoreApps.dll
func (c *msStoreAppsCollector) Collect() ([]*Entry, []*Warning, error) {
	var entries []*Entry
	var warnings []*Warning
	var outArray *msStoreEntry
	var outCount int32

	r1, _, _ := listStoreEntries.Call(
		uintptr(unsafe.Pointer(&outArray)),
		uintptr(unsafe.Pointer(&outCount)),
	)
	if r1 != 0 {
		return nil, warnings, fmt.Errorf("ListStoreEntries failed with code %d", r1)
	}
	defer freeStoreEntries.Call(uintptr(unsafe.Pointer(outArray)), uintptr(outCount))

	raw := unsafe.Slice(outArray, int(outCount))
	for _, e := range raw {
		entries = append(entries, &Entry{
			DisplayName: windows.BytePtrToString(e.displayName),
			Version:     windows.BytePtrToString(e.version),
			InstallDate: windows.BytePtrToString(e.installDate),
			Source:      "msstore",
			UserSID:     "",
			Is64Bit:     e.is64bit == 1,
			Publisher:   windows.BytePtrToString(e.publisher),
			Status:      "installed",
			ProductCode: windows.BytePtrToString(e.productCode),
		})
	}

	return entries, warnings, nil
}
