package platform

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// From winnt.h (see https://docs.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-osversioninfoexw)
// This is used by https://docs.microsoft.com/en-us/windows/win32/devnotes/rtlgetversion
type OSVERSIONINFOEXW struct {
	dwOSVersionInfoSize uint32
	dwMajorVersion      uint32
	dwMinorVersion      uint32
	dwBuildNumber       uint32
	dwPlatformId        uint32
	szCSDVersion        [128]uint16
	wServicePackMajor   uint16
	wServicePackMinor   uint16
	wSuiteMask          uint16
	wProductType        uint8
	wReserved           uint8
}

var (
	modNetapi32                        = windows.NewLazyDLL("Netapi32.dll")
	procNetServerGetInfo               = modNetapi32.NewProc("NetServerGetInfo")
	procNetApiBufferFree               = modNetapi32.NewProc("NetApiBufferFree")
	ntdll                              = windows.NewLazyDLL("Ntdll.dll")
	procRtlGetVersion                  = ntdll.NewProc("RtlGetVersion")
	winbrand                           = windows.NewLazyDLL("winbrand.dll")
	kernel32                           = windows.NewLazyDLL("kernel32.dll")
	procIsWow64Process2                = kernel32.NewProc("IsWow64Process2")
	ERROR_SUCESS         syscall.Errno = 0
)

const (
	SV_TYPE_WORKSTATION       = uint32(0x00000001)
	SV_TYPE_SERVER            = uint32(0x00000002)
	SV_TYPE_SQLSERVER         = uint32(0x00000004)
	SV_TYPE_DOMAIN_CTRL       = uint32(0x00000008)
	SV_TYPE_DOMAIN_BAKCTRL    = uint32(0x00000010)
	SV_TYPE_TIME_SOURCE       = uint32(0x00000020)
	SV_TYPE_AFP               = uint32(0x00000040)
	SV_TYPE_NOVELL            = uint32(0x00000080)
	SV_TYPE_DOMAIN_MEMBER     = uint32(0x00000100)
	SV_TYPE_PRINTQ_SERVER     = uint32(0x00000200)
	SV_TYPE_DIALIN_SERVER     = uint32(0x00000400)
	SV_TYPE_XENIX_SERVER      = uint32(0x00000800)
	SV_TYPE_SERVER_UNIX       = SV_TYPE_XENIX_SERVER
	SV_TYPE_NT                = uint32(0x00001000)
	SV_TYPE_WFW               = uint32(0x00002000)
	SV_TYPE_SERVER_MFPN       = uint32(0x00004000)
	SV_TYPE_SERVER_NT         = uint32(0x00008000)
	SV_TYPE_POTENTIAL_BROWSER = uint32(0x00010000)
	SV_TYPE_BACKUP_BROWSER    = uint32(0x00020000)
	SV_TYPE_MASTER_BROWSER    = uint32(0x00040000)
	SV_TYPE_DOMAIN_MASTER     = uint32(0x00080000)
	SV_TYPE_SERVER_OSF        = uint32(0x00100000)
	SV_TYPE_SERVER_VMS        = uint32(0x00200000)
	SV_TYPE_WINDOWS           = uint32(0x00400000) /* Windows95 and above */
	SV_TYPE_DFS               = uint32(0x00800000) /* Root of a DFS tree */
	SV_TYPE_CLUSTER_NT        = uint32(0x01000000) /* NT Cluster */
	SV_TYPE_TERMINALSERVER    = uint32(0x02000000) /* Terminal Server(Hydra) */
	SV_TYPE_CLUSTER_VS_NT     = uint32(0x04000000) /* NT Cluster Virtual Server Name */
	SV_TYPE_DCE               = uint32(0x10000000) /* IBM DSS (Directory and Security Services) or equivalent */
	SV_TYPE_ALTERNATE_XPORT   = uint32(0x20000000) /* return list for alternate transport */
	SV_TYPE_LOCAL_LIST_ONLY   = uint32(0x40000000) /* Return local list only */
	SV_TYPE_DOMAIN_ENUM       = uint32(0x80000000)
	SV_TYPE_ALL               = uint32(0xFFFFFFFF) /* handy for NetServerEnum2 */
)

