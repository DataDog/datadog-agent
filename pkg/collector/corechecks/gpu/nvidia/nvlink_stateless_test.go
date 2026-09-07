// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestNVLinkStatelessCollectorUnsupportedBeforeHopper(t *testing.T) {
	device := setupMockDevice(t, testutil.WithArchitecture("ampere"), testutil.WithNVLinkLinkCount(4))
	_, err := newNVLinkStatelessCollector(device, nil)
	require.ErrorIs(t, err, errUnsupportedDevice)
}

func TestNVLinkStatelessCollectorCollectsPerLinkErrorCounters(t *testing.T) {
	counterValues := map[nvml.NvLinkErrorCounter]uint64{
		nvml.NVLINK_ERROR_DL_REPLAY:    1,
		nvml.NVLINK_ERROR_DL_RECOVERY:  2,
		nvml.NVLINK_ERROR_DL_CRC_FLIT:  3,
		nvml.NVLINK_ERROR_DL_ECC_DATA:  4,
	}

	device := setupMockDevice(t,
		testutil.WithArchitecture("hopper"),
		testutil.WithNVLinkLinkCount(2),
		testutil.WithCustomHook(func(d *testutil.MockDevice) {
			d.GetNvLinkErrorCounterFunc = func(_ int, counter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
				value, ok := counterValues[counter]
				if !ok {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return value, nvml.SUCCESS
			}
		}),
	)

	collector, err := newNVLinkStatelessCollector(device, nil)
	require.NoError(t, err)

	collected, err := collector.Collect()
	require.NoError(t, err)

	metricsByName := make(map[string][]*Metric)
	for _, sample := range requireMetrics(t, collected) {
		metricsByName[sample.Name] = append(metricsByName[sample.Name], sample)
	}

	require.Len(t, metricsByName["nvlink.errors.replay"], 2)
	require.Len(t, metricsByName["nvlink.errors.recovery"], 2)
	require.Len(t, metricsByName["nvlink.errors.crc.flit"], 2)
	require.Len(t, metricsByName["nvlink.errors.ecc"], 2)

	for _, metric := range metricsByName["nvlink.errors.replay"] {
		require.Equal(t, Medium, metric.Priority())
		require.Equal(t, float64(1), metric.Value)
		require.Equal(t, metrics.GaugeType, metric.Type)
	}
}

func TestNVLinkStatelessCollectorWinsOverNVLinkFieldsOnHopper(t *testing.T) {
	counterValues := map[nvml.NvLinkErrorCounter]uint64{
		nvml.NVLINK_ERROR_DL_REPLAY:   10,
		nvml.NVLINK_ERROR_DL_RECOVERY: 20,
		nvml.NVLINK_ERROR_DL_CRC_FLIT: 30,
		nvml.NVLINK_ERROR_DL_ECC_DATA: 40,
	}

	device := setupMockDevice(t,
		testutil.WithArchitecture("hopper"),
		testutil.WithNVLinkLinkCount(1),
		testutil.WithCustomHook(func(d *testutil.MockDevice) {
			d.GetNvLinkErrorCounterFunc = func(_ int, counter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
				return counterValues[counter], nvml.SUCCESS
			}
		}),
	)

	fieldsCollector, err := newNVLinkFieldsCollector(device, nil)
	require.NoError(t, err)
	statelessCollector, err := newNVLinkStatelessCollector(device, nil)
	require.NoError(t, err)

	fieldsSamples, err := fieldsCollector.Collect()
	require.NoError(t, err)
	statelessSamples, err := statelessCollector.Collect()
	require.NoError(t, err)

	deduped := requireMetrics(t, RemoveDuplicateSamples(map[CollectorName][]Sample{
		nvlinkFields:    fieldsSamples,
		nvlinkStateless: statelessSamples,
	}))

	replayMetrics := filterMetricsByName(deduped, "nvlink.errors.replay")
	require.Len(t, replayMetrics, 1)
	require.Equal(t, Medium, replayMetrics[0].Priority())
	require.Equal(t, float64(10), replayMetrics[0].Value)
}

func filterMetricsByName(metrics []*Metric, name string) []*Metric {
	var out []*Metric
	for _, metric := range metrics {
		if metric.Name == name {
			out = append(out, metric)
		}
	}
	return out
}
