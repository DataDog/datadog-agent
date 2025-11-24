// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowsnative


import (
	"fmt"
	"syscall"
	"unsafe"
)

// Must mirror C++ structs exactly (64-bit build assumed).

// C++:
// typedef struct MSStoreEntry {
//     const wchar_t* display_name;
//     uint16_t version_major;
//     uint16_t version_minor;
//     uint16_t version_build;
//     uint16_t version_revision;
//     int64_t  install_date;
//     uint8_t  is_64bit;
//     const wchar_t* publisher;
//     const wchar_t* product_code;
// };
//
// Layout (MSVC, x64):
//  0:  wchar_t* display_name   (8 bytes)
//  8:  uint16_t version_major  (2)
// 10:  uint16_t version_minor  (2)
// 12:  uint16_t version_build  (2)
// 14:  uint16_t version_revision (2)   -> now at offset 16
// 16:  int64_t  install_date   (8)     -> now at offset 24
// 24:  uint8_t  is_64bit       (1)
// 25:  padding (7)             -> to align next pointer at 32
// 32:  wchar_t* publisher      (8)
// 40:  wchar_t* product_code   (8)
// sizeof = 48

type cStoreEntry struct {
	displayName     *uint16
	VersionMajor    uint16
	VersionMinor    uint16
	VersionBuild    uint16
	VersionRevision uint16
	InstallDate     int64
	Is64Bit         uint8
	_               [7]byte // padding to align next pointer to 8 bytes
	Publisher       *uint16
	ProductCode     *uint16
}

// C++:
// typedef struct MSStore {
//     int32_t       count;
//     MSStoreEntry* entries;
// };
//
// Layout (x64):
//  0: int32_t count  (4)
//  4: padding (4)    -> to align pointer
//  8: MSStoreEntry* entries (8)
// sizeof = 16

type cStore struct {
	Count   int32
	_       [4]byte // padding
	Entries *cStoreEntry
}

var (
	mod      = syscall.NewLazyDLL("WindowsNative.dll")
	procList = mod.NewProc("MSStore_ListEntries")
	procFree = mod.NewProc("MSStore_FreeEntries")
)

func MSStore_ListEntries() (*cStore, error) {
	var out *cStore
	r1, _, _ := procList.Call(uintptr(unsafe.Pointer(&out)))
	if r1 != 0 {
		return nil, fmt.Errorf("MSStore_ListEntries: %d", r1)
	}
	return out, nil
}

func MSStore_FreeEntries(store *cStore) {
	procFree.Call(uintptr(unsafe.Pointer(store)))
}