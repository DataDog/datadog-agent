// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package amdgpu implements the workloadmeta collector for AMD GPUs, which
// reports GPU entities read from the amdgpu driver's sysfs attributes.
package amdgpu

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/amd"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "amdgpu"
	componentName = "workloadmeta-amdgpu"
)

// kfdDeniedLogInterval is how often the unreadable-KFD warning is repeated
// after the first one.
const kfdDeniedLogInterval = 10 * time.Minute

type collector struct {
	id                                 string
	catalog                            workloadmeta.AgentType
	store                              workloadmeta.Component
	sysRoot                            string
	enabled                            bool
	integrateWithWorkloadmetaProcesses bool
	seenUUIDs                          map[string]struct{}
	seenPIDsToGPUs                     map[int][]string // PID -> GPU UUIDs
	// kfdDeniedLogLimit rate-limits the warning about an unreadable KFD
	// topology: that condition does not change between pulls.
	kfdDeniedLogLimit *log.Limit
}

// newCollector creates a collector reading sysRoot. The GPU check and AMD
// collection must both be enabled for it to start.
func newCollector(cfg config.Component, sysRoot string) *collector {
	c := &collector{
		id:                collectorID,
		catalog:           workloadmeta.NodeAgent,
		sysRoot:           sysRoot,
		seenUUIDs:         make(map[string]struct{}),
		seenPIDsToGPUs:    make(map[int][]string),
		kfdDeniedLogLimit: log.NewLogLimit(1, kfdDeniedLogInterval),
	}
	if cfg != nil {
		c.enabled = cfg.GetBool("gpu.enabled") && cfg.GetBool("gpu.amd.enabled")
		c.integrateWithWorkloadmetaProcesses = cfg.GetBool("gpu.integrate_with_workloadmeta_processes")
	}
	return c
}

// NewCollector returns a CollectorProvider for the AMD GPU collector.
func NewCollector(cfg config.Component) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: newCollector(cfg, kernel.SysFSRoot()),
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start sets the store if AMD GPU collection is enabled.
func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !c.enabled {
		return dderrors.NewDisabled(componentName, "GPU monitoring or AMD GPU collection is disabled")
	}
	c.store = store
	return nil
}

// Pull discovers the AMD GPUs and the processes using them, and reconciles the
// store with the result. Partial results are published without retracting
// unobserved entities; the store handles error logging and pull telemetry.
func (c *collector) Pull(_ context.Context) error {
	devices, discoveryErr := amd.Discover(c.sysRoot)
	if msg := amd.KFDAccessDeniedWarning(devices); msg != "" && c.kfdDeniedLogLimit.ShouldLog() {
		log.Warn(msg)
	}

	pidToGPUs := make(map[int][]string)
	activePIDs := make(map[string][]int)
	usage, processesComplete, usageErr := amd.ReadProcessMemory(c.sysRoot, devices)
	for _, u := range usage {
		activePIDs[u.DeviceUUID] = append(activePIDs[u.DeviceUUID], u.PID)
		pidToGPUs[u.PID] = append(pidToGPUs[u.PID], u.DeviceUUID)
	}

	// An incomplete snapshot cannot establish that a device or process stopped
	// using a GPU. Keep the previous associations until a successful pull.
	processesComplete = processesComplete && discoveryErr == nil
	events := make([]workloadmeta.CollectorEvent, 0, len(devices))
	currentUUIDs := make(map[string]struct{}, len(devices))
	for _, dev := range devices {
		currentUUIDs[dev.UUID] = struct{}{}
		pids := activePIDs[dev.UUID]
		if !processesComplete {
			pids = nil
			if previous, err := c.store.GetGPU(dev.UUID); err == nil {
				pids = slices.Clone(previous.ActivePIDs)
			}
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceAMDGPU,
			Type:   workloadmeta.EventTypeSet,
			Entity: gpuEntity(dev, pids),
		})
	}
	for uuid := range c.seenUUIDs {
		if _, ok := currentUUIDs[uuid]; ok {
			continue
		}
		if discoveryErr != nil {
			currentUUIDs[uuid] = struct{}{}
			continue
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceAMDGPU,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.GPU{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: uuid}},
		})
	}
	c.seenUUIDs = currentUUIDs

	if c.integrateWithWorkloadmetaProcesses && processesComplete {
		events = append(events, c.processEvents(pidToGPUs)...)
	}

	c.store.Notify(events)
	return errors.Join(discoveryErr, usageErr)
}

// gpuEntity builds the workloadmeta entity of an AMD GPU. Fields without an
// AMD equivalent (NVLink, MIG, compute capability, core count) are left empty.
func gpuEntity(dev *amd.Device, pids []int) *workloadmeta.GPU {
	slices.Sort(pids)
	return &workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   dev.UUID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: dev.Name,
		},
		Vendor:        amd.Vendor,
		Device:        dev.Name,
		GPUType:       gpuutil.ExtractGPUType(dev.Name),
		DriverVersion: dev.DriverVersion,
		Index:         dev.Index,
		Architecture:  dev.Architecture,
		PCIBusID:      dev.PCIBusID,
		TotalMemory:   dev.MemoryTotal,
		MaxClockRates: [workloadmeta.GPUCOUNT]uint32{
			workloadmeta.GPUSM:     dev.MaxEngineClockMHz,
			workloadmeta.GPUMemory: dev.MaxMemoryClockMHz,
		},
		MemoryBusWidth: dev.MemoryBusWidthBits,
		DeviceType:     workloadmeta.GPUDeviceTypePhysical,
		ActivePIDs:     pids,
		// No AMD health/error-counter source is collected yet. As with NVML,
		// this means no unhealthy condition has been observed, not a health probe.
		Healthy: true,
	}
}

// processEvents sets the GPUs of the processes using AMD GPUs, and unsets
// them for processes that no longer do. Since the events use SourceAMDGPU,
// the process entities reported by other sources are not removed.
func (c *collector) processEvents(pidToGPUs map[int][]string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(pidToGPUs)+len(c.seenPIDsToGPUs))
	for pid, uuids := range pidToGPUs {
		gpus := make([]workloadmeta.EntityID, 0, len(uuids))
		for _, uuid := range uuids {
			gpus = append(gpus, workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: uuid})
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceAMDGPU,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: strconv.Itoa(pid)},
				Pid:      int32(pid),
				GPUs:     gpus,
			},
		})
	}
	for pid := range c.seenPIDsToGPUs {
		if _, active := pidToGPUs[pid]; active {
			continue
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceAMDGPU,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: strconv.Itoa(pid)},
			},
		})
	}
	c.seenPIDsToGPUs = pidToGPUs
	return events
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}
