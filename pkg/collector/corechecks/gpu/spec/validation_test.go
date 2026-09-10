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

func TestValidateEmittedMetricsAgainstSpecExternalValues(t *testing.T) {
	value := 10.0
	specs := &Specs{
		Metrics: &MetricsSpec{
			Metrics: map[string]MetricSpec{
				"temperature": {
					Validator: &MetricValidator{
						NvidiaSMI:      true,
						ValueTolerance: &MetricValueTolerance{Absolute: ptrTo(1.0)},
					},
					Support: MetricSupportSpec{
						DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true},
					},
				},
				"unmarked": {
					Support: MetricSupportSpec{
						DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true},
					},
				},
			},
		},
		Tags: &TagsSpec{},
	}
	config := GPUConfig{Architecture: "hopper", DeviceMode: DeviceModePhysical}
	emitted := map[string][]MetricObservation{
		"temperature": {{Value: &value}},
		"unmarked":    {{Value: &value}},
	}

	t.Run("matches reference", func(t *testing.T) {
		reference := 10.5
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, emitted, nil, ValidationOptions{
			NvidiaSMIValues: map[string]*float64{"temperature": &reference},
		})
		require.NoError(t, err)
		require.False(t, result.HasFailures())
		require.Contains(t, result.Metrics, "unmarked")
	})

	t.Run("missing reference fails", func(t *testing.T) {
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, emitted, nil, ValidationOptions{
			NvidiaSMIValues: map[string]*float64{},
		})
		require.NoError(t, err)
		require.True(t, result.HasFailures())
		require.Equal(t, 1, result.Metrics["temperature"].InvalidValue)
	})

	t.Run("empty observations fail", func(t *testing.T) {
		reference := 10.0
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, map[string][]MetricObservation{
			"temperature": {},
			"unmarked":    {{Value: &value}},
		}, nil, ValidationOptions{
			NvidiaSMIValues: map[string]*float64{"temperature": &reference},
		})
		require.NoError(t, err)
		require.True(t, result.HasFailures())
		require.Equal(t, 1, result.Metrics["temperature"].InvalidValue)
		require.Contains(t, result.Metrics["temperature"].InvalidValueSamples[0], "observation is missing")
	})

	t.Run("external validation still checks unmarked metrics", func(t *testing.T) {
		reference := 10.0
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, map[string][]MetricObservation{
			"temperature": {{Value: &value}},
		}, nil, ValidationOptions{
			NvidiaSMIValues: map[string]*float64{"temperature": &reference},
		})
		require.NoError(t, err)
		require.Equal(t, 1, result.Metrics["unmarked"].Missing)
	})
}

func TestValidationAvailabilityOptions(t *testing.T) {
	value := 1.0
	specs := &Specs{
		Metrics: &MetricsSpec{
			Metrics: map[string]MetricSpec{
				"required": {
					Support: MetricSupportSpec{DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true}},
				},
				"optional": {
					Optional: true,
					Validator: &MetricValidator{
						Range: &MetricValidatorRange{Min: ptrTo(0.0), Max: ptrTo(1.0)},
					},
					Support: MetricSupportSpec{DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true}},
				},
				"ebpf": {
					ConfigRequired: []ConfigFeature{ConfigFeatureSystemProbeEBPF},
					Support:        MetricSupportSpec{DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true}},
				},
				"workload": {
					WorkloadOnly: true,
					Tagsets:      []string{"process", "container"},
					Support:      MetricSupportSpec{DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true}},
				},
			},
		},
		Tags: &TagsSpec{
			Tags: map[string]TagSpec{
				"pid":                 {},
				"kube_container_name": {},
			},
			Tagsets: map[string]TagsetSpec{
				"process":   {Tags: []string{"pid"}, WorkloadOnly: true},
				"container": {Tags: []string{"kube_container_name"}, WorkloadOnly: true},
			},
		},
	}
	config := GPUConfig{Architecture: "hopper", DeviceMode: DeviceModePhysical}

	t.Run("optional metrics may be absent", func(t *testing.T) {
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, map[string][]MetricObservation{
			"required": {{Value: &value}},
		}, nil, ValidationOptions{})
		require.NoError(t, err)
		require.False(t, result.HasFailures())
		require.NotContains(t, result.Metrics, "optional")
		require.NotContains(t, result.Metrics, "ebpf")
		require.NotContains(t, result.Metrics, "workload")
	})

	t.Run("emitted optional metrics are validated", func(t *testing.T) {
		invalid := 2.0
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, map[string][]MetricObservation{
			"required": {{Value: &value}},
			"optional": {{Value: &invalid}},
		}, nil, ValidationOptions{})
		require.NoError(t, err)
		require.Equal(t, 1, result.Metrics["optional"].InvalidValue)
	})

	t.Run("enabled config feature requires metric", func(t *testing.T) {
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, map[string][]MetricObservation{
			"required": {{Value: &value}},
		}, nil, ValidationOptions{
			ConfigFeatures: map[ConfigFeature]bool{ConfigFeatureSystemProbeEBPF: true},
		})
		require.NoError(t, err)
		require.Equal(t, 1, result.Metrics["ebpf"].Missing)
	})

	t.Run("selected workload tagsets are required", func(t *testing.T) {
		result, err := ValidateEmittedMetricsAgainstSpec(specs, config, map[string][]MetricObservation{
			"required": {{Value: &value}},
			"workload": {{Value: &value, Tags: []string{"pid:123"}}},
		}, nil, ValidationOptions{
			WorkloadActive:  true,
			WorkloadTagsets: map[string]bool{"process": true},
		})
		require.NoError(t, err)
		require.False(t, result.HasFailures())
	})

	t.Run("invalid enabled tagset is rejected", func(t *testing.T) {
		_, err := ValidateEmittedMetricsAgainstSpec(specs, config, nil, nil, ValidationOptions{
			WorkloadTagsets: map[string]bool{"unknown": true},
		})
		require.ErrorContains(t, err, `unknown enabled workload tagset "unknown"`)
	})
}

func TestExternalValidationDoesNotRepeatStaticFailure(t *testing.T) {
	value := 101.0
	reference := 100.0
	specs := &Specs{
		Metrics: &MetricsSpec{
			Metrics: map[string]MetricSpec{
				"sm_active": {
					Validator: &MetricValidator{
						Range:              &MetricValidatorRange{Min: ptrTo(0.0), Max: ptrTo(100.0)},
						NvidiaSMI:          true,
						CalibratedWorkload: true,
						ValueTolerance:     &MetricValueTolerance{Absolute: ptrTo(15.0)},
					},
					Support: MetricSupportSpec{DeviceModes: map[DeviceMode]bool{DeviceModePhysical: true}},
				},
			},
		},
		Tags: &TagsSpec{},
	}

	result, err := ValidateEmittedMetricsAgainstSpec(specs, GPUConfig{DeviceMode: DeviceModePhysical}, map[string][]MetricObservation{
		"sm_active": {{Value: &value}},
	}, nil, ValidationOptions{
		NvidiaSMIValues:          map[string]*float64{"sm_active": &reference},
		CalibratedWorkloadValues: map[string]*float64{"sm_active": &reference},
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Metrics["sm_active"].InvalidValue)
}