const (
	IMAGE_FILE_MACHINE_UNKNOWN     = uint16(0x0)
	IMAGE_FILE_MACHINE_TARGET_HOST = uint16(0x0001) // Useful for indicating we want to interact with the host and not a WoW guest.  Win10/2016 and above only
	IMAGE_FILE_MACHINE_I386        = uint16(0x014c) // Intel 386.
	IMAGE_FILE_MACHINE_R3000       = uint16(0x0162) // MIPS little-endian, = uint16(0x160 big-endian
	IMAGE_FILE_MACHINE_R4000       = uint16(0x0166) // MIPS little-endian
	IMAGE_FILE_MACHINE_R10000      = uint16(0x0168) // MIPS little-endian
	IMAGE_FILE_MACHINE_WCEMIPSV2   = uint16(0x0169) // MIPS little-endian WCE v2
	IMAGE_FILE_MACHINE_ALPHA       = uint16(0x0184) // Alpha_AXP
	IMAGE_FILE_MACHINE_SH3         = uint16(0x01a2) // SH3 little-endian
	IMAGE_FILE_MACHINE_SH3DSP      = uint16(0x01a3)
	IMAGE_FILE_MACHINE_SH3E        = uint16(0x01a4) // SH3E little-endian
	IMAGE_FILE_MACHINE_SH4         = uint16(0x01a6) // SH4 little-endian
	IMAGE_FILE_MACHINE_SH5         = uint16(0x01a8) // SH5
	IMAGE_FILE_MACHINE_ARM         = uint16(0x01c0) // ARM Little-Endian
	IMAGE_FILE_MACHINE_THUMB       = uint16(0x01c2) // ARM Thumb/Thumb-2 Little-Endian
	IMAGE_FILE_MACHINE_ARMNT       = uint16(0x01c4) // ARM Thumb-2 Little-Endian
	IMAGE_FILE_MACHINE_AM33        = uint16(0x01d3)
	IMAGE_FILE_MACHINE_POWERPC     = uint16(0x01F0) // IBM PowerPC Little-Endian
	IMAGE_FILE_MACHINE_POWERPCFP   = uint16(0x01f1)
	IMAGE_FILE_MACHINE_IA64        = uint16(0x0200) // Intel 64
	IMAGE_FILE_MACHINE_MIPS16      = uint16(0x0266) // MIPS
	IMAGE_FILE_MACHINE_ALPHA64     = uint16(0x0284) // ALPHA64
	IMAGE_FILE_MACHINE_MIPSFPU     = uint16(0x0366) // MIPS
	IMAGE_FILE_MACHINE_MIPSFPU16   = uint16(0x0466) // MIPS
	IMAGE_FILE_MACHINE_AXP64       = IMAGE_FILE_MACHINE_ALPHA64
	IMAGE_FILE_MACHINE_TRICORE     = uint16(0x0520) // Infineon
	IMAGE_FILE_MACHINE_CEF         = uint16(0x0CEF)
	IMAGE_FILE_MACHINE_EBC         = uint16(0x0EBC) // EFI Byte Code
	IMAGE_FILE_MACHINE_AMD64       = uint16(0x8664) // AMD64 (K8)
	IMAGE_FILE_MACHINE_M32R        = uint16(0x9041) // M32R little-endian
	IMAGE_FILE_MACHINE_ARM64       = uint16(0xAA64) // ARM64 Little-Endian
	IMAGE_FILE_MACHINE_CEE         = uint16(0xC0EE)
)
const registryHive = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"
const productNameKey = "ProductName"
const buildNumberKey = "CurrentBuildNumber"
const majorKey = "CurrentMajorVersionNumber"
const minorKey = "CurrentMinorVersionNumber"

func netServerGetInfo() (si SERVER_INFO_101, err error) {
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
	// ignore free errors
	//nolint:errcheck
	defer procNetApiBufferFree.Call(uintptr(unsafe.Pointer(outdata)))
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
			if err == ERROR_SUCESS {
				// ignore free errors
				//nolint:errcheck
				defer syscall.LocalFree(syscall.Handle(os))
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
		// ignore registry key close errors
		//nolint:staticcheck
		defer k.Close()
		os, _, err := k.GetStringValue(productNameKey)
		if err == nil {
			return os, nil
		}
	}

	return "(undetermined windows version)", err
}

func fetchWindowsVersion() (major uint64, minor uint64, build uint64, err error) {
	var osversion OSVERSIONINFOEXW
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
		// ignore registry key close errors
		//nolint:staticcheck
		defer regkey.Close()
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
	nativearch := "x86_64"
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
				case IMAGE_FILE_MACHINE_AMD64:
					// it's already set to this
					nativearch = "x86_64"
				case IMAGE_FILE_MACHINE_ARM64:
					nativearch = "ARM64"
				}
			}
		}
	}
	return nativearch
}

// GetArchInfo() returns basic host architecture information
func GetArchInfo() (systemInfo map[string]string, err error) {
	systemInfo = map[string]string{}

	hostname, err := os.Hostname()
	if err == nil {
		systemInfo["hostname"] = hostname
	}

	systemInfo["machine"] = getNativeArchInfo()

	osDescription, err := fetchOsDescription()
	if err == nil {
		systemInfo["os"] = osDescription
	}

	maj, min, bld, err := fetchWindowsVersion()
	if err == nil {
		verstring := fmt.Sprintf("%d.%d.%d", maj, min, bld)
		systemInfo["kernel_release"] = verstring
	}

	systemInfo["kernel_name"] = "Windows"

	// do additional work so that we don't panic() when the library's
	// not there (like in a container)
	family := "Unknown"
	si, sierr := netServerGetInfo()
	if sierr == nil {
		if (si.sv101_type&SV_TYPE_WORKSTATION) == SV_TYPE_WORKSTATION ||
			(si.sv101_type&SV_TYPE_SERVER) == SV_TYPE_SERVER {
			if (si.sv101_type & SV_TYPE_WORKSTATION) == SV_TYPE_WORKSTATION {
				family = "Workstation"
			} else if (si.sv101_type & SV_TYPE_SERVER) == SV_TYPE_SERVER {
				family = "Server"
			}
			if (si.sv101_type & SV_TYPE_DOMAIN_MEMBER) == SV_TYPE_DOMAIN_MEMBER {
				family = "Domain Joined " + family
			} else {
				family = "Standalone " + family
			}
		} else if (si.sv101_type & SV_TYPE_DOMAIN_CTRL) == SV_TYPE_DOMAIN_CTRL {
			family = "Domain Controller"
		} else if (si.sv101_type & SV_TYPE_DOMAIN_BAKCTRL) == SV_TYPE_DOMAIN_BAKCTRL {
			family = "Backup Domain Controller"
		}
	}
	systemInfo["family"] = family

	// systemInfo is never empty so we never return an error
	err = nil

	return
}
