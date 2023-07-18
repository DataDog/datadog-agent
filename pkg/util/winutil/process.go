// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	modntdll                       = windows.NewLazyDLL("ntdll.dll")
	modkernel                      = windows.NewLazyDLL("kernel32.dll")
	procNtQueryInformationProcess  = modntdll.NewProc("NtQueryInformationProcess")
	procReadProcessMemory          = modkernel.NewProc("ReadProcessMemory")
	procIsWow64Process             = modkernel.NewProc("IsWow64Process")
	procQueryFullProcessImageNameW = modkernel.NewProc("QueryFullProcessImageNameW")
)

// C definition from winternl.h

//typedef enum _PROCESSINFOCLASS {
//    ProcessBasicInformation = 0,
//    ProcessDebugPort = 7,
//    ProcessWow64Information = 26,
//    ProcessImageFileName = 27,
//    ProcessBreakOnTermination = 29
//} PROCESSINFOCLASS;

// PROCESSINFOCLASS is the Go representation of the above enum
type PROCESSINFOCLASS uint32

const (
	// ProcessBasicInformation returns the PEB type
	ProcessBasicInformation = PROCESSINFOCLASS(0)
	// ProcessDebugPort included for completeness
	ProcessDebugPort = PROCESSINFOCLASS(7)
	// ProcessWow64Information included for completeness
	ProcessWow64Information = PROCESSINFOCLASS(26)
	// ProcessImageFileName included for completeness
	ProcessImageFileName = PROCESSINFOCLASS(27)
	// ProcessBreakOnTermination included for completeness
	ProcessBreakOnTermination = PROCESSINFOCLASS(29)
)

// IsWow64Process determines if the specified process is running under WOW64
// that is, if it's a 32 bit process running on 64 bit winodws
func IsWow64Process(h windows.Handle) (is32bit bool, err error) {
	var wow64Process uint32

	r, _, _ := procIsWow64Process.Call(uintptr(h),
		uintptr(unsafe.Pointer(&wow64Process)))

	if r == 0 {
		return false, windows.GetLastError()
	}
	if wow64Process == 0 {
		is32bit = false
	} else {
		is32bit = true
	}
	return
}

// NtQueryInformationProcess wraps the Windows NT kernel call of the same name
func NtQueryInformationProcess(h windows.Handle, class PROCESSINFOCLASS, target, size uintptr) (err error) {
	r, _, _ := procNtQueryInformationProcess.Call(uintptr(h),
		uintptr(class),
		target,
		size,
		uintptr(0))
	if r != 0 {
		err = windows.GetLastError()
		return
	}
	return
}

// ReadProcessMemory wraps the Windows kernel.dll function of the same name
// https://docs.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-readprocessmemory
func ReadProcessMemory(h windows.Handle, from, to uintptr, count uint32) (bytesRead uint64, err error) {
	var bytes uint64

	r, _, e := procReadProcessMemory.Call(uintptr(h),
		from,
		to,
		uintptr(count),
		uintptr(unsafe.Pointer(&bytes)))

	if r == 0 {
		if e == windows.ERROR_ACCESS_DENIED {
			log.Debugf("Access denied error getting process memory")
		} else {
			log.Warnf("Unexpected error getting process memory for handle (h) %v (err) %v", h, e)
		}
		return 0, e
	}
	bytesRead = bytes
	return
}

type peb32 struct {
	Reserved1         [2]byte
	BeingDebugged     byte
	Reserved2         [1]byte
	Reserved3         [2]uint32
	Ldr               uint32
	ProcessParameters uint32
	// more fields...
}

type unicodeString32 struct {
	length    uint16
	maxLength uint16
	buffer    uint32
}
type procParams32 struct {
	Reserved1              [16]byte
	Reserved2              [5]uint32
	CurrentDirectoryPath   unicodeString32
	CurrentDirectoryHandle uint32
	DllPath                unicodeString32
	ImagePath              unicodeString32
	commandLine            unicodeString32
	env                    uint32
}

// ProcessCommandParams defines process command params
type ProcessCommandParams struct {
	CmdLine   string
	ImagePath string
}

