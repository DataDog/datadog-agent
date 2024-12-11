// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
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
