// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver && test

package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestGPUDevicesFromWorkloadmeta(t *testing.T) {
	assert.Nil(t, gpuDevicesFromWorkloadmeta(nil))

	gpus := []*workloadmeta.GPU{
		{
			EntityID:          workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: "MIG-669110c5-64b4-589d-bd8e-cdbf3706aa27"},
			Device:            "NVIDIA A100-SXM4-40GB MIG 1g.5gb",
			GPUType:           "a100",
			Architecture:      "ampere",
			ComputeCapability: workloadmeta.GPUComputeCapability{Major: 8, Minor: 0},
			TotalMemory:       4864 << 20,
			TotalCores:        896,
			DriverVersion:     "580.65.06",
			DeviceType:        workloadmeta.GPUDeviceTypeMIG,
			ParentGPUUUID:     "GPU-9786a73c-06f3-e8d7-e13d-4da1f3f2012f",
			MIGProfile:        "1g.5gb",
		},
		{
			EntityID:          workloadmeta.EntityID{Kind: workloadmeta.KindGPU, ID: "GPU-e80c8f33-55f5-55d2-18b7-64563f748393"},
			Device:            "NVIDIA L4",
			GPUType:           "l4",
			Architecture:      "ada",
			ComputeCapability: workloadmeta.GPUComputeCapability{Major: 8, Minor: 9},
			TotalMemory:       23034 << 20,
			TotalCores:        7424,
			DriverVersion:     "580.65.06",
			PCIBusID:          "0000:00:03.0",
			DeviceType:        workloadmeta.GPUDeviceTypePhysical,
			MinorNumber:       0,
			SharingMode:       workloadmeta.GPUSharingModeTimeSlicing,
			SharingReplicas:   4,
		},
	}

	assert.Equal(t, []*model.NodeGPUDevice{
		{
			Uuid:              "GPU-e80c8f33-55f5-55d2-18b7-64563f748393",
			Model:             "NVIDIA L4",
			GpuType:           "l4",
			Architecture:      "ada",
			ComputeCapability: "8.9",
			TotalMemoryBytes:  23034 << 20,
			TotalCores:        7424,
			DriverVersion:     "580.65.06",
			PciBusId:          "0000:00:03.0",
			DeviceType:        "physical",
			SharingMode:       "time-slicing",
			SharingReplicas:   4,
		},
		{
			Uuid:              "MIG-669110c5-64b4-589d-bd8e-cdbf3706aa27",
			Model:             "NVIDIA A100-SXM4-40GB MIG 1g.5gb",
			GpuType:           "a100",
			Architecture:      "ampere",
			ComputeCapability: "8.0",
			TotalMemoryBytes:  4864 << 20,
			TotalCores:        896,
			DriverVersion:     "580.65.06",
			DeviceType:        "mig",
			ParentUuid:        "GPU-9786a73c-06f3-e8d7-e13d-4da1f3f2012f",
			MigProfile:        "1g.5gb",
		},
	}, gpuDevicesFromWorkloadmeta(gpus))
}
