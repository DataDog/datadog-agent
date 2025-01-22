// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvml

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
)

const (
	collectorID   = "nvml"
	componentName = "workloadmeta-nvml"
	nvidiaVendor  = "nvidia"
)

type collector struct {
	id      string
	catalog workloadmeta.AgentType
	store   workloadmeta.Component
	nvmlLib nvml.Interface
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
	// TODO: Add configuration option for NVML library path
	c.nvmlLib = nvml.New()
	ret := c.nvmlLib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("failed to initialize NVML library: %v", nvml.ErrorString(ret))
	}

	return nil
}

// Pull collects the GPUs available on the node and notifies the store
func (c *collector) Pull(_ context.Context) error {
	count, ret := c.nvmlLib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	var events []workloadmeta.CollectorEvent
	for i := 0; i < count; i++ {
		dev, ret := c.nvmlLib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device handle for index %d: %v", i, nvml.ErrorString(ret))
		}

		uuid, ret := dev.GetUUID()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device UUID for index %d: %v", i, nvml.ErrorString(ret))
		}

		name, ret := dev.GetName()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device name for index %d: %v", i, nvml.ErrorString(ret))
		}

		arch, ret := dev.GetArchitecture()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get architecture for device index %d: %v", i, nvml.ErrorString(ret))
		}

		major, minor, ret := dev.GetCudaComputeCapability()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get CUDA compute capability for device index %d: %v", i, nvml.ErrorString(ret))
		}

		devAttr, ret := dev.GetAttributes()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device attributes for device index %d: %v", i, nvml.ErrorString(ret))
		}

		gpu := &workloadmeta.GPU{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindGPU,
				ID:   uuid,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name: name,
			},
			Vendor:       nvidiaVendor,
			Device:       name,
			Index:        i,
			Architecture: gpuArchToString(arch),
			ComputeCapability: workloadmeta.GPUComputeCapability{
				Major: major,
				Minor: minor,
			},
			SMCount: int(devAttr.MultiprocessorCount),
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
