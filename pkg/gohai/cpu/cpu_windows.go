// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/windows/registry"
)

const registryHive = "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0"

// see https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/nf-sysinfoapi-getlogicalprocessorinformationex
const (
	// RelationProcessorCore retrieves information about logical processors
	// that share a single processor core.
	RelationProcessorCore = 0
	// RelationNumaNode retrieves information about logical processors
	// that are part of the same NUMA node.
	RelationNumaNode = 1
	// RelationCache retrieves information about logical processors
	// that share a cache.
	RelationCache = 2
	// RelationProcessorPackage retrieves information about logical processors
	// that share a physical package.
	RelationProcessorPackage = 3
	// RelationGroup retrieves information about logical processors
	// that share a processor group.
	RelationGroup = 4
)

// SYSTEM_INFO contains information about the current computer system.
// This includes the architecture and type of the processor, the number
// of processors in the system, the page size, and other such information.
// see https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/ns-sysinfoapi-system_info
//
//nolint:revive
type SYSTEM_INFO struct {
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

// CPU_INFO contains information about cpu, eg. number of cores, cache size
//
//nolint:revive
type CPU_INFO struct {
	numaNodeCount int    // number of NUMA nodes
	pkgcount      int    // number of packages (physical CPUS)
	corecount     int    // total number of cores
	logicalcount  int    // number of logical CPUS
	l1CacheSize   uint32 // layer 1 cache size
	l2CacheSize   uint32 // layer 2 cache size
	l3CacheSize   uint32 // layer 3 cache size
	//nolint:unused
	relationGroups int // number of cpu relation groups
	//nolint:unused
	maxProcsInGroups int // max number of processors
	//nolint:unused
	activeProcsInGroups int // active processors
}

func countBits(num uint64) (count int) {
	count = 0
	for num > 0 {
		if (num & 0x1) == 1 {
			count++
		}
		num >>= 1
	}
	return
}

func getSystemInfo() (si SYSTEM_INFO) {
	var mod = windows.NewLazyDLL("kernel32.dll")
	var gsi = mod.NewProc("GetSystemInfo")

	// syscall does not fail
	//nolint:errcheck
	gsi.Call(uintptr(unsafe.Pointer(&si)))
	return
}

func getCPUInfo() *Info {
	cpuInfo := &Info{
		CacheSizeKB: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		registryHive,
		registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()

		dw, _, err := k.GetIntegerValue("~MHz")
		cpuInfo.Mhz = utils.NewValueFrom(float64(dw), err)

		s, _, err := k.GetStringValue("ProcessorNameString")
		cpuInfo.ModelName = utils.NewValueFrom(s, err)

		s, _, err = k.GetStringValue("VendorIdentifier")
		cpuInfo.VendorID = utils.NewValueFrom(s, err)

		s, _, err = k.GetStringValue("Identifier")
		if err == nil {
			cpuInfo.Family = utils.NewValue(extract(s, "Family"))
		} else {
			cpuInfo.Family = utils.NewErrorValue[string](err)
		}
	} else {
		cpuInfo.Mhz = utils.NewErrorValue[float64](err)
		cpuInfo.ModelName = utils.NewErrorValue[string](err)
		cpuInfo.VendorID = utils.NewErrorValue[string](err)
		cpuInfo.Family = utils.NewErrorValue[string](err)
	}

	cpus, cpuerr := computeCoresAndProcessors()
	cpuInfo.CPUPkgs = utils.NewValueFrom(uint64(cpus.pkgcount), cpuerr)
	cpuInfo.CPUNumaNodes = utils.NewValueFrom(uint64(cpus.numaNodeCount), cpuerr)
	cpuInfo.CPUCores = utils.NewValueFrom(uint64(cpus.corecount), cpuerr)
	cpuInfo.CPULogicalProcessors = utils.NewValueFrom(uint64(cpus.logicalcount), cpuerr)
	cpuInfo.CacheSizeL1Bytes = utils.NewValueFrom(uint64(cpus.l1CacheSize), cpuerr)
	cpuInfo.CacheSizeL2Bytes = utils.NewValueFrom(uint64(cpus.l2CacheSize), cpuerr)
	cpuInfo.CacheSizeL3Bytes = utils.NewValueFrom(uint64(cpus.l3CacheSize), cpuerr)

	si := getSystemInfo()
	cpuInfo.Model = utils.NewValue(strconv.Itoa(int((si.wProcessorRevision >> 8) & 0xFF)))
	cpuInfo.Stepping = utils.NewValue(strconv.Itoa(int(si.wProcessorRevision & 0xFF)))

	return cpuInfo
}

func extract(caption, field string) string {
	re := regexp.MustCompile(fmt.Sprintf("%s [0-9]* ", field))
	return strings.Split(re.FindStringSubmatch(caption)[0], " ")[1]
}
