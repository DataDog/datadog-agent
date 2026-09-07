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

func TestNVLinkStatelessCollectorCollectsHopperErrorCounters(t *testing.T) {
	counterValues := map[nvml.NvLinkErrorCounter]uint64{
		nvml.NVLINK_ERROR_DL_REPLAY:   1,
		nvml.NVLINK_ERROR_DL_RECOVERY: 2,
		nvml.NVLINK_ERROR_DL_CRC_FLIT: 3,
		nvml.NVLINK_ERROR_DL_ECC_DATA: 4,
	}
	expectedByName := map[string]float64{
		"nvlink.errors.replay":   1,
		"nvlink.errors.recovery": 2,
		"nvlink.errors.crc.flit": 3,
		"nvlink.errors.ecc":      4,
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

	statelessCollector, err := newNVLinkStatelessCollector(device, nil)
	require.NoError(t, err)
	statelessSamples, err := statelessCollector.Collect()
	require.NoError(t, err)

	counts := make(map[string]int, len(expectedByName))
	for _, metric := range requireMetrics(t, statelessSamples) {
		require.Equal(t, Medium, metric.Priority())
		require.Equal(t, metrics.GaugeType, metric.Type)
		require.Equal(t, expectedByName[metric.Name], metric.Value)
		counts[metric.Name]++
	}
	for name := range expectedByName {
		require.Equal(t, 2, counts[name])
	}

	fieldsCollector, err := newNVLinkFieldsCollector(device, nil)
	require.NoError(t, err)
	fieldsSamples, err := fieldsCollector.Collect()
	require.NoError(t, err)

	deduped := requireMetrics(t, RemoveDuplicateSamples(map[CollectorName][]Sample{
		nvlinkFields:    fieldsSamples,
		nvlinkStateless: statelessSamples,
	}))

	var replayMetrics []*Metric
	for _, metric := range deduped {
		if metric.Name == "nvlink.errors.replay" {
			replayMetrics = append(replayMetrics, metric)
		}
	}
	require.Len(t, replayMetrics, 2)
	for _, metric := range replayMetrics {
		require.Equal(t, Medium, metric.Priority())
		require.Equal(t, float64(1), metric.Value)
	}
}
