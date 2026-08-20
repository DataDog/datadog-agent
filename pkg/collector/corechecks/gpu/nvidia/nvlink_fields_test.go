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

func TestNVLinkFieldsCollectorQueriesAllConfiguredPorts(t *testing.T) {
	var requests [][]nvml.FieldValue
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			require.NotEmpty(t, fv)
			for i := range fv {
				if fv[i].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
					testutil.ApplyMockFieldValue(&fv[i], testutil.NewFieldValue(3))
					continue
				}
				testutil.ApplyMockFieldValue(&fv[i], testutil.DefaultFieldValues[fv[i].FieldId])
			}
			if fv[0].FieldId != nvml.FI_DEV_NVLINK_LINK_COUNT {
				requests = append(requests, append([]nvml.FieldValue(nil), fv...))
			}
			return nvml.SUCCESS
		}
	}), testutil.WithNVLinkLinkCount(3))

	collector, err := newNVLinkFieldsCollectorWithMetrics(device, map[uint32]nvlinkFieldValueMetric{
		nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS: {
			name:         "nvlink.tx.discards",
			fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
			metricType:   metrics.GaugeType,
		},
		nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS: {
			name:         "nvlink.errors.rx",
			fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS,
			metricType:   metrics.GaugeType,
		},
	})
	require.NoError(t, err)
	_, err = collector.Collect()
	require.NoError(t, err)

	require.Len(t, requests, 4, "one initialization request per port plus one batched collection request")
	var requestedFieldsAndScopes [][2]uint32
	for _, request := range requests[3] {
		requestedFieldsAndScopes = append(requestedFieldsAndScopes, [2]uint32{request.FieldId, request.ScopeId})
	}
	require.ElementsMatch(t, [][2]uint32{
		{nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS, 0},
		{nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS, 1},
		{nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS, 2},
		{nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS, 0},
		{nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS, 1},
		{nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS, 2},
	}, requestedFieldsAndScopes)
}

func TestNVLinkFieldsCollectorQueriesForcedScopeForEachPort(t *testing.T) {
	var requests [][]nvml.FieldValue
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			for i := range fv {
				if fv[i].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
					testutil.ApplyMockFieldValue(&fv[i], testutil.NewFieldValue(3))
					continue
				}
				testutil.ApplyMockFieldValue(&fv[i], testutil.DefaultFieldValues[fv[i].FieldId])
			}
			if fv[0].FieldId != nvml.FI_DEV_NVLINK_LINK_COUNT {
				requests = append(requests, append([]nvml.FieldValue(nil), fv...))
			}
			return nvml.SUCCESS
		}
	}), testutil.WithNVLinkLinkCount(3))

	scopeID := uint32(0)
	collector, err := newNVLinkFieldsCollectorWithMetrics(device, map[uint32]nvlinkFieldValueMetric{
		nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON: {
			name:              "nvlink.speed",
			fieldValueID:      nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON,
			forceScopeIDValue: &scopeID,
			metricType:        metrics.GaugeType,
		},
	})
	require.NoError(t, err)
	collected, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, requests, 4)

	var commonSpeedRequests []nvml.FieldValue
	for _, request := range requests[3] {
		if request.FieldId == nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON {
			commonSpeedRequests = append(commonSpeedRequests, request)
		}
	}
	require.Len(t, commonSpeedRequests, 1)
	for _, request := range commonSpeedRequests {
		require.Equal(t, uint32(0), request.ScopeId)
	}

	var speeds []*Metric
	for _, metric := range collected {
		if metric.Name == "nvlink.speed" {
			speeds = append(speeds, metric)
		}
	}
	require.Len(t, speeds, 3)
	require.ElementsMatch(t, []string{
		nvlinkPortTag(1),
		nvlinkPortTag(2),
		nvlinkPortTag(3),
	}, []string{
		speeds[0].Tags[0],
		speeds[1].Tags[0],
		speeds[2].Tags[0],
	})
}

