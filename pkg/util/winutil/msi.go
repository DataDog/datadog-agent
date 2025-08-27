// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	msi                           = windows.NewLazySystemDLL("msi.dll")
	procMsiSourceListAddSourceExW = msi.NewProc("MsiSourceListAddSourceExW")
	procMsiSourceListSetInfoW     = msi.NewProc("MsiSourceListSetInfoW")
	procMsiSourceListForceResExW  = msi.NewProc("MsiSourceListForceResolutionExW")
)

// MsiSourceListAddSourceEx adds or reorders the set of sources of a patch or product in a specified context.
// It can also create a source list for a patch that does not exist in the specified context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistaddsourceexw
func MsiSourceListAddSourceEx(productCode string, context uint32, sourceType uint32, src string, index uint32) windows.Errno {
	r1, _, _ := procMsiSourceListAddSourceExW.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(productCode))), // szProductCodeOrPatchCode
		0,                   // szUserSid (NULL for machine/current user)
		uintptr(context),    // MSIINSTALLCONTEXT
		uintptr(sourceType), // dwOptions
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(src))), // szSource (path only; no filename)
		uintptr(index), // dwIndex (1..N; 0 = append)
	)
	return windows.Errno(r1)
}

// MsiSourceListSetInfo sets information about the source list for a product or patch in a specific context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistsetinfow
func MsiSourceListSetInfo(productCode string, context uint32, options uint32, propName string, value string) windows.Errno {
	r1, _, _ := procMsiSourceListSetInfoW.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(productCode))), // szProductCodeOrPatchCode
		0, // szUserSid
		uintptr(context),
		uintptr(options), // dwOptions (MSICODE_*)
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(propName))), // szProperty (e.g., "PackageName")
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(value))),    // szValue
	)
	return windows.Errno(r1)
}

// MsiSourceListForceResolutionEx removes the registration of the property called "LastUsedSource".
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistforceresolutionexw
func MsiSourceListForceResolutionEx(productCode string, context uint32, options uint32) windows.Errno {
	r1, _, _ := procMsiSourceListForceResExW.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(productCode))), // szProductCodeOrPatchCode
		0, // szUserSid
		uintptr(context),
		uintptr(options), // dwOptions
	)
	return windows.Errno(r1)
}
