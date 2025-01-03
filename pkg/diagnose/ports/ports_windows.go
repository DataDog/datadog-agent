// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// NTSTATUS is the return type used by many native Windows functions.
type NTSTATUS uint32

// ntSuccess is a helper function to check if a status is a success. 0 is success, all other values are failure.
func ntSuccess(status NTSTATUS) bool {
	return int32(status) >= 0
}

const (
	SystemProcessIDInformationClass = 88 // SystemProcessIDInformationClass gives access to process names without elevated privileges on Windows.
)

// SystemProcessIDInformation mirrors the SystemProcessIdInformation struct used by NtQuerySystemInformation.
type SystemProcessIDInformation struct {
	ProcessID uintptr
	ImageName winutil.UnicodeString
}

// Loading NtQuerySystemInformation
var (
	ntdll                        = syscall.NewLazyDLL("ntdll.dll")
	procNtQuerySystemInformation = ntdll.NewProc("NtQuerySystemInformation")
)

// NtQuerySystemInformation is an *undocumented* function prototype:
//
//	NTSTATUS NtQuerySystemInformation(
//	    SYSTEM_INFORMATION_CLASS SystemInformationClass,
//	    PVOID SystemInformation,
//	    ULONG SystemInformationLength,
//	    PULONG ReturnLength
//	);
func NtQuerySystemInformation(
	systemInformationClass uint32,
	systemInformation unsafe.Pointer,
	systemInformationLength uint32,
	returnLength *uint32,
) NTSTATUS {
	r0, _, _ := procNtQuerySystemInformation.Call(
		uintptr(systemInformationClass),
		uintptr(systemInformation),
		uintptr(systemInformationLength),
		uintptr(unsafe.Pointer(returnLength)),
	)
	return NTSTATUS(r0)
}

// RetrieveProcessName fetches the process name on Windows using NtQuerySystemInformation
// with SystemProcessIDInformationClass, which does not require elevated privileges.
func RetrieveProcessName(pid int, _ string) (string, error) {
	// Allocate a slice of 256 uint16s (512 bytes).
	// Used for unicodeString buffer.
	buf := make([]uint16, 256)

	// Prepare the SystemProcessIDInformation struct
	var info SystemProcessIDInformation
	info.ProcessID = uintptr(pid)
	info.ImageName.Length = 0
	info.ImageName.MaxLength = 256 * 2
	info.ImageName.Buffer = uintptr(unsafe.Pointer(&buf[0]))

	// Call NtQuerySystemInformation
	var returnLength uint32
	status := NtQuerySystemInformation(
		SystemProcessIDInformationClass,
		unsafe.Pointer(&info),
		uint32(unsafe.Sizeof(info)),
		&returnLength,
	)

	// If ntSuccess(status) is false, return an error and empty string
	if !ntSuccess(status) {
		return "", fmt.Errorf("NtQuerySystemInformation failed with NTSTATUS 0x%X", status)
	}

	// Convert imageName, which is a UnicodeString, to Go string
	imageName := winutil.UnicodeStringToString(info.ImageName)

	// Extract the base name of the process, remove .exe extension if present
	imageName = filepath.Base(imageName)
	imageName = strings.TrimSuffix(imageName, ".exe")

	return imageName, nil
}
