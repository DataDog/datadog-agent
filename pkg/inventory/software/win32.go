// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKey   = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKey = advapi32.NewProc("RegUnLoadKeyW")
)

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
