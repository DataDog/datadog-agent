// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/comp/core/config"
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

// this regex matches device names from NVML and extracts the GPU type. For example, from "nvidia_a100-80gb" it will extract "a100". The groups are as follows:
// 1. The optional prefix "nvidia" or "tesla" (T4 GPUs are named "tesla_t4" despite being NVIDIA GPUs)
// 2. The optional prefix "geforce_" which we ignore
// 3. The optional prefix "rtx_pro_" or "rtx_", which we use it as it's part of the GPU type
// 4. The GPU type, which is the next alphanumeric part of the device name. Anything behind it (such as the memory size or whether it's PCI or SXM) is ignored.
var gpuTypeRegex = regexp.MustCompile(`^(?:nvidia|tesla)_(?:geforce_)?(rtx_pro_|rtx_)?([a-z\d]+)`)
var gpuNameSeparatorRegex = regexp.MustCompile(`[^a-z\d]+`)

type collector struct {
	id                                 string
	catalog                            workloadmeta.AgentType
	store                              workloadmeta.Component
	seenUUIDs                          map[string]struct{}
	seenPIDsToGPUs                     map[int][]string // PID -> GPU UUIDs
	reportedDriverNotLoaded            bool
	integrateWithWorkloadmetaProcesses bool
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
		Vendor:  nvidiaVendor,
		Device:  devInfo.Name,
		GPUType: extractGPUType(devInfo.Name),
		Index:   devInfo.Index,
		ComputeCapability: workloadmeta.GPUComputeCapability{
			Major: int(devInfo.SMVersion / 10),
			Minor: int(devInfo.SMVersion % 10),
		},
		TotalCores:   devInfo.CoreCount,
		TotalMemory:  devInfo.Memory,
		Architecture: gpuArchToString(devInfo.Architecture),
	}

	switch d := device.(type) {
	case *ddnvml.PhysicalDevice:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypePhysical
		for _, child := range d.MIGChildren {
			gpuDeviceInfo.ChildrenGPUUUIDs = append(gpuDeviceInfo.ChildrenGPUUUIDs, child.GetDeviceInfo().UUID)
		}
	case *ddnvml.MIGDevice:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypeMIG
		if d.Parent != nil {
			gpuDeviceInfo.ParentGPUUUID = d.Parent.UUID
		}
	default:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypeUnknown
	}

	c.fillNVMLAttributes(&gpuDeviceInfo, device)
	c.fillProcesses(&gpuDeviceInfo, device)

	return &gpuDeviceInfo, nil
}

// fillNVMLAttributes fills the attributes of the GPU device by querying NVML API
func (c *collector) fillNVMLAttributes(gpuDeviceInfo *workloadmeta.GPU, device ddnvml.Device) {
	migDevice, isMig := device.(*ddnvml.MIGDevice)
	deviceForVirtMode := device
	if isMig {
		deviceForVirtMode = migDevice.Parent
	}

	virtMode, err := deviceForVirtMode.GetVirtualizationMode()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("cannot get virtualization mode: %v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.VirtualizationMode = gpuVirtModeToString(virtMode)
	}

	memBusWidth, err := device.GetMemoryBusWidth()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.MemoryBusWidth = memBusWidth
	}

	// Do not generate errors for vGPU devices, we already know that they don't support max clock info
	if virtMode != nvml.GPU_VIRTUALIZATION_MODE_VGPU {
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
	} else {
		if _, ok := c.seenUUIDs[gpuDeviceInfo.EntityID.ID]; !ok && logLimiter.ShouldLog() {
			// only report the warning once for each device
			log.Infof("vGPU device %s does not support queries for max clock info", gpuDeviceInfo.EntityID.ID)
		}
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

// newCollector creates a new collector with the default values, useful for testing.
func newCollector(store workloadmeta.Component, config config.Component) *collector {
	collector := &collector{
		id:             collectorID,
		catalog:        workloadmeta.NodeAgent,
		seenUUIDs:      map[string]struct{}{},
		seenPIDsToGPUs: make(map[int][]string),
		store:          store,
	}

	if config != nil {
		collector.integrateWithWorkloadmetaProcesses = config.GetBool("gpu.integrate_with_workloadmeta_processes")
	}

	return collector
}

// NewCollector returns a kubelet CollectorProvider that instantiates its collector
func NewCollector(config config.Component) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: newCollector(nil, config),
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
func (c *collector) Pull(ctx context.Context) error {
	lib, err := ddnvml.GetSafeNvmlLib()
	if err != nil {
		// Do not consider an unloaded driver as an error more than once.
		// Some installations will have the NVIDIA libraries but not the driver. Report the error
		// only once to avoid log spam, treat it the same as if there was no library available or
		// there were no GPUs.
		if ddnvml.IsDriverNotLoaded(err) && !c.reportedDriverNotLoaded {
			c.reportedDriverNotLoaded = true
			return nil
		}

		return fmt.Errorf("failed to get NVML library : %w", err)
	}

	deviceCache := ddnvml.NewDeviceCache(ddnvml.WithDeviceCacheLib(lib))
	if err := deviceCache.Refresh(); err != nil {
		return fmt.Errorf("failed to initialize device cache: %w", err)
	}

	// driver version is equal to all devices of the same vendor
	// currently we handle only nvidia.
	// in the future this function should be refactored to support more vendors
	driverVersion, err := lib.SystemGetDriverVersion()
	// we try to get the driver version as best effort, just log warning if it fails
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v", err)
		}
	}

	// attempt getting list of unhealthy devices (if available)
	unhealthyDevices, err := c.getUnhealthyDevices(ctx)
	if err != nil && logLimiter.ShouldLog() {
		log.Warnf("failed getting unhealthy devices: %v", err)
	}

	// note: the device list can change over time so we need to set/unset for reconciliation
	allDevices, err := deviceCache.All()
	if err != nil {
		// Should not happen as we check the last init error for the library
		return fmt.Errorf("failed to get all devices: %w", err)
	}

	// add/update current devices
	currentUUIDs := map[string]struct{}{}
	pidToGPUs := make(map[int][]string) // PID -> GPU UUIDs
	var events []workloadmeta.CollectorEvent
	for _, dev := range allDevices {
		gpu, err := c.getGPUDeviceInfo(dev)
		if err != nil {
			return err
		}

		gpu.DriverVersion = driverVersion

		_, unhealthy := unhealthyDevices[gpu.ID]
		gpu.Healthy = !unhealthy

		uuid := dev.GetDeviceInfo().UUID
		currentUUIDs[uuid] = struct{}{}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeSet,
			Entity: gpu,
		})

		if c.integrateWithWorkloadmetaProcesses {
			for _, pid := range gpu.ActivePIDs {
				pidToGPUs[pid] = append(pidToGPUs[pid], uuid)
			}
		}
	}

	// remove previous devices that are no more available
	for uuid := range c.seenUUIDs {
		if _, ok := currentUUIDs[uuid]; ok {
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.GPU{
				EntityID: workloadmeta.EntityID{
					ID:   uuid,
					Kind: workloadmeta.KindGPU,
				},
			},
		})
	}

	c.seenUUIDs = currentUUIDs

	if c.integrateWithWorkloadmetaProcesses {
		events = append(events, c.createProcessEvents(pidToGPUs)...)
	}

	c.store.Notify(events)

	return nil
}

