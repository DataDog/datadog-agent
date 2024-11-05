// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	SystemProcessIdInformationClass = 88 // Querying process names without elevated privileges on Windows
)

// SYSTEM_PROCESS_ID_INFORMATION structure for Windows API.
type SYSTEM_PROCESS_ID_INFORMATION struct {
	ProcessId uintptr
	ImageName windows.NTUnicodeString
}

// RetrieveProcessName fetches the process name on Windows using NtQuerySystemInformation
// with SystemProcessIdInformation, which does not require elevated privileges.
func RetrieveProcessName(pid int, processName string) (string, error) {
	var processInfo SYSTEM_PROCESS_ID_INFORMATION
	processInfo.ProcessId = uintptr(pid)
	ret := windows.NtQuerySystemInformation(SystemProcessIdInformationClass, unsafe.Pointer(&processInfo), uint32(unsafe.Sizeof(processInfo)), nil)

	if ret != nil {
		return "", ret
	}
	return strings.TrimSuffix(windows.UTF16PtrToString((*uint16)(unsafe.Pointer(processInfo.ImageName.Buffer))), ".exe"), nil
}
