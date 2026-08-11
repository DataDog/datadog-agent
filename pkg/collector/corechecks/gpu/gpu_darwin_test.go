// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && !ios && cgo && test

package gpu

import (
	"errors"
	"math"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestAppleGPUMetrics(t *testing.T) {
	tests := []struct {
		name     string
		device   agxDeviceSnapshot
		expected []appleGPUMetric
	}{
		{
			name: "complete snapshot",
			device: agxDeviceSnapshot{
				coreCount:                20,
				hasCoreCount:             true,
				utilization:              72,
				hasUtilization:           true,
				allocatedSystemMemory:    35_000_000_000,
				hasAllocatedSystemMemory: true,
				inUseSystemMemory:        31_000_000_000,
				hasInUseSystemMemory:     true,
			},
			expected: []appleGPUMetric{
				{name: "apple.core.count", value: 20},
				{name: "apple.device.utilization", value: 72},
				{name: "apple.system_memory.allocated", value: 35_000_000_000},
				{name: "apple.system_memory.in_use", value: 31_000_000_000},
			},
		},
		{
			name:     "missing optional properties",
			device:   agxDeviceSnapshot{},
			expected: []appleGPUMetric{},
		},
		{
			name: "invalid values are omitted",
			device: agxDeviceSnapshot{
				coreCount:      -1,
				hasCoreCount:   true,
				utilization:    math.NaN(),
				hasUtilization: true,
			},
			expected: []appleGPUMetric{},
		},
		{
			name: "negative memory values are omitted",
			device: agxDeviceSnapshot{
				allocatedSystemMemory: -1, hasAllocatedSystemMemory: true,
				inUseSystemMemory: -1, hasInUseSystemMemory: true,
			},
			expected: []appleGPUMetric{},
		},
		{
			name: "out of range utilization is omitted",
			device: agxDeviceSnapshot{
				utilization:    101,
				hasUtilization: true,
			},
			expected: []appleGPUMetric{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, appleGPUMetrics(tt.device))
		})
	}
}

