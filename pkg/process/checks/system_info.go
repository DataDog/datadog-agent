// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

package checks

import (
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/host"
	"github.com/DataDog/gopsutil/mem"

	model "github.com/DataDog/agent-payload/v5/process"
)

// CollectSystemInfo collects a set of system-level information that will not
// change until a restart. This bit of information should be passed along with
// the process messages.
func CollectSystemInfo() (*model.SystemInfo, error) {
	hi, err := host.Info()
	if err != nil {
		return nil, err
	}
	cpuInfo, err := cpu.Info()
	if err != nil {
		return nil, err
	}
	mi, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	cpus := make([]*model.CPUInfo, 0, len(cpuInfo))
	for _, c := range cpuInfo {
		cpus = append(cpus, &model.CPUInfo{
			Number:     c.CPU,
			Vendor:     c.VendorID,
			Family:     c.Family,
			Model:      c.Model,
			PhysicalId: c.PhysicalID,
			CoreId:     c.CoreID,
			Cores:      c.Cores,
			Mhz:        int64(c.Mhz),
			CacheSize:  c.CacheSize,
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
