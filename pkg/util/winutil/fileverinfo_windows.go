// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modVersion                   = windows.NewLazyDLL("version.dll")
	procGetFileVersionInfoSizeW  = modVersion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW      = modVersion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW           = modVersion.NewProc("VerQueryValueW")
)

// FileVersionInfo contains common version resource strings for a file.
type FileVersionInfo struct {
	CompanyName     string
	ProductName     string
	FileVersion     string
	ProductVersion  string
	OriginalFilename string
	InternalName    string
}

// GetFileVersionInfoStrings returns common version resource strings for the specified file.
// Missing fields are returned as empty strings. An error is returned only if the version
// information block cannot be retrieved at all.
func GetFileVersionInfoStrings(executablePath string) (FileVersionInfo, error) {
	var info FileVersionInfo

	pathPtr, err := syscall.UTF16PtrFromString(executablePath)
	if err != nil {
		return info, fmt.Errorf("invalid path: %w", err)
	}

	var handle uint32
	size, _, err := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&handle)),
	)
	if size == 0 {
		if err != windows.ERROR_SUCCESS {
			return info, fmt.Errorf("GetFileVersionInfoSizeW failed: %w", err)
		}
		return info, fmt.Errorf("no version information available for %s", executablePath)
	}

	data := make([]byte, size)
	ret, _, err := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(handle),
		uintptr(size),
		uintptr(unsafe.Pointer(&data[0])),
	)
	if ret == 0 {
		return info, fmt.Errorf("GetFileVersionInfoW failed: %w", err)
	}

	// Get the first language/codepage from the translation table
	translationPtr, err := syscall.UTF16PtrFromString("\\VarFileInfo\\Translation")
	if err != nil {
		return info, fmt.Errorf("failed to create translation query: %w", err)
	}

	var langCodePagePtr *uint16
	var langCodePageLen uint32
	ret, _, err = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(translationPtr)),
		uintptr(unsafe.Pointer(&langCodePagePtr)),
		uintptr(unsafe.Pointer(&langCodePageLen)),
	)
	if ret == 0 || langCodePageLen < 4 {
		return info, fmt.Errorf("no language code page found: %w", err)
	}

	pair := (*[2]uint16)(unsafe.Pointer(langCodePagePtr))
	langCode := pair[0]
	codePage := pair[1]
	langCodePage := fmt.Sprintf("%04x%04x", langCode, codePage)

	// Helper to read a specific string value
	readString := func(key string) string {
		query := fmt.Sprintf("\\StringFileInfo\\%s\\%s", langCodePage, key)
		queryPtr, qerr := syscall.UTF16PtrFromString(query)
		if qerr != nil {
			return ""
		}
		var valuePtr *uint16
		var valueLen uint32
		ret, _, _ := procVerQueryValueW.Call(
			uintptr(unsafe.Pointer(&data[0])),
			uintptr(unsafe.Pointer(queryPtr)),
			uintptr(unsafe.Pointer(&valuePtr)),
			uintptr(unsafe.Pointer(&valueLen)),
		)
		if ret == 0 || valueLen == 0 || valuePtr == nil {
			return ""
		}
		return windows.UTF16PtrToString(valuePtr)
	}

	info.CompanyName = readString("CompanyName")
	info.ProductName = readString("ProductName")
	info.FileVersion = readString("FileVersion")
	info.ProductVersion = readString("ProductVersion")
	info.OriginalFilename = readString("OriginalFilename")
	info.InternalName = readString("InternalName")

	return info, nil
}


