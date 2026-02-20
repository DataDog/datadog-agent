// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostgpuimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

type wmsMock struct {
	workloadmeta.Component
}

func (s *wmsMock) ListGPUs() []*workloadmeta.GPU {
	return []*workloadmeta.GPU{
		{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindGPU,
				ID:   "GPU-12345678-1234-5678-1234-111111111111",
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name: "Tesla T4",
			},
			Index:        0,
			Vendor:       "Nvidia",
			Device:       "Tesla T4",
			GPUType:      "t4",
			Architecture: "turing",
			ComputeCapability: workloadmeta.GPUComputeCapability{
				Major: 12,
				Minor: 4,
			},
			DriverVersion: "460.32.03",

			TotalMemory:        4 * 1024 * 1024 * 1024, // 4 GB
			TotalCores:         2040,
			MemoryBusWidth:     256,
			MaxClockRates:      [workloadmeta.GPUCOUNT]uint32{4000, 5000},
			VirtualizationMode: "baremetal",
			ChildrenGPUUUIDs:   []string{"GPU-87654321-1234-5678-1234-222222222222"},
		},
		{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindGPU,
				ID:   "GPU-87654321-1234-5678-1234-222222222222",
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name: "H100",
			},
			Index:        1,
			Vendor:       "nvidia",
			Device:       "H100",
			GPUType:      "h100",
			Architecture: "hopper",
			ComputeCapability: workloadmeta.GPUComputeCapability{
				Major: 12,
				Minor: 4,
			},
			DriverVersion:      "460.32.03",
			TotalMemory:        8 * 1024 * 1024 * 1024, // 8 GB
			TotalCores:         4050,
			MemoryBusWidth:     256,
			MaxClockRates:      [workloadmeta.GPUCOUNT]uint32{8000, 10000},
			DeviceType:         workloadmeta.GPUDeviceTypeMIG,
			VirtualizationMode: "mig",
			ParentGPUUUID:      "GPU-12345678-1234-5678-1234-111111111111",
			ChildrenGPUUUIDs:   []string{},
		},
	}
}

type wmsErrorMock struct {
	workloadmeta.Component
}

func (s *wmsErrorMock) ListGPUs() []*workloadmeta.GPU {
	return nil
}

func getTestInventoryHost(t *testing.T) *gpuHost {
	p := NewGPUHostProvider(Requires{
		WMeta:      &wmsMock{},
		Log:        logmock.New(t),
		Config:     configmock.New(t),
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostnameimpl.NewHostnameService(),
	})
	return p.Comp.(*gpuHost)
}

func TestGetPayload(t *testing.T) {
	gh := getTestInventoryHost(t)
	expectedMetadata := &hostGPUMetadata{
		Devices: []*gpuDeviceMetadata{
			{
				Index:              0,
				Vendor:             "nvidia",
				UUID:               "GPU-12345678-1234-5678-1234-111111111111",
				Name:               "Tesla T4",
				Architecture:       "turing",
				GPUType:            "t4",
				SlicingMode:        "mig-parent",
				VirtualizationMode: "baremetal",
				ComputeVersion:     "12.4",
				DriverVersion:      "460.32.03",
				TotalCores:         2040,
				ParentGPUUUID:      "",
				TotalMemory:        4 * 1024 * 1024 * 1024, // 4 GB
				MaxSMClockRate:     4000,                   // MHz
				MaxMemoryClockRate: 5000,                   // MHz
				MemoryBusWidth:     256,                    // bits
			},
			{
				Index:              1,
				Vendor:             "nvidia",
				UUID:               "GPU-87654321-1234-5678-1234-222222222222",
				Name:               "H100",
				Architecture:       "hopper",
				GPUType:            "h100",
				SlicingMode:        "mig",
				VirtualizationMode: "mig",
				ComputeVersion:     "12.4",
				DriverVersion:      "460.32.03",
				TotalCores:         4050,
				ParentGPUUUID:      "GPU-12345678-1234-5678-1234-111111111111",
				TotalMemory:        8 * 1024 * 1024 * 1024, // 8 GB
				MaxSMClockRate:     8000,                   // MHz
				MaxMemoryClockRate: 10000,                  // MHz
				MemoryBusWidth:     256,                    // bits
			},
		},
	}

	p, ok := gh.getPayload().(*Payload)
	assert.True(t, ok)
	assert.Equal(t, expectedMetadata, p.Metadata)
}

func TestGetEmptyPayload(t *testing.T) {
	gh := getTestInventoryHost(t)
	gh.wmeta = &wmsErrorMock{}

	p := gh.getPayload()
	assert.Nil(t, p)
}

func TestFlareProviderFilename(t *testing.T) {
	gh := getTestInventoryHost(t)
	assert.Equal(t, flareFileName, gh.FlareFileName)
}
