// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

var (
	k32        = windows.NewLazyDLL("kernel32.dll")
	versiondll = windows.NewLazyDLL("version.dll")

	procGetModuleHandle          = k32.NewProc("GetModuleHandleW")
	procGetModuleFileName        = k32.NewProc("GetModuleFileNameW")
	procGetFileVersionInfoSizeEx = versiondll.NewProc("GetFileVersionInfoSizeExW")
	procGetFileVersionInfoEx     = versiondll.NewProc("GetFileVersionInfoExW")
	procVerQueryValue            = versiondll.NewProc("VerQueryValueW")
)

// ErrNoPEBuildTimestamp indicates the PE header timestamp is not present or zero.
var ErrNoPEBuildTimestamp = errors.New("no PE build timestamp")

// GetWindowsBuildString retrieves the windows build version by querying
// the resource string as directed here https://msdn.microsoft.com/en-us/library/windows/desktop/ms724429(v=vs.85).aspx
// as of Windows 8.1, the core GetVersion() APIs have been changed to
// return the version of Windows manifested with the application, not
// the application version
func GetWindowsBuildString() (verstring string, err error) {
	h, err := getModuleHandle("kernel32.dll")
	if err != nil {
		return
	}
	fullpath, err := getModuleFileName(h)
	if err != nil {
		return
	}
	data, err := getFileVersionInfo(fullpath)
	if err != nil {
		return
	}
	return getVersionInfo(data)
}

func getModuleHandle(fname string) (handle uintptr, err error) {
	file := windows.StringToUTF16Ptr(fname)
	handle, _, err = procGetModuleHandle.Call(uintptr(unsafe.Pointer(file)))
	if handle == 0 {
		return handle, err
	}
	return handle, nil
}

func getModuleFileName(h uintptr) (fname string, err error) {
	fname = ""
	err = nil
	var sizeIncr = uint32(1024)
	var size = sizeIncr
	for {
		buf := make([]uint16, size)
		ret, _, err := procGetModuleFileName.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(size))
		if ret == uintptr(size) || err == windows.ERROR_INSUFFICIENT_BUFFER {
			size += sizeIncr
			continue
		} else if err != nil {
			fname = windows.UTF16ToString(buf)
		}
		break
	}
	return

}

func getFileVersionInfo(filename string) (block []uint8, err error) {
	fname := windows.StringToUTF16Ptr(filename)
	ret, _, err := procGetFileVersionInfoSizeEx.Call(uintptr(0x02),
		uintptr(unsafe.Pointer(fname)), uintptr(0))
	if ret == 0 {
		return
	}
	size := uint32(ret)
	block = make([]uint8, size)
	ret, _, err = procGetFileVersionInfoEx.Call(uintptr(0x02),
		uintptr(unsafe.Pointer(fname)), uintptr(0), uintptr(size), uintptr(unsafe.Pointer(&block[0])))
	if ret == 0 {
		return nil, err
	}
	return block, nil

}

type tagVSFIXEDFILEINFO struct {
	dwSignature        uint32
	dwStrucVersion     uint32
	dwFileVersionMS    uint32
	dwFileVersionLS    uint32
	dwProductVersionMS uint32
	dwProductVersionLS uint32
	dwFileFlagsMask    uint32
	dwFileFlags        uint32
	dwFileOS           uint32
	dwFileType         uint32
	dwFileSubtype      uint32
	dwFileDateMS       uint32
	dwFileDateLS       uint32
}

func getVersionInfo(block []uint8) (ver string, err error) {

	subblock := windows.StringToUTF16Ptr("\\")
	var infoptr unsafe.Pointer
	var ulen uint32
	ret, _, err := procVerQueryValue.Call(uintptr(unsafe.Pointer(&block[0])),
		uintptr(unsafe.Pointer(subblock)),
		uintptr(unsafe.Pointer(&infoptr)),
		uintptr(unsafe.Pointer(&ulen)))
	if ret == 0 {
		return
	}
	ffi := (*tagVSFIXEDFILEINFO)(infoptr)
	ver = fmt.Sprintf("%d.%d Build %d", ffi.dwProductVersionMS>>16, ffi.dwProductVersionMS&0xFF, ffi.dwProductVersionLS>>16)

	return ver, nil

}

// FileVersionInfo contains common version resource strings for a file.
type FileVersionInfo struct {
	CompanyName      string
	ProductName      string
	FileVersion      string
	ProductVersion   string
	OriginalFilename string
	InternalName     string
}

// GetFileVersionInfoStrings returns common version resource strings for the specified file.
// Missing fields are returned as empty strings. An error is returned only if the version
// information block cannot be retrieved at all.
func GetFileVersionInfoStrings(executablePath string) (FileVersionInfo, error) {
	var info FileVersionInfo

	data, err := getFileVersionInfo(executablePath)
	if err != nil {
		return info, err
	}

	// Get the first language/codepage from the translation table
	translationPtr, err := syscall.UTF16PtrFromString("\\VarFileInfo\\Translation")
	if err != nil {
		return info, err
	}

	var langCodePagePtr *uint16
	var langCodePageLen uint32
	ret, _, err := procVerQueryValue.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(translationPtr)),
		uintptr(unsafe.Pointer(&langCodePagePtr)),
		uintptr(unsafe.Pointer(&langCodePageLen)),
	)
	if ret == 0 || langCodePageLen < 4 {
		return info, err
	}

	pair := (*[2]uint16)(unsafe.Pointer(langCodePagePtr))
	langCode := pair[0]
	codePage := pair[1]
	langCodePage := fmt.Sprintf("%04x%04x", langCode, codePage)

	// Helper to read a specific version string value
	readVerString := func(key string) string {
		query := fmt.Sprintf("\\StringFileInfo\\%s\\%s", langCodePage, key)
		queryPtr, qerr := syscall.UTF16PtrFromString(query)
		if qerr != nil {
			return ""
		}
		var valuePtr *uint16
		var valueLen uint32
		ret, _, _ := procVerQueryValue.Call(
			uintptr(unsafe.Pointer(&data[0])),
			uintptr(unsafe.Pointer(queryPtr)),
			uintptr(unsafe.Pointer(&valuePtr)),
			uintptr(unsafe.Pointer(&valueLen)),
		)
		if ret == 0 || valueLen == 0 || valuePtr == nil {
			return ""
		}
		return windows.UTF16PtrToString(valuePtr)
	}

	info.CompanyName = readVerString("CompanyName")
	info.ProductName = readVerString("ProductName")
	info.FileVersion = readVerString("FileVersion")
	info.ProductVersion = readVerString("ProductVersion")
	info.OriginalFilename = readVerString("OriginalFilename")
	info.InternalName = readVerString("InternalName")

	return info, nil
}
