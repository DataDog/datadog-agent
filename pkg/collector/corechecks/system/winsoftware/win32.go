// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winsoftware

import (
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

var (
	msi                = windows.NewLazySystemDLL("msi.dll")
	msiGetProductInfoW = msi.NewProc("MsiGetProductInfoW")
	// For detecting per-user installations
	msiEnumProductsExW = msi.NewProc("MsiEnumProductsExW")
	advapi32           = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKey     = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKey   = advapi32.NewProc("RegUnLoadKeyW")
)

func msiEnumProducts(index uint32, productCodeBuf *uint16, context *uint32, sidBuf *uint16, sidLen *uint32) windows.Errno {
	ret, _, _ := msiEnumProductsExW.Call(
		0, // szProductCode = NULL
		0, // szUserSid = NULL
		uintptr(MSIINSTALLCONTEXT_ALL),
		uintptr(index),
		uintptr(unsafe.Pointer(productCodeBuf)),
		uintptr(unsafe.Pointer(context)),
		uintptr(unsafe.Pointer(sidBuf)),
		uintptr(unsafe.Pointer(sidLen)),
	)
	return windows.Errno(ret)
}

func msiGetProductInfo(propName string, productCode *uint16, buf *uint16, bufLen *uint32) windows.Errno {
	propNamePtr, err := syscall.UTF16FromString(propName)
	if err != nil {
		// EINVAL is the only error returned from UTF16FromString
		return syscall.EINVAL
	}
	ret, _, _ := msiGetProductInfoW.Call(
		uintptr(unsafe.Pointer(productCode)),
		uintptr(unsafe.Pointer(&propNamePtr[0])),
		uintptr(unsafe.Pointer(buf)),
		uintptr(unsafe.Pointer(bufLen)),
	)
	return windows.Errno(ret)
}

// regLoadKey loads a registry hive file into HKU\temp.
// This is a low-level function that calls RegLoadKey via syscall.
// Loading a hive requires special privileges (SE_RESTORE_NAME and SE_BACKUP_NAME)
func regLoadKey(hivePath string) windows.Errno {
	hivePathPtr, err := syscall.UTF16FromString(hivePath)
	if err != nil {
		// EINVAL is the only error returned from UTF16FromString
		return syscall.EINVAL
	}
	tempPtr, err := syscall.UTF16FromString("temp")
	if err != nil {
		// EINVAL is the only error returned from UTF16FromString
		return syscall.EINVAL
	}
	ret, _, _ := procRegLoadKey.Call(uintptr(syscall.HKEY_USERS), uintptr(unsafe.Pointer(&tempPtr[0])), uintptr(unsafe.Pointer(&hivePathPtr[0])))
	return windows.Errno(ret)
}

// regUnloadKey unloads the registry hive loaded at HKU\temp.
// This is a low-level function that calls RegUnLoadKey via syscall.
func regUnloadKey() windows.Errno {
	tempPtr, err := syscall.UTF16FromString("temp")
	if err != nil {
		// EINVAL is the only error returned from UTF16FromString
		return syscall.EINVAL
	}
	ret, _, _ := procRegUnLoadKey.Call(uintptr(syscall.HKEY_USERS), uintptr(unsafe.Pointer(&tempPtr[0])))
	return windows.Errno(ret)
}
