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
	procMsiEnumProductsExW        = msi.NewProc("MsiEnumProductsExW")
	procMsiGetProductInfoW        = msi.NewProc("MsiGetProductInfoW")
)

// MsiSourceListAddSourceEx adds or reorders the set of sources of a patch or product in a specified context.
// It can also create a source list for a patch that does not exist in the specified context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistaddsourceexw
func MsiSourceListAddSourceEx(productCode *uint16, context uint32, sourceType uint32, src *uint16, index uint32) windows.Errno {
	r1, _, _ := procMsiSourceListAddSourceExW.Call(
		uintptr(unsafe.Pointer(productCode)), // szProductCodeOrPatchCode
		0,                                    // szUserSid (NULL for machine/current user)
		uintptr(context),                     // MSIINSTALLCONTEXT
		uintptr(sourceType),                  // dwOptions
		uintptr(unsafe.Pointer(src)),         // szSource (path only; no filename)
		uintptr(index),                       // dwIndex (1..N; 0 = append)
	)
	return windows.Errno(r1)
}

// MsiSourceListSetInfo sets information about the source list for a product or patch in a specific context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistsetinfow
func MsiSourceListSetInfo(productCode *uint16, context uint32, options uint32, propName *uint16, value *uint16) windows.Errno {
	r1, _, _ := procMsiSourceListSetInfoW.Call(
		uintptr(unsafe.Pointer(productCode)), // szProductCodeOrPatchCode
		0,                                    // szUserSid
		uintptr(context),
		uintptr(options),                  // dwOptions (MSICODE_*)
		uintptr(unsafe.Pointer(propName)), // szProperty (e.g., "PackageName")
		uintptr(unsafe.Pointer(value)),    // szValue
	)
	return windows.Errno(r1)
}

// MsiSourceListForceResolutionEx removes the registration of the property called "LastUsedSource".
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistforceresolutionexw
func MsiSourceListForceResolutionEx(productCode *uint16, context uint32, options uint32) windows.Errno {
	r1, _, _ := procMsiSourceListForceResExW.Call(
		uintptr(unsafe.Pointer(productCode)), // szProductCodeOrPatchCode
		0,                                    // szUserSid
		uintptr(context),
		uintptr(options), // dwOptions
	)
	return windows.Errno(r1)
}

// MsiEnumProductsEx  enumerates through one or all the instances of products that are
// currently advertised or installed in the specified contexts.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msienumproductsexw
func MsiEnumProductsEx(szProductCode *uint16, szUserSid *uint16, dwContext uint32, index uint32, productCodeBuf *uint16, pdwInstalledContext *uint32, sidBuf *uint16, sidLen *uint32) windows.Errno {
	ret, _, _ := procMsiEnumProductsExW.Call(
		uintptr(unsafe.Pointer(szProductCode)), // szProductCode = NULL
		uintptr(unsafe.Pointer(szUserSid)),     // szUserSid = NULL
		uintptr(dwContext),
		uintptr(index),
		uintptr(unsafe.Pointer(productCodeBuf)),
		uintptr(unsafe.Pointer(pdwInstalledContext)), // context
		uintptr(unsafe.Pointer(sidBuf)),
		uintptr(unsafe.Pointer(sidLen)),
	)
	return windows.Errno(ret)
}

// MsiGetProductInfo returns product information for advertised and installed products.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msigetproductinfoexw
func MsiGetProductInfo(productCode *uint16, propName *uint16, buf *uint16, bufLen *uint32) windows.Errno {
	ret, _, _ := procMsiGetProductInfoW.Call(
		uintptr(unsafe.Pointer(productCode)),
		uintptr(unsafe.Pointer(propName)),
		uintptr(unsafe.Pointer(buf)),
		uintptr(unsafe.Pointer(bufLen)),
	)
	return windows.Errno(ret)
}
