// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"strconv"

	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/platform"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	model "github.com/DataDog/agent-payload/v5/process"
)

// CollectSystemInfo collects a set of system-level information that will not
// change until a restart. This bit of information should be passed along with
// the process messages.
func CollectSystemInfo() (*model.SystemInfo, error) {
	hi, err := platform.GetArchInfo()
	if err != nil {
		return nil, err
	}
	cpuInfo, err := cpu.GetCpuInfo()
	if err != nil {
		return nil, err
	}
	mi, err := winutil.VirtualMemory()
	if err != nil {
		return nil, err
	}
	physCount, _ := strconv.ParseInt(cpuInfo["cpu_pkgs"], 10, 64)
	// logicalcount will be the total number of logical processors on the system
	// i.e. physCount * coreCount * 1 if not HT CPU
	//      physCount * coreCount * 2 if an HT CPU.
	logicalCount, _ := strconv.ParseInt(cpuInfo["cpu_logical_processors"], 10, 64)

	// shouldn't be possible, as `cpu.GetCpuInfo()` should return an error in this case
	// but double check before risking a divide by zero
	if physCount == 0 {
		return nil, fmt.Errorf("Returned zero physical processors")
	}
	logicalCountPerPhys := logicalCount / physCount
	clockSpeed, _ := strconv.ParseInt(cpuInfo["mhz"], 10, 64)
	l2Cache, _ := strconv.ParseInt(cpuInfo["cache_size_l2"], 10, 64)
	cpus := make([]*model.CPUInfo, 0)
	for i := int64(0); i < physCount; i++ {
		cpus = append(cpus, &model.CPUInfo{
			Number:     int32(i),
			Vendor:     cpuInfo["vendor_id"],
			Family:     cpuInfo["family"],
			Model:      cpuInfo["model"],
			PhysicalId: "",
			CoreId:     "",
			Cores:      int32(logicalCountPerPhys),
			Mhz:        int64(clockSpeed),
			CacheSize:  int32(l2Cache),
		})
	}

	m := &model.SystemInfo{
		Uuid: "",
		Os: &model.OSInfo{
			Name:          hi["kernel_name"],
			Platform:      hi["os"],
			Family:        hi["family"],
			Version:       hi["kernel_release"],
			KernelVersion: "",
		},
		Cpus:        cpus,
		TotalMemory: int64(mi.Total),
	}
	return m, nil
}
