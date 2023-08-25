// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"unicode/utf16"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// osVersionInfoEXW contains operating system version information.
// From winnt.h (see https://docs.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-osversioninfoexw)
// This is used by https://docs.microsoft.com/en-us/windows/win32/devnotes/rtlgetversion
type osVersionInfoEXW struct {
	dwOSVersionInfoSize uint32
	dwMajorVersion      uint32
	dwMinorVersion      uint32
	dwBuildNumber       uint32
	dwPlatformID        uint32
	szCSDVersion        [128]uint16
	wServicePackMajor   uint16
	wServicePackMinor   uint16
	wSuiteMask          uint16
	wProductType        uint8
	wReserved           uint8
}

// serverInfo101 contains server-specific information
// see https://learn.microsoft.com/en-us/windows/win32/api/lmserver/ns-lmserver-server_info_101
type serverInfo101 struct {
	sv101PlatformID uint32
	//nolint:unused
	sv101Name         string
	sv101VersionMajor uint32
	sv101VersionMinor uint32
	sv101Type         uint32
	//nolint:unused
	sv101Comment string
}

var (
	modNetapi32          = windows.NewLazyDLL("Netapi32.dll")
	procNetServerGetInfo = modNetapi32.NewProc("NetServerGetInfo")
	procNetAPIBufferFree = modNetapi32.NewProc("NetApiBufferFree")
	ntdll                = windows.NewLazyDLL("Ntdll.dll")
	procRtlGetVersion    = ntdll.NewProc("RtlGetVersion")
	winbrand             = windows.NewLazyDLL("winbrand.dll")
	kernel32             = windows.NewLazyDLL("kernel32.dll")
	procIsWow64Process2  = kernel32.NewProc("IsWow64Process2")
)

// see https://learn.microsoft.com/en-us/windows/win32/api/lmserver/nf-lmserver-netserverenum
const (
	// svTypeWorkstation is for all workstations.
	svTypeWorkstation = uint32(0x00000001)
	// svTypeServer is for all computers that run the Server service.
	svTypeServer = uint32(0x00000002)
	// svTypeDomainCtrl is for a server that is primary domain controller.
	svTypeDomainCtrl = uint32(0x00000008)
	// svTypeDomainBakctrl is for any server that is a backup domain controller.
	svTypeDomainBakctrl = uint32(0x00000010)
	// svTypeDomainMember is for any computer that is LAN Manager 2.x domain member.
	svTypeDomainMember = uint32(0x00000100)
)

const (
	// imageFileMachineAmd64 is AMD64 (K8)
	imageFileMachineAmd64 = uint16(0x8664)
	// imageFileMachineArm64 is ARM64 Little-Endian
	imageFileMachineArm64 = uint16(0xAA64)
)
const registryHive = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"
const productNameKey = "ProductName"
const buildNumberKey = "CurrentBuildNumber"
const majorKey = "CurrentMajorVersionNumber"
const minorKey = "CurrentMinorVersionNumber"

func netServerGetInfo() (si serverInfo101, err error) {
	var outdata *byte
	// do additional work so that we don't panic() when the library's
	// not there (like in a container)
	if err = modNetapi32.Load(); err != nil {
		return
	}
	if err = procNetServerGetInfo.Find(); err != nil {
		return
	}
	status, _, err := procNetServerGetInfo.Call(uintptr(0), uintptr(101), uintptr(unsafe.Pointer(&outdata)))
	if status != uintptr(0) {
		return
	}
	defer func() { _, _, _ = procNetAPIBufferFree.Call(uintptr(unsafe.Pointer(outdata))) }()
	return platGetServerInfo(outdata), nil
}

func fetchOsDescription() (string, error) {
	err := winbrand.Load()
	if err == nil {
		// From https://stackoverflow.com/a/69462683
		procBrandingFormatString := winbrand.NewProc("BrandingFormatString")
		if procBrandingFormatString.Find() == nil {
			// Encode the string "%WINDOWS_LONG%" to UTF-16 and append a null byte for the Windows API
			magicString := utf16.Encode([]rune("%WINDOWS_LONG%" + "\x00"))
			os, _, err := procBrandingFormatString.Call(uintptr(unsafe.Pointer(&magicString[0])))
			if err == windows.ERROR_SUCCESS {
				defer func() { _, _ = windows.LocalFree(windows.Handle(os)) }()
				// govet complains about possible misuse of unsafe.Pointer here
				//nolint:govet
				return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(os))), nil
			}
		}
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		registryHive,
		registry.QUERY_VALUE)
	if err == nil {
		defer func() { _ = k.Close() }()
		os, _, err := k.GetStringValue(productNameKey)
		if err == nil {
			return os, nil
		}
	}

	return "(undetermined windows version)", err
}

