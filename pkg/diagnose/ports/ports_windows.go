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
	SystemProcessIDInformationClass = 88 // SystemProcessIDInformationClass gives access to process names without elevated privileges on Windows
)

// SystemProcessIDInformation is a struct for Windows API.
type SystemProcessIDInformation struct {
	ProcessID uintptr
	ImageName windows.NTUnicodeString
}

var ntQuerySystemInformation = windows.NtQuerySystemInformation

// RetrieveProcessName fetches the process name on Windows using NtQuerySystemInformation
// with SystemProcessIDInformation, which does not require elevated privileges.
// RetrieveProcessName fetches the process name on Windows using NtQuerySystemInformation
// with SystemProcessIDInformation, which does not require elevated privileges.
func RetrieveProcessName(pid int, _ string) (string, error) {
	var processInfo SystemProcessIDInformation

	// Explicitly calculate the size of the buffer required for the process information.
	bufferSize := uint32(unsafe.Sizeof(processInfo))

	// Allocate memory for the buffer to hold the process information.
	buffer := make([]byte, bufferSize)

	// Cast the buffer pointer to the appropriate type.
	bufferPtr := unsafe.Pointer(&buffer[0])

	// Set the process ID in the buffer.
	processInfo.ProcessID = uintptr(pid)

	// Call NTQuerySystemInformation with the correct buffer size
	ret := ntQuerySystemInformation(SystemProcessIDInformationClass, bufferPtr, bufferSize, nil)
	if ret != nil {
		return "", ret
	}

	// Extract ImageName.Buffer from the buffer.
	// bufferStart := uintptr(bufferPtr)
	// imageNameBufferOffset := uintptr(unsafe.Offsetof(processInfo.ImageName.Buffer))
	// imageNameBuffer := (*uint16)(unsafe.Pointer(bufferStart + imageNameBufferOffset))

	// Safer version of L52-55
	imageNameBuffer := (*uint16)(unsafe.Pointer(uintptr(bufferPtr) + uintptr(unsafe.Offsetof(processInfo.ImageName.Buffer))))

	// Convert the Unicode string to a Go string.
	rawName := windows.UTF16PtrToString(imageNameBuffer)
	rawName = strings.ToLower(rawName)
	rawName = strings.TrimRight(rawName, "\x00")
	rawName = strings.TrimSuffix(rawName, ".exe")

	return rawName, nil
}
