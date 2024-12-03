// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package procutil

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// NVMLProbe is a probe for GPU devices using NVML
type NVMLProbe struct {
	nvml nvml.Interface

	DeviceUUIDByPid map[int32]string
}

// NewGpuProbe creates a new GPU probe
func NewGpuProbe(config pkgconfigmodel.Reader) *NVMLProbe {
	nvmlLib := nvml.New(nvml.WithLibraryPath(config.GetString("gpu_monitoring.nvml_lib_path")))
	ret := nvmlLib.Init()
	if ret != nvml.SUCCESS {
		log.Errorf("failed to initialize NVML library: %s", nvml.ErrorString(ret))
		return nil
	}

	log.Info("Created NVML probe")
	return &NVMLProbe{
		nvml:            nvmlLib,
		DeviceUUIDByPid: make(map[int32]string),
	}
}

// Scan scans the system for GPU devices
func (p *NVMLProbe) Scan() {
	if p == nil {
		log.Error("NVML Probe is nil")
		return
	}

	log.Info("Scan begin")
	count, ret := p.nvml.DeviceGetCount()
	log.Infof("Finished DeviceGetCount count: %d, ret: %s", count, ret)
	if ret != nvml.SUCCESS {
		log.Errorf("Unable to get device count: %v", nvml.ErrorString(ret))
		return
	}

	deviceUUIDByPid := make(map[int32]string)
	for di := 0; di < count; di++ {
		device, ret := p.nvml.DeviceGetHandleByIndex(di)
		log.Infof("Finished DeviceGetHandleByIndex device: %d, ret: %s", device, ret)
		if ret != nvml.SUCCESS {
			log.Errorf("Unable to get device at index %d: %v", di, nvml.ErrorString(ret))
			return
		}

		deviceUUID, err := device.GetUUID()
		if !errors.Is(err, nvml.SUCCESS) {
			log.Warn("Failed to get GPU UUID %v", err)
		}

		processInfos, ret := device.GetComputeRunningProcesses()
		log.Infof("Finished GetComputeRunningProcesses processInfos: %d, ret: %s", processInfos, ret)
		if ret != nvml.SUCCESS {
			log.Errorf("Unable to get process info for device at index %d: %v", di, nvml.ErrorString(ret))
			return
		}
		log.Infof("Found %d processes on device %d\n", len(processInfos), di)

		for _, processInfo := range processInfos {
			log.Infof("Found pid %d on device %s\n", processInfo.Pid, deviceUUID)
			deviceUUIDByPid[int32(processInfo.Pid)] = deviceUUID
		}
	}
	p.DeviceUUIDByPid = deviceUUIDByPid
	log.Info("Scan completed")
}

// Close closes the probe
func (p *NVMLProbe) Close() {
	log.Info("NVML Probe closing")
	_ = p.nvml.Shutdown()
}
