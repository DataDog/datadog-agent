// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMetricType(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		err := validateMetricType("gauge", "gauge")
		require.NoError(t, err)
	})

	t.Run("mismatch returns error", func(t *testing.T) {
		err := validateMetricType("gauge", "counter")
		require.ErrorContains(t, err, "does not match expected")
	})

	t.Run("case mismatch returns error", func(t *testing.T) {
		err := validateMetricType("gauge", "Gauge")
		require.ErrorContains(t, err, "does not match expected")
	})

	t.Run("missing observed type is allowed", func(t *testing.T) {
		err := validateMetricType("gauge", "")
		require.NoError(t, err)
	})
}

func TestExpectedMetricsSuppressesInactiveNVLinkMetrics(t *testing.T) {
	specs := &Specs{
		Metrics: &MetricsSpec{
			Metrics: map[string]MetricSpec{
				"nvlink.count.active": {
					Support: MetricSupportSpec{
						DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true},
					},
				},
				"nvlink.nvswitch_connected": {
					Support: MetricSupportSpec{
						DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true},
					},
				},
			},
		},
	}
	config := GPUConfig{
		Architecture: "hopper",
		DeviceMode:   DeviceModePhysical,
		Capabilities: ArchitectureCapabilities{NVLink: 4},
	}

	expected := ExpectedMetricsForConfig(specs, config, ValidationOptions{})

	require.NotContains(t, expected, "nvlink.count.active")
	require.Contains(t, expected, "nvlink.nvswitch_connected")
}

func TestValidateEmittedMetricsAllowsZeroNVSwitchWithNoActiveNVLink(t *testing.T) {
	specs := &Specs{
		Metrics: &MetricsSpec{
			Metrics: map[string]MetricSpec{
				"nvlink.nvswitch_connected": {
					Support: MetricSupportSpec{
						DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true},
					},
				},
			},
		},
		Tags: &TagsSpec{},
	}
	config := GPUConfig{
		Architecture: "hopper",
		DeviceMode:   DeviceModePhysical,
		Capabilities: ArchitectureCapabilities{NVLink: 4},
	}
	value := float64(0)
	emittedMetrics := map[string][]MetricObservation{
		"nvlink.nvswitch_connected": {{Value: &value}},
	}

	result, err := ValidateEmittedMetricsAgainstSpec(specs, config, emittedMetrics, nil, ValidationOptions{})

	require.NoError(t, err)
	require.False(t, result.HasFailures())
}
