// +build windows

package winutil

import (
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
	FOLDERID_ProgramData = GUID{0x62AB5D82, 0xFDC1, 0x4DC3, [8]byte{0xA9, 0xDD, 0x07, 0x0D, 0x1D, 0x49, 0x5D, 0x97}}
	FOLDERID_Fonts       = GUID{0xFD228CB7, 0xAE11, 0x4AE3, [8]byte{0x86, 0x4C, 0x16, 0xF3, 0x91, 0x0A, 0xB8, 0xFE}}
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

// GetProgramDataDir() returns the current programdatadir, usually
// c:\programdata
func GetProgramDataDir() (path string, err error) {
	var retstr uintptr
	err = SHGetKnownFolderPath(&FOLDERID_ProgramData, 0, 0, &retstr)
	if err == nil {
		// convert the string
		defer CoTaskMemFree(retstr)
		path = syscall.UTF16ToString((*[1 << 16]uint16)(unsafe.Pointer(retstr))[:])
	}
	return
}
