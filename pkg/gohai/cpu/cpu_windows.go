// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

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

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/registry"
)

const registryHive = "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0"

// cpuInfo holds CPU information
type cpuInfo struct {
	corecount           int     // total number of cores
	logicalcount        int     // number of logical CPUS
	pkgcount            int     // number of packages (physical CPUS)
	numaNodeCount       int     // number of NUMA nodes
	relationGroups      int     // number of relation groups
	maxProcsInGroups    int     // maximum number of processors in a relation group
	activeProcsInGroups int     // active processors in a relation group
	l1CacheSize         uint64  // layer 1 cache size
	l2CacheSize         uint64  // layer 2 cache size
	l3CacheSize         uint64  // layer 3 cache size
	vendorID            string  // vendor ID
	modelName           string  // model name
	clockMhz            float64 // CPU clock speed
	family              string  // CPU family
	model               string  // CPU model
	stepping            string  // CPU stepping
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
	var cCPUInfo C.CPU_INFO

	ret := C.computeCoresAndProcessors(&cCPUInfo)
	if ret != 0 {
		log.Errorf("failed to get CPU information, error code: %d", ret)
		return &Info{}
	}

	// Copy C struct values to Go struct
	cInfo.corecount = int(cCPUInfo.corecount)
	cInfo.logicalcount = int(cCPUInfo.logicalcount)
	cInfo.pkgcount = int(cCPUInfo.pkgcount)
	cInfo.numaNodeCount = int(cCPUInfo.numaNodeCount)
	cInfo.relationGroups = int(cCPUInfo.relationGroups)
	cInfo.maxProcsInGroups = int(cCPUInfo.maxProcsInGroups)
	cInfo.activeProcsInGroups = int(cCPUInfo.activeProcsInGroups)
	cInfo.l1CacheSize = uint64(cCPUInfo.l1CacheSize)
	cInfo.l2CacheSize = uint64(cCPUInfo.l2CacheSize)
	cInfo.l3CacheSize = uint64(cCPUInfo.l3CacheSize)

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
		cInfo.clockMhz = float64(mhz)
	} else {
		log.Errorf("failed to get MHz from registry: %v", err)
	}

	if name, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
		cInfo.modelName = name
	} else {
		log.Errorf("failed to get ModelName from registry: %v", err)
	}

	if vendor, _, err := k.GetStringValue("VendorIdentifier"); err == nil {
		cInfo.vendorID = vendor
	} else {
		log.Errorf("failed to get VendorIdentifier from registry: %v", err)
	}

	if identifier, _, err := k.GetStringValue("Identifier"); err == nil {
		cInfo.family = extract(identifier, "Family")
	} else {
		log.Errorf("failed to get Identifier from registry: %v", err)
	}

	// Get system info for model and stepping
	var cSysInfo C.SYSTEM_INFO
	ret = C.getSystemInfo(&cSysInfo)
	if ret != 0 {
		log.Errorf("failed to get system information, error code: %d", ret)
		return &Info{}
	}

	cInfo.model = strconv.Itoa(int((cSysInfo.wProcessorRevision >> 8) & 0xFF))
	cInfo.stepping = strconv.Itoa(int(cSysInfo.wProcessorRevision & 0xFF))

	// Convert to Info struct
	info := &Info{
		VendorID:             utils.NewValue(cInfo.vendorID),
		ModelName:            utils.NewValue(cInfo.modelName),
		CPUCores:             utils.NewValue(uint64(cInfo.corecount)),
		CPULogicalProcessors: utils.NewValue(uint64(cInfo.logicalcount)),
		Mhz:                  utils.NewValue(cInfo.clockMhz),
		Family:               utils.NewValue(cInfo.family),
		Model:                utils.NewValue(cInfo.model),
		Stepping:             utils.NewValue(cInfo.stepping),
		CPUPkgs:              utils.NewValue(uint64(cInfo.pkgcount)),
		CPUNumaNodes:         utils.NewValue(uint64(cInfo.numaNodeCount)),
		CacheSizeL1Bytes:     utils.NewValue(cInfo.l1CacheSize),
		CacheSizeL2Bytes:     utils.NewValue(cInfo.l2CacheSize),
		CacheSizeL3Bytes:     utils.NewValue(cInfo.l3CacheSize),
		CacheSizeKB:          utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	return info
}
