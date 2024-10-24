// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
)

// systemContext holds certain attributes about the system that are used by the GPU probe.
type systemContext struct {
	// maxGpuThreadsPerDevice maps each device index to the maximum number of threads it can run in parallel
	maxGpuThreadsPerDevice map[int]int

	// timeResolver allows to resolve kernel-time timestamps
	timeResolver *sectime.Resolver

	// nvmlLib is the NVML library used to query GPU devices
	nvmlLib nvml.Interface
}

func getSystemContext(nvmlLib nvml.Interface) (*systemContext, error) {
	ctx := &systemContext{
		maxGpuThreadsPerDevice: make(map[int]int),
		nvmlLib:                nvmlLib,
	}

	if err := ctx.queryDevices(); err != nil {
		return nil, fmt.Errorf("error querying devices: %w", err)
	}

	var err error
	ctx.timeResolver, err = sectime.NewResolver()
	if err != nil {
		return nil, fmt.Errorf("error creating time resolver: %w", err)
	}

	return ctx, nil
}

func (ctx *systemContext) queryDevices() error {
	devices, err := getGPUDevices(ctx.nvmlLib)
	if err != nil {
		return fmt.Errorf("error getting GPU devices: %w", err)
	}

	for i, device := range devices {
		maxThreads, err := getMaxThreadsForDevice(device)
		if err != nil {
			return fmt.Errorf("error getting max threads for device %s: %w", device, err)
		}

		ctx.maxGpuThreadsPerDevice[i] = maxThreads
	}

	return nil
}
