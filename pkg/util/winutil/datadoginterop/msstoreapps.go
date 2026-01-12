// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package msstoreapps provides the API for MS Store apps from the Datadog Interop DLL, libdatadog-interop.dll.
package msstoreapps

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Must match the MSStoreEntry struct in msstoreapps.h
type cStoreEntry struct {
	DisplayName     *uint16
	VersionMajor    uint16
	VersionMinor    uint16
	VersionBuild    uint16
	VersionRevision uint16
	InstallDate     int64
	Is64Bit         uint64
	Publisher       *uint16
	ProductCode     *uint16
}

// Must match the MSStore struct in msstoreapps.h
type CStore struct {
	Count   int64
	Entries *cStoreEntry
}

var (
	mod           = syscall.NewLazyDLL("libdatadog-interop.dll")
	procGetStore  = mod.NewProc("GetStore")
	procFreeStore = mod.NewProc("FreeStore")
)

// GetStore returns a pointer to a CStore struct containing the list of MS Store apps.
// The caller is responsible for freeing the memory allocated for the CStore struct using FreeStore.
func GetStore() (*CStore, error) {
	var out *CStore
	r1, _, lastErr := procGetStore.Call(uintptr(unsafe.Pointer(&out)))
	if r1 == 0 {
		return nil, fmt.Errorf("GetStore failed: %w", lastErr)
	}
	return out, nil
}

// FreeStore frees the memory allocated for the CStore struct.
// Returns an error if the free operation fails.
func FreeStore(store *CStore) error {
	r1, _, lastErr := procFreeStore.Call(uintptr(unsafe.Pointer(store)))
	if r1 == 0 {
		return fmt.Errorf("FreeStore failed: %w", lastErr)
	}
	return nil
}
