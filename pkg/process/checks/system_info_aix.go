// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// CollectSystemInfo collects a set of system-level information that will not
// change until a restart. On AIX, fields that cannot be retrieved are left empty.
func CollectSystemInfo() (*model.SystemInfo, error) {
	hi, err := host.Info()
	if err != nil || hi == nil {
		hi = &host.InfoStat{}
	}

	cpuInfo, _ := cpu.Info()

	mi, err := mem.VirtualMemory()
	if err != nil || mi == nil {
		mi = &mem.VirtualMemoryStat{}
	}

	cpus := make([]*model.CPUInfo, 0, len(cpuInfo))
	for _, c := range cpuInfo {
		cpus = append(cpus, &model.CPUInfo{
			Cores: c.Cores,
		})
	}

	return &model.SystemInfo{
		Uuid: hi.HostID,
		Os: &model.OSInfo{
			Name:          hi.OS,
			Platform:      hi.Platform,
			Family:        hi.PlatformFamily,
			Version:       hi.PlatformVersion,
			KernelVersion: hi.KernelVersion,
		},
		Cpus:        cpus,
		TotalMemory: int64(mi.Total),
	}, nil
}