func getCommandParamsForProcess32(h windows.Handle, includeImagePath bool) (*ProcessCommandParams, error) {
	// get the pointer to the PEB
	var procmem uintptr
	size := unsafe.Sizeof(procmem)
	err := NtQueryInformationProcess(h, ProcessWow64Information, uintptr(unsafe.Pointer(&procmem)), size)
	if err != nil {
		// this shouldn't happen because we already know we're asking about
		// a 32 bit process.
		return nil, err
	}
	var peb peb32
	var read uint64
	var toRead uint32
	toRead = uint32(unsafe.Sizeof(peb))

	read, err = ReadProcessMemory(h, procmem, uintptr(unsafe.Pointer(&peb)), toRead)
	if err != nil {
		return nil, err
	}
	if read != uint64(toRead) {
		err = fmt.Errorf("Wrong amount of bytes read %v != %v", read, toRead)
		return nil, err
	}

	// now go get the actual parameters
	var pparams procParams32
	pparamsSize := unsafe.Sizeof(pparams)

	read, err = ReadProcessMemory(h, uintptr(peb.ProcessParameters), uintptr(unsafe.Pointer(&pparams)), uint32(pparamsSize))
	if err != nil {
		return nil, err
	}
	if read != uint64(pparamsSize) {
		err = fmt.Errorf("Wrong amount of bytes read %v != %v", read, pparamsSize)
		return nil, err
	}

	cmdline, err := readUnicodeString32(h, pparams.commandLine)
	if err != nil {
		return nil, err
	}

	var imagepath string
	if includeImagePath {
		imagepath, err = readUnicodeString32(h, pparams.ImagePath)
		if err != nil {
			return nil, err
		}
	}

	procCommandParams := &ProcessCommandParams{
		CmdLine:   cmdline,
		ImagePath: imagepath,
	}

	return procCommandParams, nil
}

func readUnicodeString32(h windows.Handle, u unicodeString32) (string, error) {
	if u.length > u.maxLength {
		return "", fmt.Errorf("Invalid unicodeString32, maxLength %v < length %v", u.maxLength, u.length)
	}
	// length does not include null terminator, if it exists
	// allocate two extra bytes so we can add it ourself
	buf := make([]uint8, u.length+2)
	read, err := ReadProcessMemory(h, uintptr(u.buffer), uintptr(unsafe.Pointer(&buf[0])), uint32(u.length))
	if err != nil {
		return "", err
	}
	if read != uint64(u.length) {
		return "", fmt.Errorf("Wrong amount of bytes read (unicodeString32) %v != %v", read, u.length)
	}
	// null terminate string
	buf = append(buf, 0, 0)
	return ConvertWindowsString(buf), nil
}

// this definition taken from Winternl.h
type unicodeString struct {
	length    uint16
	maxLength uint16
	buffer    uintptr
}

type _rtlUserProcessParameters struct {
	Reserved1     [16]byte
	Reserved2     [10]uintptr
	imagePathName unicodeString
	commandLine   unicodeString
}
type _peb struct {
	Reserved1         [2]byte
	BeingDebugged     byte
	Reserved2         [2]byte
	Reserved3         [2]uintptr
	Ldr               uintptr // pointer to PEB_LDR_DATA
	ProcessParameters uintptr // pointer to _rtlUserProcessParameters
	// lots more stuff
}

// this definition taken from Winternl.h
type processBasicInformationStruct struct {
	Reserved1       uintptr
	PebBaseAddress  uintptr
	Reserved2       [2]uintptr
	UniqueProcessID uintptr
	Reserved3       uintptr
}

func getCommandParamsForProcess64(h windows.Handle, includeImagePath bool) (*ProcessCommandParams, error) {
	var pbi processBasicInformationStruct
	pbisize := unsafe.Sizeof(pbi)
	err := NtQueryInformationProcess(h, ProcessBasicInformation, uintptr(unsafe.Pointer(&pbi)), pbisize)
	if err != nil {
		return nil, err
	}
	// read the peb
	var peb _peb
	pebsize := unsafe.Sizeof(peb)
	readsize, err := ReadProcessMemory(h, pbi.PebBaseAddress, uintptr(unsafe.Pointer(&peb)), uint32(pebsize))
	if err != nil {
		return nil, err
	}
	if readsize != uint64(pebsize) {
		err = fmt.Errorf("Incorrect read size %v %v", readsize, pebsize)
		return nil, err
	}

	// go get the parameters
	var pparams _rtlUserProcessParameters
	paramsize := unsafe.Sizeof(pparams)
	readsize, err = ReadProcessMemory(h, peb.ProcessParameters, uintptr(unsafe.Pointer(&pparams)), uint32(paramsize))
	if err != nil {
		return nil, err
	}
	if readsize != uint64(paramsize) {
		return nil, fmt.Errorf("Incorrect read size %v %v", readsize, paramsize)
	}

	cmdline, err := readUnicodeString(h, pparams.commandLine)
	if err != nil {
		return nil, err
	}

	var imagepath string
	if includeImagePath {
		imagepath, err = readUnicodeString(h, pparams.imagePathName)
		if err != nil {
			return nil, err
		}
	}

	procCommandParams := &ProcessCommandParams{
		CmdLine:   cmdline,
		ImagePath: imagepath,
	}

	return procCommandParams, nil
}