func TestNVLinkFieldsCollectorAddsTotals(t *testing.T) {
	values := map[uint32]map[uint32]testutil.MockFieldValue{
		nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX: {
			0: testutil.NewFieldValue(10),
			1: testutil.NewFieldValue(20),
			2: testutil.NewFieldValue(30),
		},
		nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX: {
			0: testutil.NewFieldValue(1),
			1: testutil.NewFieldValue(2),
			2: testutil.NewFieldValue(3),
		},
		nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS: {
			0: testutil.NewFieldValue(100),
			1: testutil.NewFieldValue(200),
			2: testutil.NewFieldValue(300),
		},
	}

	device := setupMockDevice(t, testutil.WithScopedFieldValues(values), testutil.WithNVLinkLinkCount(3))

	collector, err := newNVLinkFieldsCollectorWithMetrics(device, map[uint32]nvlinkFieldValueMetric{
		nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX: {
			name:                "nvlink.throughput.data.rx",
			fieldValueID:        nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX,
			addTotalMetric:      true,
			metricType:          metrics.GaugeType,
			rateCalculationMode: PerSecondRateCalculation,
		},
		nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX: {
			name:                "nvlink.throughput.raw.tx",
			fieldValueID:        nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX,
			addTotalMetric:      true,
			metricType:          metrics.GaugeType,
			rateCalculationMode: PerSecondRateCalculation,
		},
		nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS: {
			name:         "nvlink.tx.discards",
			fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
			metricType:   metrics.GaugeType,
		},
	})
	require.NoError(t, err)
	collected, err := collector.Collect()
	require.NoError(t, err)

	var dataRXValues []float64
	var rawTXValues []float64
	var discardValues []float64
	var dataRXTotalCount int
	var rawTXTotalCount int
	for _, metric := range requireMetrics(t, collected) {
		switch metric.Name {
		case "nvlink.throughput.data.rx":
			dataRXValues = append(dataRXValues, metric.Value)
		case "nvlink.throughput.raw.tx":
			rawTXValues = append(rawTXValues, metric.Value)
		case "nvlink.tx.discards":
			discardValues = append(discardValues, metric.Value)
		case "nvlink.throughput.data.rx.total":
			dataRXTotalCount++
			require.Equal(t, 60.0, metric.Value)
			require.Equal(t, metrics.GaugeType, metric.Type)
			require.Equal(t, PerSecondRateCalculation, metric.RateCalculationMode)
		case "nvlink.throughput.raw.tx.total":
			rawTXTotalCount++
			require.Equal(t, 6.0, metric.Value)
			require.Equal(t, metrics.GaugeType, metric.Type)
			require.Equal(t, PerSecondRateCalculation, metric.RateCalculationMode)
		case "nvlink.tx.discards.total":
			t.Fatalf("non-total metric %s should not emit a total", metric.Name)
		}
	}

	require.ElementsMatch(t, []float64{10, 20, 30}, dataRXValues)
	require.ElementsMatch(t, []float64{1, 2, 3}, rawTXValues)
	require.ElementsMatch(t, []float64{100, 200, 300}, discardValues)
	require.Equal(t, 1, dataRXTotalCount, "expected exactly one data RX total metric")
	require.Equal(t, 1, rawTXTotalCount, "expected exactly one raw TX total metric")
}

func TestNVLinkFieldsCollectorDiscardsUnsupportedFieldMetrics(t *testing.T) {
	var requestedFieldsByScope = make(map[uint32][]uint32)
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			for i := range fv {
				requestedFieldsByScope[fv[i].ScopeId] = append(requestedFieldsByScope[fv[i].ScopeId], fv[i].FieldId)
				if fv[i].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
					testutil.ApplyMockFieldValue(&fv[i], testutil.NewFieldValue(2))
					continue
				}
				if fv[i].FieldId == nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS {
					fv[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
					continue
				}

				testutil.ApplyMockFieldValue(&fv[i], testutil.NewFieldValue(uint64(fv[i].ScopeId+1)))
			}
			return nvml.SUCCESS
		}
	}), testutil.WithNVLinkLinkCount(2))

	collector, err := newNVLinkFieldsCollectorWithMetrics(device, map[uint32]nvlinkFieldValueMetric{
		nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX: {
			name:                "nvlink.throughput.data.rx",
			fieldValueID:        nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX,
			addTotalMetric:      true,
			metricType:          metrics.GaugeType,
			rateCalculationMode: PerSecondRateCalculation,
		},
		nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS: {
			name:         "nvlink.tx.discards",
			fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
			metricType:   metrics.GaugeType,
		},
	})
	require.NoError(t, err)
	collected, err := collector.Collect()
	require.NoError(t, err)

	for _, metric := range requireMetrics(t, collected) {
		require.NotEqual(t, "nvlink.tx.discards", metric.Name)
	}

	require.Contains(t, requestedFieldsByScope[0], uint32(nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS))
	require.NotContains(t, requestedFieldsByScope[1], uint32(nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS))
}

