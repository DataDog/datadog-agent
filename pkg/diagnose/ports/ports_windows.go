// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	SystemProcessIDInformationClass = 88 // SystemProcessIDInformationClass gives access to process names without elevated privileges on Windows
)

// SystemProcessIDInformation is a struct for Windows API.
type SystemProcessIDInformation struct {
	ProcessID uintptr
	ImageName windows.NTUnicodeString
}

var NtQuerySystemInformation = windows.NtQuerySystemInformation

// RetrieveProcessName fetches the process name on Windows using NtQuerySystemInformation
// with SystemProcessIDInformation, which does not require elevated privileges.
func RetrieveProcessName(pid int, _ string) (string, error) {
	var processInfo SystemProcessIDInformation
	processInfo.ProcessID = uintptr(pid)
	ret := NtQuerySystemInformation(SystemProcessIDInformationClass, unsafe.Pointer(&processInfo), uint32(unsafe.Sizeof(processInfo)), nil)

	if ret != nil {
		return "", ret
	}

	// Step 1: Get a pointer to the buffer (which is a pointer to a wide string)
	bufferPtr := processInfo.ImageName.Buffer
	// Step 2: Convert this pointer to an unsafe.Pointer
	unsafePtr := unsafe.Pointer(bufferPtr)
	// Step 3: Convert that unsafe.Pointer to a *uint16
	utf16Ptr := (*uint16)(unsafePtr)
	// Step 4: Call windows.UTF16PtrToString on the *uint16 pointer to get a Go string
	rawName := windows.UTF16PtrToString(utf16Ptr)

	return FormatProcessName(rawName), nil
}
