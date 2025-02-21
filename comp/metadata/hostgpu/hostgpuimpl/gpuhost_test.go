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
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Test helpers
func collectBaseGPUMock() (*hostGPUMetadata, error) {
	device := gpuDeviceMetadata{
		ID:            0,
		Vendor:        "nvidia",
		Device:        "Tesla T4",
		DriverVersion: "460.32.03",
	}
	nvidiaDevice, _ := collectNvidiaGPUMock(device)
	return &hostGPUMetadata{

		Devices: []gpuDeviceMetadata{
			*nvidiaDevice,
		},
	}, nil
}

func collectNvidiaGPUMock(device gpuDeviceMetadata) (*gpuDeviceMetadata, error) {
	device.UUID = "GPU-123456789"
	device.Architecture = "Turing"
	device.RuntimeVersion = "11.2"
	device.ComputeVersion = "12.4"
	device.ProcessorUnits = 40
	device.Cores = 2560
	device.TotalMemory = 16000000000 // 16GB
	device.MaxClockRate = 1590
	device.MemoryClockRate = 5000
	device.MemoryBusWidth = 256
	device.L2CacheSize = 4194304
	device.WarpSize = 32
	device.RegistersPerBlock = 65536
	return &device, nil
}

func gpuErrorMock() (*hostGPUMetadata, error) { return &hostGPUMetadata{Devices: nil}, nil }

func setupHostGPUMetadataMock(t *testing.T) {
	t.Cleanup(func() {
		baseGPUGet = collectBaseGPUInfo
	})

	baseGPUGet = collectBaseGPUMock
}

func setupHostMetadataErrorMock(t *testing.T) {
	setupHostGPUMetadataMock(t)
	baseGPUGet = gpuErrorMock
}

func getTestInventoryHost(t *testing.T) *gpuHost {
	p := newGPUHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
		),
	)
	return p.Comp.(*gpuHost)
}

func TestGetPayload(t *testing.T) {
	setupHostGPUMetadataMock(t)

	expectedMetadata := hostGPUMetadata{
		Devices: []gpuDeviceMetadata{
			{
				ID:                1,
				Vendor:            "NVIDIA",
				Device:            "GeForce GTX 1080",
				UUID:              "GPU-12345678-1234-5678-1234-567812345678",
				DriverVersion:     "460.32.03",
				RuntimeVersion:    "11.2",
				Architecture:      "Pascal",
				ComputeVersion:    "6.1",
				ProcessorUnits:    20,
				Cores:             2560,
				TotalMemory:       8 * 1024 * 1024 * 1024, // 8 GB
				MaxClockRate:      1733,                   // MHz
				MemoryClockRate:   10000,                  // MHz
				MemoryBusWidth:    256,                    // bits
				L2CacheSize:       2048 * 1024,            // 2 MB
				WarpSize:          32,
				RegistersPerBlock: 65536,
			},
			{
				ID:                2,
				Vendor:            "AMD",
				Device:            "Radeon RX 5700 XT",
				UUID:              "GPU-87654321-4321-8765-4321-876543218765",
				Architecture:      "RDNA",
				ComputeVersion:    "1.0",
				ProcessorUnits:    40,
				Cores:             2560,
				TotalMemory:       8 * 1024 * 1024 * 1024, // 8 GB
				MaxClockRate:      1905,                   // MHz
				MemoryClockRate:   14000,                  // MHz
				MemoryBusWidth:    256,                    // bits
				L2CacheSize:       4096 * 1024,            // 4 MB
				WarpSize:          64,
				RegistersPerBlock: 131072,
			},
		},
	}

	gh := getTestInventoryHost(t)

	p := gh.getPayload().(*Payload)
	assert.Equal(t, expectedMetadata, p.Metadata)
}

func TestGetPayloadError(t *testing.T) {
	setupHostMetadataErrorMock(t)

	expected := &hostGPUMetadata{
		Devices: nil,
	}

	gh := getTestInventoryHost(t)
	p := gh.getPayload().(*Payload)
	assert.Equal(t, expected, p.Metadata)
}

func TestFlareProviderFilename(t *testing.T) {
	gh := getTestInventoryHost(t)
	assert.Equal(t, flareFileName, gh.FlareFileName)
}
