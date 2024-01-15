// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux && arm64

package cpu

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// The Linux kernel does not include much useful information in /proc/cpuinfo
// for arm64, so we must dig further into the /sys tree and build a more
// accurate representation of the contained data, rather than relying on the
// simple analysis in cpu/cpu_linux_default.go.

// nodeNRegex recognizes directories named `nodeNN`
var nodeNRegex = regexp.MustCompile("^node[0-9]+$")

func (cpuInfo *Info) fillProcCPUErr(err error) {
	cpuInfo.VendorID = utils.NewErrorValue[string](err)
	cpuInfo.ModelName = utils.NewErrorValue[string](err)
	cpuInfo.CPUCores = utils.NewErrorValue[uint64](err)
	cpuInfo.CPULogicalProcessors = utils.NewErrorValue[uint64](err)
	cpuInfo.CacheSizeKB = utils.NewErrorValue[uint64](err)
	cpuInfo.Family = utils.NewErrorValue[string](err)
	cpuInfo.Model = utils.NewErrorValue[string](err)
	cpuInfo.Stepping = utils.NewErrorValue[string](err)
	cpuInfo.CPUPkgs = utils.NewErrorValue[uint64](err)
	cpuInfo.CPUNumaNodes = utils.NewErrorValue[uint64](err)
	cpuInfo.CacheSizeL1Bytes = utils.NewErrorValue[uint64](err)
	cpuInfo.CacheSizeL2Bytes = utils.NewErrorValue[uint64](err)
	cpuInfo.CacheSizeL3Bytes = utils.NewErrorValue[uint64](err)
}

func (cpuInfo *Info) fillFirstCPUInfo(firstCPU map[string]string) {
	// determine vendor and model from CPU implementer / part
	if cpuVariantStr, ok := firstCPU["CPU implementer"]; ok {
		if cpuVariant, err := strconv.ParseUint(cpuVariantStr, 0, 64); err == nil {
			if cpuPartStr, ok := firstCPU["CPU part"]; ok {
				if cpuPart, err := strconv.ParseUint(cpuPartStr, 0, 64); err == nil {
					cpuInfo.Model = utils.NewValue(cpuPartStr)
					if impl, ok := hwVariant[cpuVariant]; ok {
						cpuInfo.VendorID = utils.NewValue(impl.name)
						if modelName, ok := impl.parts[cpuPart]; ok {
							cpuInfo.ModelName = utils.NewValue(modelName)
						} else {
							cpuInfo.ModelName = utils.NewValue(cpuPartStr)
						}
					} else {
						cpuInfo.VendorID = utils.NewValue(cpuVariantStr)
						cpuInfo.ModelName = utils.NewValue(cpuPartStr)
					}
				}
			}
		}
	}

	// 'lscpu' represents the stepping as an rXpY string
	if cpuVariantStr, ok := firstCPU["CPU variant"]; ok {
		if cpuVariant, err := strconv.ParseUint(cpuVariantStr, 0, 64); err == nil {
			if cpuRevisionStr, ok := firstCPU["CPU revision"]; ok {
				if cpuRevision, err := strconv.ParseUint(cpuRevisionStr, 0, 64); err == nil {
					cpuInfo.Stepping = utils.NewValue(fmt.Sprintf("r%dp%d", cpuVariant, cpuRevision))
				}
			}
		}
	}
}

func (cpuInfo *Info) fillProcCPUInfo() {
	procCPU, err := readProcCPUInfo()
	if err != nil {
		cpuInfo.fillProcCPUErr(err)
		return
	}

	// initialize each field collected from /proc/cpuinfo with a default error
	cpuInfo.fillProcCPUErr(errors.New("not found in /proc/cpuinfo"))

	// we blithely assume that many of the CPU characteristics are the same for
	// all CPUs, so we can just use the first.
	cpuInfo.fillFirstCPUInfo(procCPU[0])

	// Iterate over each processor and fetch additional information from /sys/devices/system/cpu
	cores := map[uint64]struct{}{}
	packages := map[uint64]struct{}{}
	cacheSizes := map[uint64]uint64{}
	for _, stanza := range procCPU {
		procID, err := strconv.ParseUint(stanza["processor"], 0, 64)
		if err != nil {
			continue
		}

		if coreID, ok := sysCPUInt(fmt.Sprintf("cpu%d/topology/core_id", procID)); ok {
			cores[coreID] = struct{}{}
		}

		if pkgID, ok := sysCPUInt(fmt.Sprintf("cpu%d/topology/physical_package_id", procID)); ok {
			packages[pkgID] = struct{}{}
		}

		// iterate over each cache this CPU can use
		i := 0
		for {
			if sharedList, ok := sysCPUList(fmt.Sprintf("cpu%d/cache/index%d/shared_cpu_list", procID, i)); ok {
				// we are scanning CPUs in order, so only count this cache if it's not shared with a
				// CPU that has already been scanned
				shared := false
				for sharedProcID := range sharedList {
					if sharedProcID < procID {
						shared = true
						break
					}
				}

				if !shared {
					if level, ok := sysCPUInt(fmt.Sprintf("cpu%d/cache/index%d/level", procID, i)); ok {
						if size, ok := sysCPUSize(fmt.Sprintf("cpu%d/cache/index%d/size", procID, i)); ok {
							cacheSizes[level] += size
						}
					}
				}
			} else {
				break
			}
			i++
		}
	}
	cpuInfo.CPUPkgs = utils.NewValue(uint64(len(packages)))
	cpuInfo.CPUCores = utils.NewValue(uint64(len(cores)))
	cpuInfo.CPULogicalProcessors = utils.NewValue(uint64(len(procCPU)))
	cpuInfo.CacheSizeL1Bytes = utils.NewValue(cacheSizes[1])
	cpuInfo.CacheSizeL2Bytes = utils.NewValue(cacheSizes[2])
	cpuInfo.CacheSizeL3Bytes = utils.NewValue(cacheSizes[3])

	// compute total cache size in KB
	cacheSize := (cacheSizes[1] + cacheSizes[2] + cacheSizes[3]) / 1024
	cpuInfo.CacheSizeKB = utils.NewValue(cacheSize)
}

func getCPUInfo() *Info {
	cpuInfo := &Info{
		// ARM does not make the clock speed available
		Mhz: utils.NewErrorValue[float64](utils.ErrNotCollectable),
	}

	cpuInfo.fillProcCPUInfo()

	// ARM does not define a family
	cpuInfo.Family = utils.NewValue("none")

	// Count the number of NUMA nodes in /sys/devices/system/node
	if dirents, err := os.ReadDir("/sys/devices/system/node"); err == nil {
		nodes := uint64(0)
		for _, dirent := range dirents {
			if dirent.IsDir() && nodeNRegex.MatchString(dirent.Name()) {
				nodes++
			}
		}
		cpuInfo.CPUNumaNodes = utils.NewValue(nodes)
	} else {
		cpuInfo.CPUNumaNodes = utils.NewErrorValue[uint64](fmt.Errorf("could not read /sys/devices/system/node: %w", err))
	}

	return cpuInfo
}
