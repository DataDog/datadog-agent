// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux && !arm64

package cpu

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

func getCPUValueSetter(cpuInfo *Info) map[string]func(string, error) {
	// cache size has a ' KB' suffix so use a custom parser
	cacheSizeParse := func(value string) (uint64, error) {
		return strconv.ParseUint(strings.TrimSuffix(value, " KB"), 10, 64)
	}
	return map[string]func(string, error){
		"vendor_id":  utils.ValueStringSetter(&cpuInfo.VendorID),
		"model name": utils.ValueStringSetter(&cpuInfo.ModelName),
		"cpu cores":  utils.ValueParseInt64Setter(&cpuInfo.CPUCores),
		"siblings":   utils.ValueParseInt64Setter(&cpuInfo.CPULogicalProcessors),
		"cpu MHz":    utils.ValueParseFloat64Setter(&cpuInfo.Mhz),
		"cache size": utils.ValueParseSetter(&cpuInfo.CacheSizeKB, cacheSizeParse),
		"cpu family": utils.ValueStringSetter(&cpuInfo.Family),
		"model":      utils.ValueStringSetter(&cpuInfo.Model),
		"stepping":   utils.ValueStringSetter(&cpuInfo.Stepping),
	}
}

func getCPUInfo() *Info {
	return getCPUInfoWithReader(readProcFile)
}

func getCPUInfoWithReader(readProcFile func() ([]string, error)) *Info {
	cpuInfo := &Info{
		CPUPkgs:          utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUNumaNodes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL1Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL2Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL3Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	cpuMap := getCPUValueSetter(cpuInfo)
	lines, err := readProcFile()
	if err != nil {
		// store the error in each field
		for _, setter := range cpuMap {
			setter("", err)
		}
		return cpuInfo
	}

	// initialize each field with a 'key not found' error by default
	for key, setter := range cpuMap {
		setter("", fmt.Errorf("%s key not found in /proc/cpuInfo", strings.TrimSpace(key)))
	}

	// Implementation of a set that holds the physical IDs
	physicalProcIDs := make(map[string]struct{})

	for _, line := range lines {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if key == "physical id" {
			physicalProcIDs[value] = struct{}{}
		}

		setter, ok := cpuMap[key]
		if ok {
			setter(value, nil)
		}
	}

	// Values that need to be multiplied by the number of physical processors
	var perPhysicalProcValues = []*utils.Value[uint64]{
		&cpuInfo.CPUCores,
		&cpuInfo.CPULogicalProcessors,
	}

	nbPhysProcs := uint64(len(physicalProcIDs))
	// Multiply the values that are "per physical processor" by the number of physical procs
	for _, field := range perPhysicalProcValues {
		if value, err := field.Value(); err == nil {
			(*field) = utils.NewValue(value * nbPhysProcs)
		}
	}

	return cpuInfo
}

func readProcFile() (lines []string, err error) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if scanner.Err() != nil {
		err = scanner.Err()
		return
	}

	return
}
