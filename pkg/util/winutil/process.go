// +build windows

package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	modntdll                      = windows.NewLazyDLL("ntdll.dll")
	modkernel                     = windows.NewLazyDLL("kernel32.dll")
	procNtQueryInformationProcess = modntdll.NewProc("NtQueryInformationProcess")
	procReadProcessMemory         = modkernel.NewProc("ReadProcessMemory")
	procIsWow64Process            = modkernel.NewProc("IsWow64Process")
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

func getCommandLineForProcess32(h windows.Handle) (cmdline string, err error) {
	// get the pointer to the PEB
	var procmem uintptr
	size := unsafe.Sizeof(procmem)
	err = NtQueryInformationProcess(h, ProcessWow64Information, uintptr(unsafe.Pointer(&procmem)), size)
	if err != nil {
		// this shouldn't happen because we already know we're asking about
		// a 32 bit process.
		return
	}
	var peb peb32
	var read uint64
	var toRead uint32
	toRead = uint32(unsafe.Sizeof(peb))

	read, err = ReadProcessMemory(h, procmem, uintptr(unsafe.Pointer(&peb)), toRead)
	if err != nil {
		return
	}
	if read != uint64(toRead) {
		err = fmt.Errorf("Wrong amount of bytes read %v != %v", read, toRead)
		return
	}

	// now go get the actual parameters
	var pparams procParams32
	pparamsSize := unsafe.Sizeof(pparams)

	read, err = ReadProcessMemory(h, uintptr(peb.ProcessParameters), uintptr(unsafe.Pointer(&pparams)), uint32(pparamsSize))
	if err != nil {
		return
	}
	if read != uint64(pparamsSize) {
		err = fmt.Errorf("Wrong amount of bytes read %v != %v", read, pparamsSize)
		return
	}
	cmdlinebuffer := pparams.commandLine.buffer
	cmdlinelen := pparams.commandLine.length
	//cmdlinelen := 1024

	finalbuf := make([]uint8, cmdlinelen+2)

	read, err = ReadProcessMemory(h, uintptr(cmdlinebuffer), uintptr(unsafe.Pointer(&finalbuf[0])), uint32(cmdlinelen+2))
	if err != nil {
		return
	}
	if read != uint64(cmdlinelen+2) {
		err = fmt.Errorf("Wrong amount of bytes read %v != %v", read, cmdlinelen+2)
		return
	}
	cmdline = ConvertWindowsString(finalbuf)
	return

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

func getCommandLineForProcess64(h windows.Handle) (cmdline string, err error) {
	var pbi processBasicInformationStruct
	pbisize := unsafe.Sizeof(pbi)
	err = NtQueryInformationProcess(h, ProcessBasicInformation, uintptr(unsafe.Pointer(&pbi)), pbisize)
	if err != nil {
		return
	}
	// read the peb
	var peb _peb
	pebsize := unsafe.Sizeof(peb)
	readsize, err := ReadProcessMemory(h, pbi.PebBaseAddress, uintptr(unsafe.Pointer(&peb)), uint32(pebsize))
	if err != nil {
		return
	}
	if readsize != uint64(pebsize) {
		err = fmt.Errorf("Incorrect read size %v %v", readsize, pebsize)
		return
	}
	// go get the parameters
	var pparams _rtlUserProcessParameters
	paramsize := unsafe.Sizeof(pparams)
	readsize, err = ReadProcessMemory(h, peb.ProcessParameters, uintptr(unsafe.Pointer(&pparams)), uint32(paramsize))
	if readsize != uint64(paramsize) {
		err = fmt.Errorf("Incorrect read size %v %v", readsize, paramsize)
		return
	}
	cmdlinebuffer := make([]uint8, pparams.commandLine.length+2)
	readsize, err = ReadProcessMemory(h, pparams.commandLine.buffer, uintptr(unsafe.Pointer(&cmdlinebuffer[0])), uint32(pparams.commandLine.length+2))
	if err != nil {
		return
	}
	if readsize != uint64(pparams.commandLine.length+2) {
		err = fmt.Errorf("Wrong amount of bytes read %v != %v", readsize, pparams.commandLine.length+2)
		return
	}
	cmdline = ConvertWindowsString(cmdlinebuffer)
	return
}

// GetCommandLineForProcess returns the command line for the given process.
func GetCommandLineForProcess(h windows.Handle) (cmdline string, err error) {
	// first need to check if this is a 32 bit process running on win64

	// for now, assumes we are win64

	is32bit, err := IsWow64Process(h)
	if is32bit {
		return getCommandLineForProcess32(h)
	}
	return getCommandLineForProcess64(h)
}

// GetCommandLineForPid returns the command line for the given PID
func GetCommandLineForPid(pid uint32) (cmdline string, err error) {
	h, err := windows.OpenProcess(0x1010, false, uint32(pid))
	if err != nil {
		err = fmt.Errorf("Failed to open process %v", err)
		return
	}
	defer windows.CloseHandle(h)
	cmdline, err = GetCommandLineForProcess(h)
	if err != nil {
		err = fmt.Errorf("Failed to get command line %v", err)
		return
	}
	return GetCommandLineForProcess(h)
}