func TestAppleGPUDeviceTags(t *testing.T) {
	device := agxDeviceSnapshot{model: " Apple M5 Pro "}
	tags := appleGPUDeviceTags(device, 0)

	assert.Contains(t, tags, "gpu_device:apple_m5_pro")
	assert.Contains(t, tags, "gpu_vendor:apple")
	assert.Contains(t, tags, "gpu_architecture:apple_silicon")
	assert.Contains(t, tags, "gpu_type:m5_pro")
	assert.Contains(t, tags, "gpu_virtualization_mode:none")
	assert.Contains(t, tags, "gpu_slicing_mode:none")
	assert.Contains(t, tags, "gpu_host:true")

	uuidTagIndex := slices.IndexFunc(tags, func(tag string) bool {
		return len(tag) > len("gpu_uuid:") && tag[:len("gpu_uuid:")] == "gpu_uuid:"
	})
	require.NotEqual(t, -1, uuidTagIndex)
	assert.Regexp(t, regexp.MustCompile(`^gpu_uuid:gpu-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), tags[uuidTagIndex])
	assert.Equal(t, syntheticAppleGPUUUID("Apple M5 Pro", 0), syntheticAppleGPUUUID("Apple M5 Pro", 0))
	assert.NotEqual(t, syntheticAppleGPUUUID("Apple M5 Pro", 0), syntheticAppleGPUUUID("Apple M5 Pro", 1))
}

func TestNormalizeAppleGPUTag(t *testing.T) {
	assert.Equal(t, "apple_m5_pro", normalizeAppleGPUTag(" Apple M5 Pro "))
	assert.Equal(t, "apple_gpu", normalizeAppleGPUTag(" !!! "))
}

func TestDarwinFactoryAvailable(t *testing.T) {
	factoryOption := Factory(nil, telemetrymock.New(t), nil)
	factory, available := factoryOption.Get()
	require.True(t, available)
	assert.IsType(t, &Check{}, factory())
}

func TestDarwinCheckRun(t *testing.T) {
	pkgconfigsetup.Datadog().SetInTest("gpu.enabled", true)
	t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest("gpu.enabled", false) })

	originalCollector := collectAGXDevices
	collectAGXDevices = func() (agxCollection, error) {
		return agxCollection{devices: []agxDeviceSnapshot{
			{
				model: "Apple M5 Pro", coreCount: 20, hasCoreCount: true,
				utilization: 65, hasUtilization: true,
				allocatedSystemMemory: 35_000_000_000, hasAllocatedSystemMemory: true,
				inUseSystemMemory: 31_000_000_000, hasInUseSystemMemory: true,
			},
			{model: "Apple Test GPU"},
		}}, nil
	}
	t.Cleanup(func() { collectAGXDevices = originalCollector })

	gpuCheck := newCheck(telemetrymock.New(t)).(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, gpuCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider"))
	mockSender := mocksender.NewMockSenderWithSenderManager(gpuCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	require.NoError(t, gpuCheck.Run())
	assertTimestampMetric(t, mockSender, "gpu.apple.device.count", 2, []string{"gpu_vendor:apple", "gpu_host:true"})
	assertTimestampMetric(t, mockSender, "gpu.apple.core.count", 20, []string{"gpu_device:apple_m5_pro"})
	assertTimestampMetric(t, mockSender, "gpu.apple.device.utilization", 65, []string{"gpu_device:apple_m5_pro"})
	assertTimestampMetric(t, mockSender, "gpu.apple.system_memory.allocated", 35_000_000_000, []string{"gpu_device:apple_m5_pro"})
	assertTimestampMetric(t, mockSender, "gpu.apple.system_memory.in_use", 31_000_000_000, []string{"gpu_device:apple_m5_pro"})
	mockSender.AssertMetricMissing(t, "GaugeWithTimestamp", "gpu.device.total")
	mockSender.AssertMetricMissing(t, "GaugeWithTimestamp", "gpu.core.limit")
	mockSender.AssertMetricMissing(t, "GaugeWithTimestamp", "gpu.gr_engine_active")
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 5)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestDarwinCheckCommitsOnCollectionError(t *testing.T) {
	pkgconfigsetup.Datadog().SetInTest("gpu.enabled", true)
	t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest("gpu.enabled", false) })

	originalCollector := collectAGXDevices
	collectAGXDevices = func() (agxCollection, error) {
		return agxCollection{}, errors.New("native read failed")
	}
	t.Cleanup(func() { collectAGXDevices = originalCollector })

	gpuCheck := newCheck(telemetrymock.New(t)).(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, gpuCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider"))
	mockSender := mocksender.NewMockSenderWithSenderManager(gpuCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err := gpuCheck.Run()
	require.ErrorContains(t, err, "native read failed")
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
	mockSender.AssertNumberOfCalls(t, "GaugeWithTimestamp", 0)
}

func TestDarwinCheckDisabled(t *testing.T) {
	pkgconfigsetup.Datadog().SetInTest("gpu.enabled", false)
	gpuCheck := newCheck(telemetrymock.New(t)).(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)

	err := gpuCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")
	require.EqualError(t, err, "GPU check is disabled")
}

func TestDarwinCheckIntervalOverride(t *testing.T) {
	pkgconfigsetup.Datadog().SetInTest("gpu.collection_interval_override", 7)
	t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest("gpu.collection_interval_override", 0) })

	gpuCheck := newCheck(telemetrymock.New(t)).(*Check)
	assert.Equal(t, 7*time.Second, gpuCheck.Interval())
}

func TestReadAGXDevicesSmoke(t *testing.T) {
	collection, err := readAGXDevices()
	require.NoError(t, err)
	if len(collection.devices) == 0 {
		t.Skip("this Mac does not expose an AGX accelerator")
	}

	device := collection.devices[0]
	if device.hasCoreCount {
		assert.Positive(t, device.coreCount)
	}
	if device.hasUtilization {
		assert.GreaterOrEqual(t, device.utilization, float64(0))
		assert.LessOrEqual(t, device.utilization, float64(100))
	}
	if device.hasAllocatedSystemMemory {
		assert.GreaterOrEqual(t, device.allocatedSystemMemory, int64(0))
	}
	if device.hasInUseSystemMemory {
		assert.GreaterOrEqual(t, device.inUseSystemMemory, int64(0))
	}
}

func assertTimestampMetric(t *testing.T, mockSender *mocksender.MockSender, name string, value float64, expectedTags []string) {
	t.Helper()
	for _, call := range mockSender.Mock.Calls {
		if call.Method != "GaugeWithTimestamp" || call.Arguments.String(0) != name || call.Arguments.Get(1) != value {
			continue
		}
		tags, ok := call.Arguments.Get(3).([]string)
		if !ok {
			continue
		}
		if slices.ContainsFunc(expectedTags, func(expected string) bool { return !slices.Contains(tags, expected) }) {
			continue
		}
		return
	}
	t.Errorf("metric %s=%v with tags %v was not emitted", name, value, expectedTags)
}
