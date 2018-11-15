// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package winutil

import (
	"C"
	"syscall"
	"unsafe"
)

// GUID is representation of the C GUID structure
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

/*
https://docs.microsoft.com/en-us/windows/desktop/shell/knownfolderid

FOLDERID_ProgramData
GUID	{62AB5D82-FDC1-4DC3-A9DD-070D1D495D97}
Display Name	ProgramData
Folder Type	FIXED
Default Path	%ALLUSERSPROFILE% (%ProgramData%, %SystemDrive%\ProgramData)
CSIDL Equivalent	CSIDL_COMMON_APPDATA
Legacy Display Name	Application Data
Legacy Default Path	%ALLUSERSPROFILE%\Application Data
*/
var (
	FOLDERIDProgramData = GUID{0x62AB5D82, 0xFDC1, 0x4DC3, [8]byte{0xA9, 0xDD, 0x07, 0x0D, 0x1D, 0x49, 0x5D, 0x97}}
)

var (
	modShell32               = syscall.NewLazyDLL("Shell32.dll")
	modOle32                 = syscall.NewLazyDLL("Ole32.dll")
	procSHGetKnownFolderPath = modShell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = modOle32.NewProc("CoTaskMemFree")
)

// SHGetKnownFolderPath syscall to windows native SHGetKnownFOlderPath
func SHGetKnownFolderPath(rfid *GUID, dwFlags uint32, hToken syscall.Handle, pszPath *uintptr) (retval error) {
	r0, _, _ := syscall.Syscall6(procSHGetKnownFolderPath.Addr(), 4, uintptr(unsafe.Pointer(rfid)), uintptr(dwFlags), uintptr(hToken), uintptr(unsafe.Pointer(pszPath)), 0, 0)
	if r0 != 0 {
		retval = syscall.Errno(r0)
	}
	return
}

// CoTaskMemFree free memory returned from SHGetKnownFolderPath
func CoTaskMemFree(pv uintptr) {
	syscall.Syscall(procCoTaskMemFree.Addr(), 1, uintptr(pv), 0, 0)
	return
}

// GetProgramDataDir returns the current programdatadir, usually
// c:\programdata
func GetProgramDataDir() (path string, err error) {
	var retstr *C.char
	var retstrptr = uintptr(unsafe.Pointer(retstr))
	err = SHGetKnownFolderPath(&FOLDERIDProgramData, 0, 0, &retstrptr)
	if err == nil {
		// convert the string
		defer CoTaskMemFree(retstrptr)
		// the path = syscall.UTF16ToString... returns a
		// go vet: "possible misuse of unsafe.Pointer"
		// Use the "C" GoString converter instead
		// path = syscall.UTF16ToString((*[1 << 16]uint16)(unsafe.Pointer(retstr))[:])
		path = C.GoString(retstr)
	}
	return
}
