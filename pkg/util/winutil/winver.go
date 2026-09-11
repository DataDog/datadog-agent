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
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	k32        = windows.NewLazyDLL("kernel32.dll")
	versiondll = windows.NewLazyDLL("version.dll")
	shlwapi    = windows.NewLazyDLL("shlwapi.dll")

	procGetModuleHandle          = k32.NewProc("GetModuleHandleW")
	procGetModuleFileName        = k32.NewProc("GetModuleFileNameW")
	procGetFileVersionInfoSizeEx = versiondll.NewProc("GetFileVersionInfoSizeExW")
	procGetFileVersionInfoEx     = versiondll.NewProc("GetFileVersionInfoExW")
	procVerQueryValue            = versiondll.NewProc("VerQueryValueW")
	procSHLoadIndirectString     = shlwapi.NewProc("SHLoadIndirectString")
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

// windowsCurrentVersionKey records the version of the running Windows installation.
const windowsCurrentVersionKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`

// GetWindowsVersion returns the full version of the running Windows installation,
// formatted as "major.minor.build.revision" — for example "10.0.26100.4652".
//
// This is the version winver.exe reports ("OS Build 26100.4652"). The revision, which
// the cumulative update advances, is recorded only here: RtlGetVersion's OSVERSIONINFOEX
// has no field for it, and GetWindowsBuildString reads kernel32.dll's version resource,
// which carries that file's revision rather than the system's.
func GetWindowsVersion() (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, windowsCurrentVersionKey, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", windowsCurrentVersionKey, err)
	}
	defer func() { _ = key.Close() }()

	major, _, err := key.GetIntegerValue("CurrentMajorVersionNumber")
	if err != nil {
		return "", fmt.Errorf("failed to read CurrentMajorVersionNumber: %w", err)
	}

	minor, _, err := key.GetIntegerValue("CurrentMinorVersionNumber")
	if err != nil {
		return "", fmt.Errorf("failed to read CurrentMinorVersionNumber: %w", err)
	}

	// CurrentBuildNumber is a string value, and is used as-is: it is the build number
	// users recognise, and reformatting it could only lose information.
	build, _, err := key.GetStringValue("CurrentBuildNumber")
	if err != nil {
		return "", fmt.Errorf("failed to read CurrentBuildNumber: %w", err)
	}

	// An absent UBR is reported as revision zero rather than as an error: the rest of the
	// version is still correct, and an installation that has had no cumulative update
	// applied does not necessarily carry the value. Any other failure leaves the revision
	// unknown, and reporting it as zero would look downstream like a version change.
	revision, _, err := key.GetIntegerValue("UBR")
	if err != nil {
		if !errors.Is(err, registry.ErrNotExist) {
			return "", fmt.Errorf("failed to read UBR: %w", err)
		}
		revision = 0
	}

	return fmt.Sprintf("%d.%d.%s.%d", major, minor, build, revision), nil
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

// vsFixedFileInfoSignature is the dwSignature value every VS_FIXEDFILEINFO carries.
const vsFixedFileInfoSignature = 0xFEEF04BD

// queryFixedFileInfo returns the VS_FIXEDFILEINFO at the root of a version resource block.
//
// The returned length and signature are both validated: VerQueryValueW can succeed on an
// existing root block while returning a value shorter than the structure, and the version
// resource comes from an arbitrary binary on the host, so dereferencing it unchecked would
// read past the block.
func queryFixedFileInfo(block []uint8) (*tagVSFIXEDFILEINFO, error) {
	if len(block) == 0 {
		return nil, errors.New("empty version information block")
	}

	subblock := windows.StringToUTF16Ptr("\\")
	var infoptr unsafe.Pointer
	var ulen uint32
	ret, _, err := procVerQueryValue.Call(uintptr(unsafe.Pointer(&block[0])),
		uintptr(unsafe.Pointer(subblock)),
		uintptr(unsafe.Pointer(&infoptr)),
		uintptr(unsafe.Pointer(&ulen)))
	if ret == 0 {
		return nil, err
	}
	if infoptr == nil || ulen < uint32(unsafe.Sizeof(tagVSFIXEDFILEINFO{})) {
		return nil, fmt.Errorf("version resource has no fixed information (%d bytes)", ulen)
	}

	ffi := (*tagVSFIXEDFILEINFO)(infoptr)
	if ffi.dwSignature != vsFixedFileInfoSignature {
		return nil, fmt.Errorf("unexpected VS_FIXEDFILEINFO signature 0x%x", ffi.dwSignature)
	}
	return ffi, nil
}

func getVersionInfo(block []uint8) (string, error) {
	ffi, err := queryFixedFileInfo(block)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d.%d Build %d", ffi.dwProductVersionMS>>16, ffi.dwProductVersionMS&0xFF, ffi.dwProductVersionLS>>16), nil
}

// fixedFileVersion returns the file version from the VS_FIXEDFILEINFO structure of a
// version resource block, formatted as a dotted quad ("10.0.19041.1").
//
// This is preferred over the FileVersion string when a machine-comparable version is
// needed: the fixed structure holds four 16-bit integers, so it carries no localization,
// no decoration ("1.0.0.1 (build_lab)") and nothing to parse. It returns an empty string
// when the resource has no fixed information.
func fixedFileVersion(block []uint8) string {
	ffi, err := queryFixedFileInfo(block)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%d.%d.%d.%d",
		ffi.dwFileVersionMS>>16, ffi.dwFileVersionMS&0xFFFF,
		ffi.dwFileVersionLS>>16, ffi.dwFileVersionLS&0xFFFF)
}

// LoadIndirectString resolves a Windows indirect string reference of the form
// "@<dll>,-<resourceID>" against the string table of the named module, as SHLoadIndirectString
// does. Registry values that hold user-visible text — service display names among them — are
// commonly stored this way so that they can be localized.
//
// The input is returned unchanged when it is not an indirect reference. An error is returned
// when the reference cannot be resolved, which happens routinely: the module may not exist, or
// may not carry the resource.
//
// https://learn.microsoft.com/en-us/windows/win32/api/shlwapi/nf-shlwapi-shloadindirectstring
func LoadIndirectString(source string) (string, error) {
	if !strings.HasPrefix(source, "@") {
		return source, nil
	}

	sourcePtr, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return "", err
	}

	// Display names are short; MAX_PATH-sized buffers are what callers of this API use.
	buffer := make([]uint16, 1024)
	ret, _, _ := procSHLoadIndirectString.Call(
		uintptr(unsafe.Pointer(sourcePtr)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		0,
	)
	// The call returns an HRESULT, where any non-zero value is a failure.
	if ret != 0 {
		return "", fmt.Errorf("SHLoadIndirectString(%q) failed: HRESULT 0x%x", source, ret)
	}

	resolved := windows.UTF16ToString(buffer)
	if resolved == "" {
		return "", fmt.Errorf("SHLoadIndirectString(%q) resolved to an empty string", source)
	}
	return resolved, nil
}

// FileVersionInfo contains common version resource strings for a file.
type FileVersionInfo struct {
	CompanyName      string
	ProductName      string
	FileVersion      string
	ProductVersion   string
	OriginalFilename string
	InternalName     string

	// FileVersionNumeric is the file version from VS_FIXEDFILEINFO, formatted as a
	// dotted quad. Unlike FileVersion it is machine-comparable: it is built from four
	// 16-bit integers, so it is never localized or decorated. Empty when the resource
	// carries no fixed information.
	FileVersionNumeric string
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
	if len(data) == 0 {
		// Guard the &data[0] indexing below: getFileVersionInfo can report success with an
		// empty block if the size query returned zero.
		return info, errors.New("empty version information block")
	}

	// Read the fixed information before the string table: a resource may carry a valid
	// VS_FIXEDFILEINFO without a translation table, and the lookup below gives up in that
	// case.
	info.FileVersionNumeric = fixedFileVersion(data)

	// Get the first language/codepage from the translation table
	translationPtr, err := syscall.UTF16PtrFromString("\\VarFileInfo\\Translation")
	if err != nil {
		return info, err
	}

	var langCodePagePtr *uint16
	var langCodePageLen uint32
	ret, _, _ := procVerQueryValue.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(translationPtr)),
		uintptr(unsafe.Pointer(&langCodePagePtr)),
		uintptr(unsafe.Pointer(&langCodePageLen)),
	)
	if ret == 0 || langCodePageLen < 4 {
		return info, nil
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
