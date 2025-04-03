// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"slices"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestProcessSysprobeStats(t *testing.T) {
	// Create a mock sender
	mockSender := mocksender.NewMockSender("gpu")
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		testutil.GetWorkloadMetaMock(t),
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	// Set NVML mock to return a single device with the default UUID
	nvmlMock := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return testutil.GetDeviceMock(index), nvml.SUCCESS
		},
	}

	// Set device cache mock
	deviceCache, err := ddnvml.NewDeviceCacheWithOptions(nvmlMock)
	require.NoError(t, err)
	check.deviceCache = deviceCache

	// Set up GPU and container tags
	containerID := "container1"
	containerTags := []string{"container_id:" + containerID}
	gpuTags := []string{"gpu_uuid:" + testutil.DefaultGpuUUID, "gpu_vendor:nvidia", "gpu_arch:pascal"}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, containerID), "foo", containerTags, nil, nil, nil)

	check.deviceTags = map[string][]string{
		testutil.DefaultGpuUUID: gpuTags,
	}

	// Create test data
	stats := model.GPUStats{
		Metrics: []model.StatsTuple{
			{
				Key: model.StatsKey{
					PID:         123,
					ContainerID: containerID,
					DeviceUUID:  testutil.DefaultGpuUUID,
				},
				UtilizationMetrics: model.UtilizationMetrics{
					UsedCores: 50,
					Memory: model.MemoryMetrics{
						CurrentBytes: 1024,
					},
				},
			},
		},
	}

	gpuToContainersMap := map[string][]*workloadmeta.Container{
		testutil.DefaultGpuUUID: {
			{
				EntityID: workloadmeta.EntityID{
					ID: containerID,
				},
			},
		},
	}

	// Process the stats
	err = check.processSysprobeStats(mockSender, stats, gpuToContainersMap)
	assert.NoError(t, err)

	// Verify we sent metrics with the correct tags and values
	expectedTags := slices.Concat([]string{"pid:123"}, containerTags, gpuTags)
	slices.Sort(expectedTags) // Sort to avoid order issues
	matchTagsFunc := func(tags []string) bool {
		// Check that both slices have the same elements, regardless of order
		slices.Sort(tags)
		return slices.Equal(tags, expectedTags)
	}

	mockSender.AssertCalled(t, "Gauge", "gpu.core.usage", float64(50), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.usage", float64(1024), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.core.limit", float64(testutil.DefaultGpuCores), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.limit", float64(testutil.DefaultTotalMemory), "", mock.MatchedBy(matchTagsFunc))
	mockSender.ResetCalls()

	// Now do another run with no data
	stats = model.GPUStats{}
	err = check.processSysprobeStats(mockSender, stats, gpuToContainersMap)
	assert.NoError(t, err)

	// Verify we sent metrics with the correct tags and zero values
	mockSender.AssertCalled(t, "Gauge", "gpu.core.usage", float64(0), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.usage", float64(0), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.core.limit", float64(testutil.DefaultGpuCores), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.limit", float64(testutil.DefaultTotalMemory), "", mock.MatchedBy(matchTagsFunc))
}
