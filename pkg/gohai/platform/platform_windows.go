package platform

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const registryHive = "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion"
const productNameKey = "ProductName"
const buildNumberKey = "CurrentBuildNumber"
const majorKey = "CurrentMajorVersionNumber"
const minorKey = "CurrentMinorVersionNumber"

func GetVersion() (maj uint64, min uint64, err error) {
	var mod = syscall.NewLazyDLL("Netapi32.dll")
	var proc = mod.NewProc("NetWkstaGetInfo")
	var freeproc = mod.NewProc("NetApiBufferFree")

	var outdata *byte
	status, _, err := proc.Call(uintptr(0), uintptr(100), uintptr(unsafe.Pointer(&outdata)))
	if status != uintptr(0) {
		return 0, 0, err
	}
	defer freeproc.Call(uintptr(unsafe.Pointer(outdata)))
	return platGetVersion(outdata)

}

// GetArchInfo() returns basic host architecture information
func GetArchInfo() (systemInfo map[string]interface{}, err error) {
	systemInfo = make(map[string]interface{})

	systemInfo["hostname"], _ = os.Hostname()

	if runtime.GOARCH == "amd64" {
		systemInfo["machine"] = "x86_64"
	} else {
		systemInfo["machine"] = runtime.GOARCH
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		registryHive,
		registry.QUERY_VALUE)
	defer k.Close()

	systemInfo["os"], _, _ = k.GetStringValue(productNameKey)

	var maj, _, _ = k.GetIntegerValue(majorKey)
	var min, _, _ = k.GetIntegerValue(minorKey)
	var bld, _, _ = k.GetStringValue(buildNumberKey)
	if maj == 0 {
		maj, min, err = GetVersion()
		if 0 != syscall.Errno(0) {
			return
		}
	}
	verstring := fmt.Sprintf("%d.%d.%s", maj, min, bld)
	systemInfo["kernel_release"] = verstring

	systemInfo["kernel_name"] = "Windows"

	return
}
