// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package procutil

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// NVMLProbe is a probe for GPU devices using NVML
type NVMLProbe struct {
	ctx  context.Context
	nvml nvml.Interface

	InfosByPid  map[int32]nvml.ProcessInfo
	DeviceByPid map[int32]nvml.Device
}

// NewGpuProbe creates a new GPU probe
func NewGpuProbe(config pkgconfigmodel.Reader) *NVMLProbe {
	nvml := nvml.New(nvml.WithLibraryPath(config.GetString("gpu_monitoring.nvml_lib_path")))
	log.Info("Created NVML probe")
	return &NVMLProbe{
		ctx:  context.Background(),
		nvml: nvml,
	}
}

// Scan scans the system for GPU devices
func (p *NVMLProbe) Scan() {
	log.Info("Scan begin")
	count, ret := p.nvml.DeviceGetCount()
	log.Infof("Finished DeviceGetCount count: %d, ret: %s", count, ret)
	if ret != nvml.SUCCESS {
		log.Errorf("Unable to get device count: %v", nvml.ErrorString(ret))
		return
	}

	infosByPid := make(map[int32]nvml.ProcessInfo)
	deviceByPid := make(map[int32]nvml.Device)
	for di := 0; di < count; di++ {
		device, ret := p.nvml.DeviceGetHandleByIndex(di)
		log.Infof("Finished DeviceGetHandleByIndex device: %d, ret: %s", device, ret)
		if ret != nvml.SUCCESS {
			log.Errorf("Unable to get device at index %d: %v", di, nvml.ErrorString(ret))
			return
		}

		processInfos, ret := device.GetComputeRunningProcesses()
		log.Infof("Finished GetComputeRunningProcesses processInfos: %d, ret: %s", processInfos, ret)
		if ret != nvml.SUCCESS {
			log.Errorf("Unable to get process info for device at index %d: %v", di, nvml.ErrorString(ret))
			return
		}
		fmt.Printf("Found %d processes on device %d\n", len(processInfos), di)

		for pi, processInfo := range processInfos {
			infosByPid[int32(pi)] = processInfo
			deviceByPid[int32(pi)] = device
		}
	}
	p.InfosByPid = infosByPid
	p.DeviceByPid = deviceByPid
	log.Info("Scan completed")
}

// Close closes the probe
func (p *NVMLProbe) Close() {
	log.Info("NVML Probe closing")
	_ = p.nvml.Shutdown()
}
