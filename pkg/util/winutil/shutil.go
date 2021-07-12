// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build windows,!dovet

package winutil

import (
	"C"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)
import "path/filepath"

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
	// this is the GUID definition from shlobj.h
	//DEFINE_KNOWN_FOLDER(FOLDERID_ProgramData,         0x62AB5D82, 0xFDC1, 0x4DC3, 0xA9, 0xDD, 0x07, 0x0D, 0x1D, 0x49, 0x5D, 0x97);
	FOLDERIDProgramData = GUID{0x62AB5D82, 0xFDC1, 0x4DC3, [8]byte{0xA9, 0xDD, 0x07, 0x0D, 0x1D, 0x49, 0x5D, 0x97}}
)

var (
	modShell32               = windows.NewLazyDLL("Shell32.dll")
	modOle32                 = windows.NewLazyDLL("Ole32.dll")
	procSHGetKnownFolderPath = modShell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = modOle32.NewProc("CoTaskMemFree")
)

// SHGetKnownFolderPath syscall to windows native SHGetKnownFOlderPath
func SHGetKnownFolderPath(rfid *GUID, dwFlags uint32, hToken windows.Handle, pszPath *uintptr) (retval error) {
	r0, _, _ := procSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(rfid)), uintptr(dwFlags), uintptr(hToken), uintptr(unsafe.Pointer(pszPath)), 0, 0)
	if r0 != 0 {
		retval = syscall.Errno(r0)
	}
	return
}

// CoTaskMemFree free memory returned from SHGetKnownFolderPath
func CoTaskMemFree(pv uintptr) {
	procCoTaskMemFree.Call(uintptr(pv), 0, 0)
	return
}

func getDefaultProgramDataDir() (path string, err error) {
	var retstr uintptr
	err = SHGetKnownFolderPath(&FOLDERIDProgramData, 0, 0, &retstr)
	if err == nil {
		// convert the string
		defer CoTaskMemFree(retstr)
		// the path = windows.UTF16ToString... returns a
		// go vet: "possible misuse of unsafe.Pointer"
		path = windows.UTF16ToString((*[1 << 16]uint16)(unsafe.Pointer(retstr))[:])
		path = filepath.Join(path, "Datadog")
	}
	return
}

// GetProgramDataDir returns the current programdatadir, usually
// c:\programdata\Datadog
func GetProgramDataDir() (path string, err error) {
	return GetProgramDataDirForProduct("Datadog Agent")
}

// GetProgramDataDirForProduct returns the current programdatadir, usually
// c:\programdata\Datadog given a product key name
func GetProgramDataDirForProduct(product string) (path string, err error) {
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", keyname)
		return getDefaultProgramDataDir()
	}
	defer k.Close()
	val, _, err := k.GetStringValue("ConfigRoot")
	if err != nil {
		log.Warnf("Windows installation key config not found, using default program data dir")
		return getDefaultProgramDataDir()
	}
	path = val
	return
}
