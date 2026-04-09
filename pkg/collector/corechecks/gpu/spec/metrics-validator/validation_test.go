package main

import (
	"testing"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/stretchr/testify/require"
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

	result := getExpectedMetricsForGPUConfig(spec, gpuConfig{
		Architecture: "ampere",
		DeviceMode:   gpuspec.DeviceModePhysical,
		IsKnown:      true,
	})

	require.Equal(t, map[string]gpuspec.MetricSpec{
		"gpu.device.total": spec.Metrics["device.total"],
	}, result)
}

func TestCombineKnownAndLiveGPUConfigsAddsUnknownLiveConfig(t *testing.T) {
	known := []gpuConfig{
		{Architecture: "ampere", DeviceMode: gpuspec.DeviceModePhysical, IsKnown: true},
	}

	result := combineKnownAndLiveGPUConfigs(known, map[string]struct{}{
		"hopper|mig":      {},
		"ampere|physical": {},
	})

	require.Len(t, result, 2)
	require.Equal(t, gpuConfig{Architecture: "ampere", DeviceMode: gpuspec.DeviceModePhysical, IsKnown: true}, result[0])
	require.Equal(t, gpuConfig{Architecture: "hopper", DeviceMode: gpuspec.DeviceModeMIG, IsKnown: false}, result[1])
}
