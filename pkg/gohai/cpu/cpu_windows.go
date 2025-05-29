//go:build windows

package cpu

/*
#cgo LDFLAGS: -lkernel32
#include "cpu_windows.h"
*/
import "C"
import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const registryHive = "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0"

// systemInfo contains information about the current computer system.
// This includes the architecture and type of the processor, the number
// of processors in the system, the page size, and other such information.
// see https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/ns-sysinfoapi-system_info
//
//nolint:revive
type systemInfo struct {
	wProcessorArchitecture  uint16
	wReserved               uint16
	dwPageSize              uint32
	lpMinApplicationAddress *uint32
	lpMaxApplicationAddress *uint32
	dwActiveProcessorMask   uintptr
	dwNumberOfProcessors    uint32
	dwProcessorType         uint32
	dwAllocationGranularity uint32
	wProcessorLevel         uint16
	wProcessorRevision      uint16
}

// cpuInfo holds CPU information
type cpuInfo struct {
	corecount           int
	logicalcount        int
	pkgcount            int
	numaNodeCount       int
	relationGroups      int
	maxProcsInGroups    int
	activeProcsInGroups int
	l1CacheSize         uint64
	l2CacheSize         uint64
	l3CacheSize         uint64
	VendorID            string
	ModelName           string
	Mhz                 float64
	Family              string
	Model               string
	Stepping            string
}

func extract(caption, field string) string {
	re := regexp.MustCompile(fmt.Sprintf("%s [0-9]* ", field))
	matches := re.FindStringSubmatch(caption)
	if len(matches) > 0 {
		return strings.Split(matches[0], " ")[1]
	}
	return ""
}

func getCPUInfo() *Info {
	var cInfo cpuInfo
	var cCpuInfo C.CPU_INFO

	ret := C.computeCoresAndProcessors(&cCpuInfo)
	if ret != 0 {
		log.Errorf("failed to get CPU information, error code: %d", ret)
		return &Info{}
	}

	// Copy C struct values to Go struct
	cInfo.corecount = int(cCpuInfo.corecount)
	cInfo.logicalcount = int(cCpuInfo.logicalcount)
	cInfo.pkgcount = int(cCpuInfo.pkgcount)
	cInfo.numaNodeCount = int(cCpuInfo.numaNodeCount)
	cInfo.relationGroups = int(cCpuInfo.relationGroups)
	cInfo.maxProcsInGroups = int(cCpuInfo.maxProcsInGroups)
	cInfo.activeProcsInGroups = int(cCpuInfo.activeProcsInGroups)
	cInfo.l1CacheSize = uint64(cCpuInfo.l1CacheSize)
	cInfo.l2CacheSize = uint64(cCpuInfo.l2CacheSize)
	cInfo.l3CacheSize = uint64(cCpuInfo.l3CacheSize)

	// Get additional CPU information from Windows registry
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		registryHive,
		registry.QUERY_VALUE)
	if err != nil {
		log.Errorf("failed to open registry key: %v", err)
		return &Info{}
	}
	defer k.Close()

	if mhz, _, err := k.GetIntegerValue("~MHz"); err == nil {
		cInfo.Mhz = float64(mhz)
	}

	if name, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
		cInfo.ModelName = name
	}

	if vendor, _, err := k.GetStringValue("VendorIdentifier"); err == nil {
		cInfo.VendorID = vendor
	}

	if identifier, _, err := k.GetStringValue("Identifier"); err == nil {
		cInfo.Family = extract(identifier, "Family")
	}

	// Get system info for model and stepping
	var si systemInfo
	var mod = windows.NewLazyDLL("kernel32.dll")
	var gsi = mod.NewProc("GetSystemInfo")
	//nolint:errcheck
	gsi.Call(uintptr(unsafe.Pointer(&si)))

	cInfo.Model = strconv.Itoa(int((si.wProcessorRevision >> 8) & 0xFF))
	cInfo.Stepping = strconv.Itoa(int(si.wProcessorRevision & 0xFF))

	// Convert to Info struct
	info := &Info{
		VendorID:             utils.NewValue(cInfo.VendorID),
		ModelName:            utils.NewValue(cInfo.ModelName),
		CPUCores:             utils.NewValue(uint64(cInfo.corecount)),
		CPULogicalProcessors: utils.NewValue(uint64(cInfo.logicalcount)),
		Mhz:                  utils.NewValue(cInfo.Mhz),
		Family:               utils.NewValue(cInfo.Family),
		Model:                utils.NewValue(cInfo.Model),
		Stepping:             utils.NewValue(cInfo.Stepping),
		CPUPkgs:              utils.NewValue(uint64(cInfo.pkgcount)),
		CPUNumaNodes:         utils.NewValue(uint64(cInfo.numaNodeCount)),
		CacheSizeL1Bytes:     utils.NewValue(cInfo.l1CacheSize),
		CacheSizeL2Bytes:     utils.NewValue(cInfo.l2CacheSize),
		CacheSizeL3Bytes:     utils.NewValue(cInfo.l3CacheSize),
		CacheSizeKB:          utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	return info
}
