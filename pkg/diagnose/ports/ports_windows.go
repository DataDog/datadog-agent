// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import (
	"fmt"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// -----------------------------------------------------------------------
// 1. Definitions & Structures
// -----------------------------------------------------------------------
// NTSTATUS is the return type used by many native Windows functions.
type NTSTATUS uint32

// NT_SUCCESS is a helper function to check if a status is a success. 0 is success, all other values are failure.
func NT_SUCCESS(status NTSTATUS) bool {
	return status >= 0
}

const (
	SystemProcessIDInformation = 88 // SystemProcessIDInformation gives access to process names without elevated privileges on Windows.
)

// UNICODE_STRING mirrors the Windows UNICODE_STRING struct.
type UNICODE_STRING struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// SYSTEM_PROCESS_ID_INFORMATION mirrors the SystemProcessIdInformation struct used by NtQuerySystemInformation.
type SYSTEM_PROCESS_ID_INFORMATION struct {
	ProcessID uintptr
	ImageName UNICODE_STRING
}

// -----------------------------------------------------------------------
// 2. Loading NtQuerySystemInformation
// -----------------------------------------------------------------------
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

// -----------------------------------------------------------------------
// 3. Helper function to convert a UNICODE_STRING to a Go string
// -----------------------------------------------------------------------
func unicodeStringToString(u UNICODE_STRING) string {
	// Length is in bytes; divide by 2 for number of uint16 chars
	length := int(u.Length / 2)
	if length == 0 || u.Buffer == nil {
		return ""
	}
	// Convert from a pointer to a slice of uint16
	buf := (*[1 << 20]uint16)(unsafe.Pointer(u.Buffer))[:length:length]
	// Convert UTF-16 to Go string
	return string(utf16.Decode(buf))
}

// RetrieveProcessName fetches the process name on Windows using NtQuerySystemInformation
// with SystemProcessIDInformation, which does not require elevated privileges.
func RetrieveProcessName(pid int, _ string) (string, error) {
	// Allocate a slice of 256 uint16s (512 bytes).
	// Used for UNICODE_STRING buffer.
	buf := make([]uint16, 256)

	// Prepare the SYSTEM_PROCESS_ID_INFORMATION struct
	var info SYSTEM_PROCESS_ID_INFORMATION
	info.ProcessID = uintptr(pid)
	info.ImageName.Length = 0
	info.ImageName.MaximumLength = 256 * 2
	info.ImageName.Buffer = &buf[0]

	// Call NtQuerySystemInformation
	var returnLength uint32
	status := NtQuerySystemInformation(
		SystemProcessIDInformation,
		unsafe.Pointer(&info),
		uint32(unsafe.Sizeof(info)),
		&returnLength,
	)

	// If NT_SUCCESS(status) is false, return an error and empty string
	if !NT_SUCCESS(status) {
		return "", fmt.Errorf("NtQuerySystemInformation failed with NTSTATUS 0x%X", status)
	}

	// Convert UNICODE_STRING to Go string
	imageName := unicodeStringToString(info.ImageName)
	rawName := strings.TrimSuffix(imageName, ".exe")

	return rawName, nil
}
