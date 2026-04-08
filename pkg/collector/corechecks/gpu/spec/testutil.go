// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package spec

import (
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

var nvmlFieldNameToFieldID = map[string]uint32{
	"FI_DEV_MEMORY_TEMP":                       nvml.FI_DEV_MEMORY_TEMP,
	"FI_DEV_NVLINK_LINK_COUNT":                 nvml.FI_DEV_NVLINK_LINK_COUNT,
	"FI_DEV_NVLINK_THROUGHPUT_DATA_RX":         nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX,
	"FI_DEV_NVLINK_THROUGHPUT_DATA_TX":         nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX,
	"FI_DEV_NVLINK_THROUGHPUT_RAW_RX":          nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX,
	"FI_DEV_NVLINK_THROUGHPUT_RAW_TX":          nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX,
	"FI_DEV_NVLINK_SPEED_MBPS_COMMON":          nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON,
	"FI_DEV_NVLINK_GET_SPEED":                  nvml.FI_DEV_NVLINK_GET_SPEED,
	"FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT":     nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT,
	"FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL":   nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL,
	"FI_DEV_PCIE_REPLAY_COUNTER":               nvml.FI_DEV_PCIE_REPLAY_COUNTER,
	"FI_DEV_PERF_POLICY_THERMAL":               nvml.FI_DEV_PERF_POLICY_THERMAL,
}

func unsupportedFieldIDsFromNames(t *testing.T, names []string) []uint32 {
	t.Helper()

	ids := make([]uint32, 0, len(names))
	for _, name := range names {
		id, found := nvmlFieldNameToFieldID[name]
		require.True(t, found, "unknown NVML field in architectures capabilities: %s", name)
		ids = append(ids, id)
	}
	return ids
}

// UnsupportedFieldIDsForMode computes unsupported NVML field IDs for an architecture+mode.
func UnsupportedFieldIDsForMode(t *testing.T, archSpec ArchitectureSpec, mode DeviceMode) []uint32 {
	t.Helper()

	unsupportedNameSet := make(map[string]struct{})
	for _, group := range archSpec.Capabilities.UnsupportedFieldsByDeviceMode {
		if len(group.DeviceModes) > 0 && !slices.Contains(group.DeviceModes, mode) {
			continue
		}
		for _, name := range group.Fields {
			unsupportedNameSet[name] = struct{}{}
		}
	}

	unsupportedNames := make([]string, 0, len(unsupportedNameSet))
	for name := range unsupportedNameSet {
		unsupportedNames = append(unsupportedNames, name)
	}
	return unsupportedFieldIDsFromNames(t, unsupportedNames)
}

// BuildMockOptionsForArchAndMode creates canonical NVML mock options from spec capabilities.
func BuildMockOptionsForArchAndMode(t *testing.T, archName string, mode DeviceMode, archSpec ArchitectureSpec) []testutil.NvmlMockOption {
	t.Helper()

	testMode := testutil.DeviceFeatureMode(mode)
	caps := testutil.Capabilities{
		GPM:               archSpec.Capabilities.GPM,
		UnsupportedFields: UnsupportedFieldIDsForMode(t, archSpec, mode),
	}
	opts := []testutil.NvmlMockOption{
		testutil.WithArchitecture(archName),
		testutil.WithCapabilities(caps),
		testutil.WithMockAllFunctions(),
		testutil.WithDeviceCount(1),
		testutil.WithDeviceFeatureMode(testMode),
	}

	if mode == DeviceModeMIG {
		opts = append(opts, testutil.WithMIGChildCount(1))
	} else {
		opts = append(opts, testutil.WithMIGDisabled())
	}

	return opts
}

// AllConfiguredNVMLFieldValues returns all field IDs configured in the test mapping.
func AllConfiguredNVMLFieldValues() []nvml.FieldValue {
	ids := make([]uint32, 0, len(nvmlFieldNameToFieldID))
	for _, id := range nvmlFieldNameToFieldID {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	values := make([]nvml.FieldValue, len(ids))
	for i, id := range ids {
		values[i] = nvml.FieldValue{FieldId: id}
	}
	return values
}

// EmittedMetric is a normalized emitted GPU metric sample used by spec-aware tests.
type EmittedMetric struct {
	Name string
	Tags []string
}

// TagsToKeyValues converts Datadog-style tags to a key -> values map.
func TagsToKeyValues(tags []string) map[string][]string {
	result := make(map[string][]string, len(tags))
	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || key == "" || value == "" {
			continue
		}
		result[key] = append(result[key], value)
	}
	return result
}

// ValidateMetricTagsAgainstSpec validates emitted tags against the spec for a metric.
// If knownTagValues is provided, matching keys are additionally checked for exact values.
func ValidateMetricTagsAgainstSpec(t *testing.T, spec *MetricsSpec, metricName string, metricSpec MetricSpec, emittedMetrics []EmittedMetric, knownTagValues map[string]string) {
	t.Helper()
	require.NotEmpty(t, emittedMetrics, "metric %s has no emitted samples to validate tags", metricName)

	requiredTags := metricRequiredTags(spec, metricSpec)

	for _, emittedMetric := range emittedMetrics {
		tagsByKey := TagsToKeyValues(emittedMetric.Tags)

		for tag := range requiredTags {
			require.Contains(t, tagsByKey, tag, "metric %s missing required tag key %s", metricName, tag)
		}

		for key, values := range tagsByKey {
			_, allowed := requiredTags[key]
			require.True(t, allowed, "metric %s has unknown tag key %s", metricName, key)

			for _, value := range values {
				require.NotEmpty(t, value, "metric %s has empty value for tag %s", metricName, key)
				if expectedValue, ok := knownTagValues[key]; ok {
					require.Equal(t, expectedValue, value, "metric %s has unexpected value for tag %s", metricName, key)
				}
			}
		}
	}
}

func metricRequiredTags(spec *MetricsSpec, metricSpec MetricSpec) map[string]struct{} {
	requiredTags := make(map[string]struct{})
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, ok := spec.Tagsets[tagsetName]
		if !ok {
			continue
		}
		for _, tag := range tagsetSpec.Tags {
			requiredTags[tag] = struct{}{}
		}
	}
	for _, tag := range metricSpec.CustomTags {
		requiredTags[tag] = struct{}{}
	}
	return requiredTags
}
