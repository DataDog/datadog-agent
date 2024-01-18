// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	model "github.com/DataDog/agent-payload/v5/process"
)

// CollectSystemInfo collects a set of system-level information that will not
// change until a restart. This bit of information should be passed along with
// the process messages.
func CollectSystemInfo() (*model.SystemInfo, error) {
	hi := platform.CollectInfo()
	cpuInfo := cpu.CollectInfo()
	mi, err := winutil.VirtualMemory()
	if err != nil {
		return nil, err
	}
	physCount, err := cpuInfo.CPUPkgs.Value()
	if err != nil {
		return nil, fmt.Errorf("gohai cpuInfo.CPUPkgs: %w", err)
	}

	// logicalcount will be the total number of logical processors on the system
	// i.e. physCount * coreCount * 1 if not HT CPU
	//      physCount * coreCount * 2 if an HT CPU.
	logicalCount := cpuInfo.CPULogicalProcessors.ValueOrDefault()

	// shouldn't be possible, as `cpuInfo.CPUPkgs.Value()` should return an error in this case
	// but double check before risking a divide by zero
	if physCount == 0 {
		return nil, fmt.Errorf("Returned zero physical processors")
	}
	logicalCountPerPhys := logicalCount / physCount
	clockSpeed := cpuInfo.Mhz.ValueOrDefault()
	l2Cache := cpuInfo.CacheSizeL2Bytes.ValueOrDefault()
	cpus := make([]*model.CPUInfo, 0)
	vendor := cpuInfo.VendorID.ValueOrDefault()
	family := cpuInfo.Family.ValueOrDefault()
	modelName := cpuInfo.Model.ValueOrDefault()
	for i := uint64(0); i < physCount; i++ {
		cpus = append(cpus, &model.CPUInfo{
			Cores: int32(logicalCountPerPhys),
		})
	}

	kernelName := hi.KernelName.ValueOrDefault()
	osName := hi.OS.ValueOrDefault()
	platformFamily := hi.Family.ValueOrDefault()
	kernelRelease := hi.KernelRelease.ValueOrDefault()
	m := &model.SystemInfo{
		Uuid: "",
		Os: &model.OSInfo{
			Name:          kernelName,
			Platform:      osName,
			Family:        platformFamily,
			Version:       kernelRelease,
			KernelVersion: "",
		},
		Cpus:        cpus,
		TotalMemory: int64(mi.Total),
	}
	return m, nil
}
