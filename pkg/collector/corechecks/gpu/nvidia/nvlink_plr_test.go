// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/gpu/prm"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

type fakePRMCache struct {
	responses map[int]map[string]uint64
	errors    map[int]error
	requests  []model.PRMRequest
}

func (f *fakePRMCache) RegisterRequest(request model.PRMRequest) {
	f.requests = append(f.requests, request)
}

func (f *fakePRMCache) GetCounters(_ string, port int) (map[string]uint64, error) {
	if err := f.errors[port]; err != nil {
		return nil, err
	}
	counters, found := f.responses[port]
	if !found {
		return nil, errors.New("missing response")
	}
	return counters, nil
}

func TestNVLinkPLRCollectorWithPRMCache(t *testing.T) {
	mockDevice := setupMockDevice(t, testutil.WithNVLinkLinkCount(2))

	cache := &fakePRMCache{
		responses: map[int]map[string]uint64{
			1: makeCounters(100),
			2: makeCounters(200),
		},
		errors: map[int]error{},
	}

	collector, err := newNVLinkCollectorWithCacheForTest(mockDevice, cache)
	require.NoError(t, err)
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, len(prm.PLRCounterFields)*2)

	port1Count := 0
	port2Count := 0
	for _, metric := range metrics {
		switch {
		case hasTag(metric.Tags, "nvlink_port:1"):
			port1Count++
		case hasTag(metric.Tags, "nvlink_port:2"):
			port2Count++
		default:
			t.Fatalf("missing nvlink_port tag on metric %+v", metric)
		}
	}
	require.Equal(t, len(prm.PLRCounterFields), port1Count)
	require.Equal(t, len(prm.PLRCounterFields), port2Count)
	require.Len(t, cache.requests, 2)
}

func TestNVLinkPLRCollectorCachePartialError(t *testing.T) {
	mockDevice := setupMockDevice(t, testutil.WithNVLinkLinkCount(2))

	cache := &fakePRMCache{
		responses: map[int]map[string]uint64{
			1: makeCounters(100),
		},
		errors: map[int]error{
			2: errors.New("port unavailable"),
		},
	}

	collector, err := newNVLinkCollectorWithCacheForTest(mockDevice, cache)
	require.NoError(t, err)
	metrics, err := collector.Collect()
	require.Error(t, err)
	require.Len(t, metrics, len(prm.PLRCounterFields))
	for _, metric := range metrics {
		require.Contains(t, metric.Tags, "nvlink_port:1")
	}
}

func TestNVLinkCollectorNilCacheReturnsUnsupported(t *testing.T) {
	mockDevice := setupMockDevice(t, testutil.WithNVLinkLinkCount(2))

	_, err := newNVLinkPLRCollector(mockDevice, nil)
	require.ErrorIs(t, err, errUnsupportedDevice)

	_, err = newNVLinkPLRCollector(mockDevice, &CollectorDependencies{})
	require.ErrorIs(t, err, errUnsupportedDevice)
}

func TestNVLinkPLRCollectorUnsupportedDevice(t *testing.T) {
	tests := []struct {
		name      string
		customize []testutil.NvmlMockOption
	}{
		{
			name: "field API unsupported",
			customize: []testutil.NvmlMockOption{
				testutil.WithMockAllFunctions(),
				testutil.WithFieldValuesReturn(nvml.ERROR_NOT_SUPPORTED),
			},
		},
		{
			name: "no nvlink ports",
			customize: []testutil.NvmlMockOption{
				testutil.WithMockAllFunctions(),
				testutil.WithNVLinkLinkCount(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []testutil.NvmlMockOption{
				testutil.WithArchitecture("blackwell"),
			}
			opts = append(opts, tt.customize...)
			mockDevice := setupMockDevice(t, opts...)
			_, err := newNVLinkPLRCollector(mockDevice, &CollectorDependencies{PRMCache: &PRMCache{}})
			require.ErrorIs(t, err, errUnsupportedDevice)
		})
	}
}

func TestNVLinkPLRCollectorPreBlackwellUnsupported(t *testing.T) {
	mockDevice := setupMockDevice(t, testutil.WithArchitecture("hopper"), testutil.WithNVLinkLinkCount(2))

	_, err := newNVLinkPLRCollector(mockDevice, &CollectorDependencies{PRMCache: &PRMCache{}})
	require.ErrorIs(t, err, errUnsupportedDevice)
	require.ErrorContains(t, err, "Blackwell or newer")
}

func TestPLRMetricSpecEntries(t *testing.T) {
	spec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)

	for _, metricName := range prm.PLRCounterFields {
		t.Run(metricName, func(t *testing.T) {
			metricSpec, ok := spec.Metrics[metricName]
			require.True(t, ok, "metric %s missing from spec", metricName)
			require.Contains(t, metricSpec.Tagsets, "nvlink")
			require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical))
			require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeMIG))
			require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeVGPU))
		})
	}
}

func newNVLinkCollectorWithCacheForTest(device ddnvml.Device, cache prmMetricsSource) (Collector, error) {
	ports, err := getSupportedNvlinkPorts(device, portIsAlwaysSupported)
	if err != nil {
		return nil, err
	}

	for _, port := range ports {
		cache.RegisterRequest(model.PRMRequest{
			DeviceUUID: device.GetDeviceInfo().UUID,
			Port:       port,
			Group:      prm.PPCNTGroupPLR,
		})
	}

	return &nvlinkPLRCollector{
		device:   device,
		ports:    ports,
		prmCache: cache,
	}, nil
}

func makeCounters(seed uint64) map[string]uint64 {
	counters := make(map[string]uint64, len(prm.PLRCounterFields))
	for i, field := range prm.PLRCounterFields {
		counters[field] = seed + uint64(i)
	}
	return counters
}

func hasTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}
