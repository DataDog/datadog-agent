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
	"unicode/utf16"
	"unsafe"
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

// unicodeString mirrors the Windows unicodeString struct.
type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// SystemProcessIDInformation mirrors the SystemProcessIdInformation struct used by NtQuerySystemInformation.
type SystemProcessIDInformation struct {
	ProcessID uintptr
	ImageName unicodeString
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

// unicodeStringToString is a helper function to convert a unicodeString to a Go string
func unicodeStringToString(u unicodeString) string {
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
// with SystemProcessIDInformationClass, which does not require elevated privileges.
func RetrieveProcessName(pid int, _ string) (string, error) {
	// Allocate a slice of 256 uint16s (512 bytes).
	// Used for unicodeString buffer.
	buf := make([]uint16, 256)

	// Prepare the SystemProcessIDInformation struct
	var info SystemProcessIDInformation
	info.ProcessID = uintptr(pid)
	info.ImageName.Length = 0
	info.ImageName.MaximumLength = 256 * 2
	info.ImageName.Buffer = &buf[0]

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

	// Convert unicodeString to Go string
	imageName := unicodeStringToString(info.ImageName)

	// Extract the base name of the process, remove .exe extension if present
	imageName = filepath.Base(imageName)
	imageName = strings.TrimSuffix(imageName, ".exe")

	return imageName, nil
}
