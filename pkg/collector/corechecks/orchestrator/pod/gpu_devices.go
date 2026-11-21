// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package pod

import (
	"cmp"
	"slices"

	model "github.com/DataDog/agent-payload/v5/process"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// collectGPUDevices returns the node's GPUs as listed by the NVML workloadmeta
// collector. The collector runs with GPU Monitoring (gpu.enabled) or with
// orchestrator_explorer.gpu_devices.enabled, and lists nothing otherwise.
// Pod status reports the allocated device IDs: UUIDs for the NVIDIA device
// plugin, indexes (nvidia0/vgpu3) for GKE's. This list resolves both to the
// GPU's UUID, model and specs without GPU Feature Discovery.
func collectGPUDevices(store workloadmeta.Component) []*model.NodeGPUDevice {
	return gpuDevicesFromWorkloadmeta(store.ListGPUs())
}

func gpuDevicesFromWorkloadmeta(gpus []*workloadmeta.GPU) []*model.NodeGPUDevice {
	if len(gpus) == 0 {
		return nil
	}

	devices := make([]*model.NodeGPUDevice, 0, len(gpus))
	for _, g := range gpus {
		devices = append(devices, &model.NodeGPUDevice{
			Uuid:              g.ID,
			Minor:             int32(g.MinorNumber),
			Index:             int32(g.Index),
			Model:             g.Device,
			GpuType:           g.GPUType,
			Architecture:      g.Architecture,
			ComputeCapability: g.ComputeCapability.String(),
			TotalMemoryBytes:  g.TotalMemory,
			TotalCores:        int32(g.TotalCores),
			DriverVersion:     g.DriverVersion,
			PciBusId:          g.PCIBusID,
			DeviceType:        gpuDeviceType(g.DeviceType),
			ParentUuid:        g.ParentGPUUUID,
			MigProfile:        g.MIGProfile,
			SharingMode:       string(g.SharingMode),
			SharingReplicas:   int32(g.SharingReplicas),
		})
	}
	// Workloadmeta lists entities in map order: sort for stable payloads.
	slices.SortFunc(devices, func(a, b *model.NodeGPUDevice) int {
		return cmp.Compare(a.Uuid, b.Uuid)
	})
	return devices
}

func gpuDeviceType(t workloadmeta.GPUDeviceType) string {
	switch t {
	case workloadmeta.GPUDeviceTypePhysical:
		return "physical"
	case workloadmeta.GPUDeviceTypeMIG:
		return "mig"
	default:
		return "unknown"
	}
}