func fetchWindowsVersion() (major uint64, minor uint64, build uint64, err error) {
	var osversion osVersionInfoEXW
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&osversion)))
	if status == 0 {
		major = uint64(osversion.dwMajorVersion)
		minor = uint64(osversion.dwMinorVersion)
		build = uint64(osversion.dwBuildNumber)
	} else {
		var regkey registry.Key
		regkey, err = registry.OpenKey(registry.LOCAL_MACHINE,
			registryHive,
			registry.QUERY_VALUE)
		if err != nil {
			return
		}
		defer func() { _ = regkey.Close() }()
		major, _, err = regkey.GetIntegerValue(majorKey)
		if err != nil {
			return
		}

		minor, _, err = regkey.GetIntegerValue(minorKey)
		if err != nil {
			return
		}

		var regbuild string
		regbuild, _, err = regkey.GetStringValue(buildNumberKey)
		if err != nil {
			return
		}
		build, err = strconv.ParseUint(regbuild, 10, 0)
	}
	return
}

// check to see if we're running on syswow64 on another architecture
// (specifically arm)
// the function we're going to use (IsWow64Process2) isn't available prior
// to win10/2016.  Fail gracefully, and assume we're not on wow in that
// case
func getNativeArchInfo() string {
	var nativearch string
	if runtime.GOARCH == "amd64" {
		nativearch = "x86_64"
	} else {
		nativearch = runtime.GOARCH
	}
	var err error
	if err = kernel32.Load(); err == nil {
		if err = procIsWow64Process2.Find(); err == nil {
			var pmachine uint16
			var pnative uint16
			h := windows.CurrentProcess()
			b, _, _ := procIsWow64Process2.Call(uintptr(h), uintptr(unsafe.Pointer(&pmachine)), uintptr(unsafe.Pointer(&pnative)))
			if b != uintptr(0) {
				// check to see the native processor type.
				switch pnative {
				case imageFileMachineAmd64:
					// it's already set to this
					nativearch = "x86_64"
				case imageFileMachineArm64:
					nativearch = "ARM64"
				}
			}
		}
	}
	return nativearch
}

func (platformInfo *Info) fillPlatformInfo() {
	platformInfo.KernelVersion = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.Processor = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.HardwarePlatform = utils.NewErrorValue[string](utils.ErrNotCollectable)

	platformInfo.Hostname = utils.NewValueFrom(os.Hostname())
	platformInfo.Machine = utils.NewValue(getNativeArchInfo())
	platformInfo.OS = utils.NewValueFrom(fetchOsDescription())

	maj, min, bld, err := fetchWindowsVersion()
	platformInfo.KernelRelease = utils.NewValueFrom(fmt.Sprintf("%d.%d.%d", maj, min, bld), err)

	platformInfo.KernelName = utils.NewValue("Windows")

	// do additional work so that we don't panic() when the library's
	// not there (like in a container)
	family := "Unknown"
	si, sierr := netServerGetInfo()
	if sierr == nil {
		if (si.sv101Type&svTypeWorkstation) == svTypeWorkstation ||
			(si.sv101Type&svTypeServer) == svTypeServer {
			if (si.sv101Type & svTypeWorkstation) == svTypeWorkstation {
				family = "Workstation"
			} else if (si.sv101Type & svTypeServer) == svTypeServer {
				family = "Server"
			}
			if (si.sv101Type & svTypeDomainMember) == svTypeDomainMember {
				family = "Domain Joined " + family
			} else {
				family = "Standalone " + family
			}
		} else if (si.sv101Type & svTypeDomainCtrl) == svTypeDomainCtrl {
			family = "Domain Controller"
		} else if (si.sv101Type & svTypeDomainBakctrl) == svTypeDomainBakctrl {
			family = "Backup Domain Controller"
		}
	}
	platformInfo.Family = utils.NewValue(family)
}
