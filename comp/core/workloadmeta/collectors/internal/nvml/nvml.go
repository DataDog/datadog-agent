// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

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
	nvmlLib nvml.Interface
}

func (c *collector) getDeviceInfo(device nvml.Device) (string, string, error) {
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

// getMigProfileName() returns the canonical name of the MIG device
func getMigProfileName(attr nvml.DeviceAttributes) (string, error) {
	g := attr.GpuInstanceSliceCount
	gb := ((attr.MemorySizeMB + 1024 - 1) / 1024)
	r := fmt.Sprintf("%dg.%dgb", g, gb)
	return r, nil
}

func (c *collector) getDeviceInfoMig(migDevice nvml.Device) (*workloadmeta.MigDevice, error) {
	uuid, name, err := c.getDeviceInfo(migDevice)
	if err != nil {
		return nil, err
	}
	gpuInstanceID, ret := c.nvmlLib.DeviceGetGpuInstanceId(migDevice)
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

func (c *collector) getGPUdeviceInfo(device nvml.Device) (*workloadmeta.GPU, error) {
	uuid, name, err := c.getDeviceInfo(device)
	if err != nil {
		return nil, err
	}
	gpuIndexID, ret := c.nvmlLib.DeviceGetIndex(device)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get GPU index ID: %v", nvml.ErrorString(ret))
	}
	gpuDeviceInfo := workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   uuid,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: name,
		},
		Vendor:     nvidiaVendor,
		Device:     name,
		Index:      gpuIndexID,
		MigEnabled: false,
		MigDevices: nil,
	}

	c.fillMIGData(&gpuDeviceInfo, device)
	c.fillAttributes(&gpuDeviceInfo, device)
	c.fillProcesses(&gpuDeviceInfo, device)

	return &gpuDeviceInfo, nil
}

func (c *collector) fillMIGData(gpuDeviceInfo *workloadmeta.GPU, device nvml.Device) {
	migEnabled, _, ret := c.nvmlLib.DeviceGetMigMode(device)
	if ret != nvml.SUCCESS || migEnabled != nvml.DEVICE_MIG_ENABLE {
		return
	}
	// If any mid detection fails, we will return an mig disabled in config
	migDeviceCount, ret := c.nvmlLib.DeviceGetMaxMigDeviceCount(device)
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get MIG capable device count for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
		return
	}

	migDevs := make([]*workloadmeta.MigDevice, 0, migDeviceCount)
	for j := 0; j < migDeviceCount; j++ {
		migDevice, ret := c.nvmlLib.DeviceGetMigDeviceHandleByIndex(device, j)
		if ret != nvml.SUCCESS {
			if logLimiter.ShouldLog() {
				log.Warnf("failed to get handle for MIG device %d: %v", j, nvml.ErrorString(ret))
			}
			return
		}
		migDeviceInfo, err := c.getDeviceInfoMig(migDevice)
		if err != nil {
			if logLimiter.ShouldLog() {
				log.Warnf("failed to get device info for MIG device %d: %v", j, err)
			}
			return
		}
		migDevs = append(migDevs, migDeviceInfo)
	}

	gpuDeviceInfo.MigEnabled = true
	gpuDeviceInfo.MigDevices = migDevs
}

func (c *collector) fillAttributes(gpuDeviceInfo *workloadmeta.GPU, device nvml.Device) {
	arch, ret := device.GetArchitecture()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get architecture for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.Architecture = gpuArchToString(arch)
	}

	major, minor, ret := device.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get CUDA compute capability for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.ComputeCapability.Major = major
		gpuDeviceInfo.ComputeCapability.Minor = minor
	}

	devAttr, ret := device.GetAttributes()
	if ret != nvml.SUCCESS {
		if logLimiter.ShouldLog() {
			log.Warnf("failed to get device attributes for device index %d: %v", gpuDeviceInfo.Index, nvml.ErrorString(ret))
		}
	} else {
		gpuDeviceInfo.SMCount = int(devAttr.MultiprocessorCount)
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

func (c *collector) getNVML() (nvml.Interface, error) {
	if c.nvmlLib != nil {
		return c.nvmlLib, nil
	}

	// TODO: Add configuration option for NVML library path
	nvmlLib := nvml.New()
	ret := nvmlLib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return nil, fmt.Errorf("failed to initialize NVML library: %v", nvml.ErrorString(ret))
	}

	c.nvmlLib = nvmlLib
	return nvmlLib, nil
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
	nvmlLib, err := c.getNVML()
	if err != nil {
		return fmt.Errorf("failed to get NVML library: %w", err)
	}

	count, ret := nvmlLib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	var events []workloadmeta.CollectorEvent
	for i := 0; i < count; i++ {
		dev, ret := nvmlLib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device handle for index %d: %v", i, nvml.ErrorString(ret))
		}

		gpu, err := c.getGPUdeviceInfo(dev)
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
