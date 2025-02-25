// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostgpuimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
			Architecture: "turing",
			ComputeCapability: workloadmeta.GPUComputeCapability{
				Major: 12,
				Minor: 4,
			},
			DriverVersion: "460.32.03",

			TotalMemoryMB:  4096,
			SMCount:        2040,
			MemoryBusWidth: 256,
			MaxClockRates:  [workloadmeta.COUNT]uint32{4000, 5000},
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
			Architecture: "hopper",
			ComputeCapability: workloadmeta.GPUComputeCapability{
				Major: 12,
				Minor: 4,
			},
			DriverVersion:  "460.32.03",
			TotalMemoryMB:  8192,
			SMCount:        4050,
			MemoryBusWidth: 256,
			MaxClockRates:  [workloadmeta.COUNT]uint32{8000, 10000},
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
	p := newGPUHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() workloadmeta.Component { return &wmsMock{} }),
			config.MockModule(),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
		),
	)
	return p.Comp.(*gpuHost)
}

func TestGetPayload(t *testing.T) {
	gh := getTestInventoryHost(t)
	expectedMetadata := hostGPUMetadata{
		Devices: []*gpuDeviceMetadata{
			{
				Index:              0,
				Vendor:             "nvidia",
				UUID:               "GPU-12345678-1234-5678-1234-111111111111",
				Name:               "Tesla T4",
				Architecture:       "turing",
				ComputeVersion:     "12.4",
				DriverVersion:      "460.32.03",
				ProcessorUnits:     2040,
				TotalMemory:        4 * 1024 * 1024 * 1024, // 4 GB
				MaxSMClockRate:     4000,                   // MHz
				MaxMemoryClockRate: 5000,                   // MHz
				MemoryBusWidth:     256,                    // bits
			},
			{
				Index:              1,
				Vendor:             "nvidia",
				UUID:               "GPU-87654321-4321-8765-4321-876543218765",
				Name:               "H100",
				Architecture:       "hopper",
				ComputeVersion:     "12.4",
				DriverVersion:      "460.32.03",
				ProcessorUnits:     4050,
				TotalMemory:        8 * 1024 * 1024 * 1024, // 8 GB
				MaxSMClockRate:     8000,                   // MHz
				MaxMemoryClockRate: 10000,                  // MHz
				MemoryBusWidth:     256,                    // bits
			},
		},
	}

	p := gh.getPayload().(*Payload)
	assert.Equal(t, expectedMetadata, p.Metadata)
}

func TestGetPayloadError(t *testing.T) {
	gh := getTestInventoryHost(t)
	gh.wmeta = &wmsErrorMock{}
	expected := &hostGPUMetadata{
		Devices: nil,
	}

	p := gh.getPayload().(*Payload)
	assert.Equal(t, expected, p.Metadata)
}

func TestFlareProviderFilename(t *testing.T) {
	gh := getTestInventoryHost(t)
	assert.Equal(t, flareFileName, gh.FlareFileName)
}
