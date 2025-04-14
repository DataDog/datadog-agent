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
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
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

// getMigProfileName() returns the canonical name of the MIG device
func getMigProfileName(attr nvml.DeviceAttributes) (string, error) {
	g := attr.GpuInstanceSliceCount
	gb := (attr.MemorySizeMB + 1024 - 1) / 1024
	r := fmt.Sprintf("%dg.%dgb", g, gb)
	return r, nil
}

func (c *collector) getMigDeviceInfo(device nvml.Device) (string, string, error) {
	uuid, ret := device.GetUUID()
	if ret != nvml.SUCCESS {
		return "", "", fmt.Errorf("failed to get device UUID: %v", nvml.ErrorString(ret))
	}
	name, ret := device.GetName()
	if ret != nvml.SUCCESS {
		return "", "", fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
	}
	return uuid, name, nil
}

func (c *collector) getDeviceInfoMig(migDevice nvml.Device) (*workloadmeta.MigDevice, error) {
	uuid, name, err := c.getMigDeviceInfo(migDevice)
	if err != nil {
		return nil, err
	}

	gpuInstanceID, ret := migDevice.GetGpuInstanceId()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get GPU instance ID: %v", nvml.ErrorString(ret))
	}
	attr, ret := migDevice.GetAttributes()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device attributes: %v", nvml.ErrorString(ret))
	}
	canonoicalName, _ := getMigProfileName(attr)
	return &workloadmeta.MigDevice{
		GPUInstanceID:         gpuInstanceID,
		UUID:                  uuid,
		Name:                  name,
		GPUInstanceSliceCount: attr.GpuInstanceSliceCount,
		MemorySizeMB:          attr.MemorySizeMB,
		ResourceName:          canonoicalName,
	}, nil
}

func (c *collector) getGPUDeviceInfo(device *ddnvml.Device) (*workloadmeta.GPU, error) {
	gpuDeviceInfo := workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   device.UUID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: device.Name,
		},
		Vendor:     nvidiaVendor,
		Device:     device.Name,
		Index:      device.Index,
		MigEnabled: false,
		MigDevices: nil,
	}

	c.fillAttributes(&gpuDeviceInfo, device)
	c.fillMIGData(&gpuDeviceInfo, device.NVMLDevice)
	c.fillProcesses(&gpuDeviceInfo, device.NVMLDevice)

	return &gpuDeviceInfo, nil
}

func (c *collector) fillMIGData(gpuDeviceInfo *workloadmeta.GPU, device nvml.Device) {
	migEnabled, _, ret := device.GetMigMode()
	if ret != nvml.SUCCESS || migEnabled != nvml.DEVICE_MIG_ENABLE {
		return
	}
	// If any MIG detection fails, we will return mig disabled in config
	migDeviceCount, ret := device.GetMaxMigDeviceCount()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get MIG capable device count for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
		return
	}

	migDevs := make([]*workloadmeta.MigDevice, 0, migDeviceCount)
	for j := 0; j < migDeviceCount; j++ {
		migDevice, ret := device.GetMigDeviceHandleByIndex(j)
		if ret != nvml.SUCCESS {
			if logLimiter.ShouldLog() {
				log.Warnf("failed to get handle for MIG device %d: %v", j, nvml.ErrorString(ret))
			}
			continue
		}
		migDeviceInfo, err := c.getDeviceInfoMig(migDevice)
		if err != nil {
			if logLimiter.ShouldLog() {
				log.Warnf("failed to get device info for MIG device %d: %v", j, err)
			}
		} else {
			migDevs = append(migDevs, migDeviceInfo)
		}
	}

	gpuDeviceInfo.MigEnabled = true
	gpuDeviceInfo.MigDevices = migDevs
}

func (c *collector) fillAttributes(gpuDeviceInfo *workloadmeta.GPU, device *ddnvml.Device) {
	arch, ret := device.NVMLDevice.GetArchitecture()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get architecture for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.Architecture = gpuArchToString(arch)
	}

	major, minor, ret := device.NVMLDevice.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get CUDA compute capability for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.ComputeCapability.Major = major
		gpuDeviceInfo.ComputeCapability.Minor = minor
	}

	gpuDeviceInfo.TotalCores = device.CoreCount
	gpuDeviceInfo.TotalMemory = device.Memory

	memBusWidth, ret := device.NVMLDevice.GetMemoryBusWidth()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get device attributes for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.MemoryBusWidth = memBusWidth
	}

	maxSMClock, ret := device.NVMLDevice.GetMaxClockInfo(nvml.CLOCK_SM)
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get device attributes for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.MaxClockRates[workloadmeta.GPUSM] = maxSMClock
	}

	maxMemoryClock, ret := device.NVMLDevice.GetMaxClockInfo(nvml.CLOCK_MEM)
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get device attributes for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.MaxClockRates[workloadmeta.GPUMemory] = maxMemoryClock
	}
}

func (c *collector) fillProcesses(gpuDeviceInfo *workloadmeta.GPU, device nvml.Device) {
	procs, ret := device.GetComputeRunningProcesses()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get compute running processes for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
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
	lib, err := ddnvml.GetNvmlLib()
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
	driverVersion, ret := lib.SystemGetDriverVersion()
	//we try to get the driver version as best effort, just log warning if it fails
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get nvidia driver version: %v", nvml.ErrorString(ret))
		}
	}

	var events []workloadmeta.CollectorEvent
	for i := 0; i < deviceCache.Count(); i++ {
		dev, err := deviceCache.GetByIndex(i)
		if err != nil {
			return err
		}

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