func TestNVLinkFieldsCollectorReturnsErrorsForUnsupportedCollectedFields(t *testing.T) {
	collecting := false
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			if len(fv) == 0 {
				panic("GetFieldValues called with empty fields")
			}
			for i := range fv {
				if fv[i].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
					testutil.ApplyMockFieldValue(&fv[i], testutil.NewFieldValue(2))
					continue
				}
				if collecting {
					fv[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
					continue
				}
				testutil.ApplyMockFieldValue(&fv[i], testutil.DefaultFieldValues[fv[i].FieldId])
			}
			return nvml.SUCCESS
		}
	}), testutil.WithNVLinkLinkCount(2))

	collector, err := newNVLinkFieldsCollectorWithMetrics(device, map[uint32]nvlinkFieldValueMetric{
		nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS: {
			name:         "nvlink.tx.discards",
			fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
			metricType:   metrics.GaugeType,
		},
	})
	require.NoError(t, err)
	collecting = true

	var collected []*Metric
	var collectErr error
	require.NotPanics(t, func() {
		collected, collectErr = collector.Collect()
	})
	require.Empty(t, collected)
	require.ErrorContains(t, collectErr, "failed to get field value nvlink.tx.discards for ports [1]")
	require.ErrorContains(t, collectErr, "failed to get field value nvlink.tx.discards for ports [2]")
}

func TestFieldsCollector_NvlinkSpeedPriority(t *testing.T) {
	tests := []struct {
		name             string
		unsupportedField uint32 // field ID to mark as unsupported; 0 means all supported
		expectPriority   MetricPriority
		expectValue      float64
	}{
		{
			name:           "both supported, newer wins after dedup",
			expectPriority: MediumLow,
			expectValue:    float64(testutil.DefaultFieldValues[nvml.FI_DEV_NVLINK_GET_SPEED].Value),
		},
		{
			name:             "newer unsupported, legacy selected",
			unsupportedField: nvml.FI_DEV_NVLINK_GET_SPEED,
			expectPriority:   Low,
			expectValue:      float64(testutil.DefaultFieldValues[nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON].Value),
		},
		{
			name:             "legacy unsupported, newer selected",
			unsupportedField: nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON,
			expectPriority:   MediumLow,
			expectValue:      float64(testutil.DefaultFieldValues[nvml.FI_DEV_NVLINK_GET_SPEED].Value),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []testutil.NvmlMockOption{testutil.WithNVLinkLinkCount(1)}
			if tt.unsupportedField != 0 {
				opts = append(opts, testutil.WithUnsupportedFields(tt.unsupportedField))
			}
			device := setupMockDevice(t, opts...)

			collector, err := newNVLinkFieldsCollector(device, nil)
			require.NoError(t, err)

			collected, err := collector.Collect()
			require.NoError(t, err)

			// Run through RemoveDuplicateSamples, same as the real check
			deduped := requireMetrics(t, RemoveDuplicateSamples(map[CollectorName][]Sample{
				nvlinkFields: collected,
			}))

			var nvlinkSpeed []*Metric
			for _, m := range deduped {
				if m.Name == "nvlink.speed" {
					nvlinkSpeed = append(nvlinkSpeed, m)
				}
			}

			require.Len(t, nvlinkSpeed, 1, "exactly one nvlink.speed metric should survive dedup")
			require.Equal(t, tt.expectPriority, nvlinkSpeed[0].Priority())
			require.Equal(t, tt.expectValue, nvlinkSpeed[0].Value)
		})
	}
}

func TestNVlinkFieldsCollectorTreatsInvalidArgumentAsUnsupportedOnlyWhenConfigured(t *testing.T) {
	device := setupMockDevice(t, testutil.WithInvalidArgumentFields(nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS), testutil.WithNVLinkLinkCount(1))

	collector, err := newNVLinkFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*nvlinkFieldsCollector)
	require.True(t, ok, "expected *nvlinkFieldsCollector")

	foundNvlinkEffective := false
	for _, metric := range fc.metrics {
		if metric.name == "nvlink.errors.effective" {
			foundNvlinkEffective = true
		}
	}

	require.False(t, foundNvlinkEffective, "nvlink.errors.effective should be removed when INVALID_ARGUMENT is explicitly mapped to unsupported")
}
