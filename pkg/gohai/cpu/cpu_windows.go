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
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var getCPUInfo = GetCpuInfo

// ERROR_INSUFFICIENT_BUFFER is the error number associated with the
// "insufficient buffer size" error
//
//nolint:revive
const ERROR_INSUFFICIENT_BUFFER syscall.Errno = 122

const registryHive = "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0"

// CACHE_DESCRIPTOR contains cache related information
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-cache_descriptor
//
//nolint:unused,revive
type CACHE_DESCRIPTOR struct {
	Level         uint8
	Associativity uint8
	LineSize      uint16
	Size          uint32
	cacheType     uint32
}

// SYSTEM_LOGICAL_PROCESSOR_INFORMATION describes the relationship
// between the specified processor set.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-system_logical_processor_information
//
//nolint:unused,revive
type SYSTEM_LOGICAL_PROCESSOR_INFORMATION struct {
	ProcessorMask uintptr
	Relationship  int // enum (int)
	// in the Windows header, this is a union of a byte, a DWORD,
	// and a CACHE_DESCRIPTOR structure
	dataunion [16]byte
}

//.const SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE = 32

// GROUP_AFFINITY represents a processor group-specific affinity,
// such as the affinity of a thread.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-group_affinity
//
//nolint:revive
type GROUP_AFFINITY struct {
	Mask     uintptr
	Group    uint16
	Reserved [3]uint16
}

// NUMA_NODE_RELATIONSHIP represents information about a NUMA node
// in a processor group.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-numa_node_relationship
//
//nolint:revive
type NUMA_NODE_RELATIONSHIP struct {
	NodeNumber uint32
	Reserved   [20]uint8
	GroupMask  GROUP_AFFINITY
}

// CACHE_RELATIONSHIP describes cache attributes.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-cache_relationship
//
//nolint:revive
type CACHE_RELATIONSHIP struct {
	Level         uint8
	Associativity uint8
	LineSize      uint16
	CacheSize     uint32
	CacheType     int // enum in C
	Reserved      [20]uint8
	GroupMask     GROUP_AFFINITY
}

// PROCESSOR_GROUP_INFO represents the number and affinity of processors
// in a processor group.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-processor_group_info
//
//nolint:revive
type PROCESSOR_GROUP_INFO struct {
	MaximumProcessorCount uint8
	ActiveProcessorCount  uint8
	Reserved              [38]uint8
	ActiveProcessorMask   uintptr
}

// GROUP_RELATIONSHIP represents information about processor groups.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-group_relationship
//
//nolint:revive
type GROUP_RELATIONSHIP struct {
	MaximumGroupCount uint16
	ActiveGroupCount  uint16
	Reserved          [20]uint8
	// variable size array of PROCESSOR_GROUP_INFO
}

// PROCESSOR_RELATIONSHIP represents information about affinity
// within a processor group.
// see https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-processor_relationship
//
//nolint:unused,revive
type PROCESSOR_RELATIONSHIP struct {
	Flags           uint8
	EfficiencyClass uint8
	wReserved       [20]uint8
	GroupCount      uint16
	// what follows is an array of zero or more GROUP_AFFINITY structures
}

// SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX contains information about
// the relationships of logical processors and related hardware.
// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-system_logical_processor_information_ex
//
//nolint:revive
type SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX struct {
	Relationship int
	Size         uint32
	// what follows is a C union of
	// PROCESSOR_RELATIONSHIP,
	// NUMA_NODE_RELATIONSHIP,
	// CACHE_RELATIONSHIP,
	// GROUP_RELATIONSHIP
}

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
	numaNodeCount       int    // number of NUMA nodes
	pkgcount            int    // number of packages (physical CPUS)
	corecount           int    // total number of cores
	logicalcount        int    // number of logical CPUS
	l1CacheSize         uint32 // layer 1 cache size
	l2CacheSize         uint32 // layer 2 cache size
	l3CacheSize         uint32 // layer 3 cache size
	relationGroups      int    // number of cpu relation groups
	maxProcsInGroups    int    // max number of processors
	activeProcsInGroups int    // active processors

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
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var gsi = mod.NewProc("GetSystemInfo")

	// syscall does not fail
	//nolint:errcheck
	gsi.Call(uintptr(unsafe.Pointer(&si)))
	return
}

// GetCpuInfo returns map of interesting bits of information about the CPU
func GetCpuInfo() (cpuInfo map[string]string, err error) {
	cpuInfo = make(map[string]string)

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		registryHive,
		registry.QUERY_VALUE)
	if err == nil {
		// ignore registry key close errors
		//nolint:staticcheck
		defer k.Close()

		dw, _, err := k.GetIntegerValue("~MHz")
		if err == nil {
			cpuInfo["mhz"] = strconv.Itoa(int(dw))
		}

		s, _, err := k.GetStringValue("ProcessorNameString")
		if err == nil {
			cpuInfo["model_name"] = s
		}

		s, _, err = k.GetStringValue("VendorIdentifier")
		if err == nil {
			cpuInfo["vendor_id"] = s
		}

		s, _, err = k.GetStringValue("Identifier")
		if err == nil {
			cpuInfo["family"] = extract(s, "Family")
		}
	}

	cpus, err := computeCoresAndProcessors()
	if err == nil {
		cpuInfo["cpu_pkgs"] = strconv.Itoa(cpus.pkgcount)
		cpuInfo["cpu_numa_nodes"] = strconv.Itoa(cpus.numaNodeCount)
		cpuInfo["cpu_cores"] = strconv.Itoa(cpus.corecount)
		cpuInfo["cpu_logical_processors"] = strconv.Itoa(cpus.logicalcount)

		cpuInfo["cache_size_l1"] = strconv.Itoa(int(cpus.l1CacheSize))
		cpuInfo["cache_size_l2"] = strconv.Itoa(int(cpus.l2CacheSize))
		cpuInfo["cache_size_l3"] = strconv.Itoa(int(cpus.l3CacheSize))
	}

	si := getSystemInfo()
	cpuInfo["model"] = strconv.Itoa(int((si.wProcessorRevision >> 8) & 0xFF))
	cpuInfo["stepping"] = strconv.Itoa(int(si.wProcessorRevision & 0xFF))

	// cpuInfo cannot be empty
	err = nil

	return
}

func extract(caption, field string) string {
	re := regexp.MustCompile(fmt.Sprintf("%s [0-9]* ", field))
	return strings.Split(re.FindStringSubmatch(caption)[0], " ")[1]
}