func readUnicodeString(h windows.Handle, u unicodeString) (string, error) {
	if u.length > u.maxLength {
		return "", fmt.Errorf("Invalid unicodeString, maxLength %v < length %v", u.maxLength, u.length)
	}
	// length does not include null terminator, if it exists
	// allocate two extra bytes so we can add it ourself
	buf := make([]uint8, u.length+2)
	read, err := ReadProcessMemory(h, uintptr(u.buffer), uintptr(unsafe.Pointer(&buf[0])), uint32(u.length))
	if err != nil {
		return "", err
	}
	if read != uint64(u.length) {
		return "", fmt.Errorf("Wrong amount of bytes read (unicodeString) %v != %v", read, u.length)
	}
	// null terminate string
	buf = append(buf, 0, 0)
	return ConvertWindowsString(buf), nil
}

// GetCommandParamsForProcess returns the command line (and optionally image path) for the given process
func GetCommandParamsForProcess(h windows.Handle, includeImagePath bool) (*ProcessCommandParams, error) {
	// first need to check if this is a 32 bit process running on win64

	// for now, assumes we are win64
	is32bit, _ := IsWow64Process(h)
	if is32bit {
		return getCommandParamsForProcess32(h, includeImagePath)
	}
	return getCommandParamsForProcess64(h, includeImagePath)
}

// GetCommandParamsForPid returns the command line (and optionally image path) for the given PID
func GetCommandParamsForPid(pid uint32, includeImagePath bool) (*ProcessCommandParams, error) {
	h, err := windows.OpenProcess(0x1010, false, uint32(pid))
	if err != nil {
		err = fmt.Errorf("Failed to open process %v", err)
		return nil, err
	}
	defer windows.CloseHandle(h)
	return GetCommandParamsForProcess(h, includeImagePath)
}

// GetImagePathForProcess returns executable path name in the win32 format
func GetImagePathForProcess(h windows.Handle) (string, error) {
	const maxPath = 260
	// Note that this isn't entirely accurate in all cases, the max can actually be 32K
	// (requires a registry setting change)
	// https://docs.microsoft.com/en-us/windows/win32/fileio/maximum-file-path-limitation?tabs=cmd
	// In this particular case we are opting for MAX_PATH because 32k is a lot to allocate
	// in most cases where this API will be used (process enumeration loop)
	var buf [maxPath + 1]uint16
	n := uint32(len(buf))
	_, _, lastErr := procQueryFullProcessImageNameW.Call(
		uintptr(h),
		uintptr(0),
		uintptr(unsafe.Pointer(&buf)),
		uintptr(unsafe.Pointer(&n)))
	if lastErr.(syscall.Errno) == 0 {
		return syscall.UTF16ToString(buf[:n]), nil
	}
	return "", lastErr
}

const (
	processQueryLimitedInformation = windows.PROCESS_QUERY_LIMITED_INFORMATION

	stillActive = windows.STATUS_PENDING
)

// IsProcess checks to see if a given pid is currently valid in the process table
func IsProcess(pid int) bool {
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	var c windows.NTStatus
	err = windows.GetExitCodeProcess(h, (*uint32)(&c))
	windows.Close(h)
	if err == nil {
		return c == stillActive
	}
	return false
}

func getProcessStartTimeAsNs(pid uint64) (uint64, error) {
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return 0, fmt.Errorf("Error opening process %v", err)
	}
	var creation windows.Filetime
	var exit windows.Filetime
	var krn windows.Filetime
	var user windows.Filetime
	err = windows.GetProcessTimes(h, &creation, &exit, &krn, &user)
	if err != nil {
		return 0, err
	}
	return uint64(creation.Nanoseconds()), nil
}
