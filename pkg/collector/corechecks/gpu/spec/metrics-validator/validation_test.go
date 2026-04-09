package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

func TestGetExpectedMetricsForGPUConfigSkipsUnsupportedEntries(t *testing.T) {
	spec := &gpuspec.MetricsSpec{
		MetricPrefix: "gpu",
		Metrics: map[string]gpuspec.MetricSpec{
			"device.total": {
				Support: gpuspec.MetricSupportSpec{
					DeviceModes: map[gpuspec.DeviceMode]bool{
						gpuspec.DeviceModePhysical: true,
					},
				},
			},
			"device.mig_only": {
				Support: gpuspec.MetricSupportSpec{
					DeviceModes: map[gpuspec.DeviceMode]bool{
						gpuspec.DeviceModeMIG: true,
					},
				},
			},
			"device.unsupported_arch": {
				Support: gpuspec.MetricSupportSpec{
					UnsupportedArchitectures: []string{"ampere"},
					DeviceModes: map[gpuspec.DeviceMode]bool{
						gpuspec.DeviceModePhysical: true,
					},
				},
			},
		},
	}

	result := gpuspec.ExpectedMetricsForConfig(spec, "ampere", gpuspec.DeviceModePhysical)

	require.Equal(t, map[string]gpuspec.MetricSpec{
		"gpu.device.total": spec.Metrics["device.total"],
	}, result)
}

func TestCombineKnownAndLiveGPUConfigsAddsUnknownLiveConfig(t *testing.T) {
	known := []gpuConfig{
		{Architecture: "ampere", DeviceMode: gpuspec.DeviceModePhysical, IsKnown: true},
	}

	result := combineKnownAndLiveGPUConfigs(known, []gpuConfig{
		{Architecture: "hopper", DeviceMode: gpuspec.DeviceModeMIG, IsKnown: false},
		{Architecture: "ampere", DeviceMode: gpuspec.DeviceModePhysical, IsKnown: true},
	})

	require.Len(t, result, 2)
	require.Equal(t, gpuConfig{Architecture: "ampere", DeviceMode: gpuspec.DeviceModePhysical, IsKnown: true}, result[0])
	require.Equal(t, gpuConfig{Architecture: "hopper", DeviceMode: gpuspec.DeviceModeMIG, IsKnown: false}, result[1])
}
