// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/fx"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "nvml"
	componentName = "workloadmeta-nvml"
	nvidiaVendor  = "nvidia"
)

var logLimiter = log.NewLogLimit(20, 10*time.Minute)

type collector struct {
	id      string
	catalog workloadmeta.AgentType
	store   workloadmeta.Component
}

func (c *collector) getGPUDeviceInfo(device ddnvml.Device) (*workloadmeta.GPU, error) {
	// build the GPU device info using the pre-computed values
	// from the device cache
	devInfo := device.GetDeviceInfo()
	gpuDeviceInfo := workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   devInfo.UUID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: devInfo.Name,
		},
		Vendor: nvidiaVendor,
		Device: devInfo.Name,
		Index:  devInfo.Index,
		ComputeCapability: workloadmeta.GPUComputeCapability{
			Major: int(devInfo.SMVersion / 10),
			Minor: int(devInfo.SMVersion % 10),
		},
		TotalCores:  devInfo.CoreCount,
		TotalMemory: devInfo.Memory,
	}

	switch device.(type) {
	case *ddnvml.PhysicalDevice:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypePhysical
	case *ddnvml.MIGDevice:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypeMIG
	default:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypeUnknown
	}

	c.fillNVMLAttributes(&gpuDeviceInfo, device)
	c.fillProcesses(&gpuDeviceInfo, device)

	return &gpuDeviceInfo, nil
}

// fillNVMLAttributes fills the attributes of the GPU device by querying NVML API
func (c *collector) fillNVMLAttributes(gpuDeviceInfo *workloadmeta.GPU, device ddnvml.Device) {
	arch, err := device.GetArchitecture()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.Architecture = gpuArchToString(arch)
	}

	memBusWidth, err := device.GetMemoryBusWidth()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.MemoryBusWidth = memBusWidth
	}

	maxSMClock, err := device.GetMaxClockInfo(nvml.CLOCK_SM)
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.MaxClockRates[workloadmeta.GPUSM] = maxSMClock
	}

	maxMemoryClock, err := device.GetMaxClockInfo(nvml.CLOCK_MEM)
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.MaxClockRates[workloadmeta.GPUMemory] = maxMemoryClock
	}
}

func (c *collector) fillProcesses(gpuDeviceInfo *workloadmeta.GPU, device ddnvml.Device) {
	procs, err := device.GetComputeRunningProcesses()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
		return
	}

	for _, proc := range procs {
		gpuDeviceInfo.ActivePIDs = append(gpuDeviceInfo.ActivePIDs, int(proc.Pid))
	}
}

// NewCollector returns a kubelet CollectorProvider that instantiates its collector
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			catalog: workloadmeta.NodeAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start initializes the NVML library and sets the store
func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.NVML) {
		return errors.NewDisabled(componentName, "Agent does not have NVML library available")
	}

	c.store = store

	return nil
}

// Pull collects the GPUs available on the node and notifies the store
func (c *collector) Pull(_ context.Context) error {
	lib, err := ddnvml.GetSafeNvmlLib()
	if err != nil {
		return fmt.Errorf("failed to get NVML library : %w", err)
	}
	deviceCache, err := ddnvml.NewDeviceCacheWithOptions(lib)
	if err != nil {
		return fmt.Errorf("failed to get GPU devices: %w", err)
	}

	// driver version is equal to all devices of the same vendor
	// currently we handle only nvidia.
	// in the future this function should be refactored to support more vendors
	driverVersion, err := lib.SystemGetDriverVersion()
	//we try to get the driver version as best effort, just log warning if it fails
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v", err)
		}
	}

	var events []workloadmeta.CollectorEvent
	for _, dev := range deviceCache.All() {
		gpu, err := c.getGPUDeviceInfo(dev)
		gpu.DriverVersion = driverVersion
		if err != nil {
			return err
		}

		event := workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: gpu,
		}
		events = append(events, event)
	}

	c.store.Notify(events)

	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func gpuArchToString(nvmlArch nvml.DeviceArchitecture) string {
	switch nvmlArch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "kepler"
	case nvml.DEVICE_ARCH_PASCAL:
		return "pascal"
	case nvml.DEVICE_ARCH_VOLTA:
		return "volta"
	case nvml.DEVICE_ARCH_TURING:
		return "turing"
	case nvml.DEVICE_ARCH_AMPERE:
		return "ampere"
	case nvml.DEVICE_ARCH_ADA:
		return "ada"
	case nvml.DEVICE_ARCH_HOPPER:
		return "hopper"
	case nvml.DEVICE_ARCH_UNKNOWN:
		return "unknown"
	default:
		// Distinguish invalid and unknown, NVML can return unknown but we should always
		// be able to process the return value of NVML. If we reach this part, we forgot
		// to add a new case for a new architecture.
		return "invalid"
	}

}