func (c *collector) createProcessEvents(pidToGPUs map[int][]string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(pidToGPUs))

	// Create events for active processes
	for pid, uuids := range pidToGPUs {
		var gpuEntityIDs []workloadmeta.EntityID
		for _, uuid := range uuids {
			gpuEntityIDs = append(gpuEntityIDs, workloadmeta.EntityID{
				Kind: workloadmeta.KindGPU,
				ID:   uuid,
			})
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindProcess,
					ID:   strconv.Itoa(int(pid)),
				},
				Pid:  int32(pid),
				GPUs: gpuEntityIDs,
			},
		})
	}

	// Remove inactive processes. Because we use SourceNVML for the Process entities, workloadmeta
	// will not remove the process if it has been added by another source.
	for pid := range c.seenPIDsToGPUs {
		if _, stillActive := pidToGPUs[pid]; stillActive {
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindProcess,
					ID:   strconv.Itoa(int(pid)),
				},
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

func gpuArchToString(nvmlArch nvml.DeviceArchitecture) string {
	switch nvmlArch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "kepler"
	case nvml.DEVICE_ARCH_MAXWELL:
		return "maxwell"
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
	case 10: // nvml.DEVICE_ARCH_BLACKWELL in newer versions of go-nvml
		// note: we hardcode the enum to avoid updating to an untested newer go-nvml version
		return "blackwell"
	case nvml.DEVICE_ARCH_UNKNOWN:
		return "unknown"
	default:
		// Distinguish invalid and unknown, NVML can return unknown but we should always
		// be able to process the return value of NVML. If we reach this part, we forgot
		// to add a new case for a new architecture.
		return "invalid"
	}
}

func extractGPUType(deviceName string) string {
	if deviceName == "" {
		return ""
	}

	// Normalize case/whitespace and remove leading/trailing noise so regex matching is stable.
	normalizedName := strings.ToLower(strings.TrimSpace(deviceName))
	// Collapse any non-alphanumeric separators (spaces, dashes, quotes, punctuation) into underscores.
	normalizedName = gpuNameSeparatorRegex.ReplaceAllString(normalizedName, "_")
	// Trim underscores added by leading/trailing separators.
	normalizedName = strings.Trim(normalizedName, "_")

	// Extract the optional RTX prefix and the GPU model token.
	matches := gpuTypeRegex.FindStringSubmatch(normalizedName)
	if len(matches) == 0 {
		return ""
	}

	// Combine optional RTX prefix with the model token (e.g., rtx_3090).
	return matches[1] + matches[2]
}

func gpuVirtModeToString(nvmlVirtMode nvml.GpuVirtualizationMode) string {
	switch nvmlVirtMode {
	case nvml.GPU_VIRTUALIZATION_MODE_NONE:
		return "none"
	case nvml.GPU_VIRTUALIZATION_MODE_HOST_VGPU:
		return "host_vgpu"
	case nvml.GPU_VIRTUALIZATION_MODE_PASSTHROUGH:
		return "passthrough"
	case nvml.GPU_VIRTUALIZATION_MODE_HOST_VSGA:
		return "host_vsga"
	case nvml.GPU_VIRTUALIZATION_MODE_VGPU:
		return "vgpu"
	default:
		return "unknown"
	}
}
