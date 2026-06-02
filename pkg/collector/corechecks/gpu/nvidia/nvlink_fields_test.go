// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestNVLinkFieldsCollectorQueriesAllConfiguredPorts(t *testing.T) {
	var requestedScopes []uint32
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *mock.Device) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			require.NotEmpty(t, fv)
			requestedScopes = append(requestedScopes, fv[0].ScopeId)
			for i := range fv {
				require.Equal(t, fv[0].ScopeId, fv[i].ScopeId, "all fields in a call should target the same NVLink port")
				testutil.ApplyMockFieldValue(&fv[i], testutil.DefaultFieldValues[fv[i].FieldId])
			}
			return nvml.SUCCESS
		}
	}))

	collector := &nvlinkFieldsCollector{
		device: device,
		ports:  []int{1, 2, 3},
		metrics: []nvlinkFieldValueMetric{
			{
				name:         "nvlink.tx.discards",
				fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
				metricType:   metrics.GaugeType,
			},
		},
	}

	_, err := collector.Collect()
	require.NoError(t, err)

	require.Equal(t, []uint32{0, 1, 2}, requestedScopes)
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

	device := setupMockDevice(t, testutil.WithScopedFieldValues(values))

	collector := &nvlinkFieldsCollector{
		device: device,
		ports:  []int{1, 2, 3},
		metrics: []nvlinkFieldValueMetric{
			{
				name:                "nvlink.throughput.data.rx",
				fieldValueID:        nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX,
				addTotalMetric:      true,
				metricType:          metrics.GaugeType,
				rateCalculationMode: PerSecondRateCalculation,
			},
			{
				name:                "nvlink.throughput.raw.tx",
				fieldValueID:        nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX,
				addTotalMetric:      true,
				metricType:          metrics.GaugeType,
				rateCalculationMode: PerSecondRateCalculation,
			},
			{
				name:         "nvlink.tx.discards",
				fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
				metricType:   metrics.GaugeType,
			},
		},
	}

	collected, err := collector.Collect()
	require.NoError(t, err)

	var dataRXValues []float64
	var rawTXValues []float64
	var discardValues []float64
	for _, metric := range collected {
		switch metric.Name {
		case "nvlink.throughput.data.rx":
			dataRXValues = append(dataRXValues, metric.Value)
		case "nvlink.throughput.raw.tx":
			rawTXValues = append(rawTXValues, metric.Value)
		case "nvlink.tx.discards":
			discardValues = append(discardValues, metric.Value)
		case "nvlink.throughput.data.rx.total":
			require.Equal(t, 60.0, metric.Value)
			require.Equal(t, metrics.GaugeType, metric.Type)
			require.Equal(t, PerSecondRateCalculation, metric.RateCalculationMode)
		case "nvlink.throughput.raw.tx.total":
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
}

func TestNVLinkFieldsCollectorDiscardsUnsupportedFieldMetrics(t *testing.T) {
	var requestedFieldsByScope = make(map[uint32][]uint32)
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *mock.Device) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			for i := range fv {
				requestedFieldsByScope[fv[i].ScopeId] = append(requestedFieldsByScope[fv[i].ScopeId], fv[i].FieldId)
				if fv[i].FieldId == nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS {
					fv[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
					continue
				}

				testutil.ApplyMockFieldValue(&fv[i], testutil.NewFieldValue(uint64(fv[i].ScopeId+1)))
			}
			return nvml.SUCCESS
		}
	}))

	collector := &nvlinkFieldsCollector{
		device: device,
		ports:  []int{1, 2},
		metrics: []nvlinkFieldValueMetric{
			{
				name:                "nvlink.throughput.data.rx",
				fieldValueID:        nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX,
				addTotalMetric:      true,
				metricType:          metrics.GaugeType,
				rateCalculationMode: PerSecondRateCalculation,
			},
			{
				name:         "nvlink.tx.discards",
				fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
				metricType:   metrics.GaugeType,
			},
		},
	}

	collected, err := collector.Collect()
	require.NoError(t, err)

	for _, metric := range collected {
		require.NotEqual(t, "nvlink.tx.discards", metric.Name)
	}

	require.Len(t, collector.metrics, 1)
	require.Equal(t, uint32(nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX), collector.metrics[0].fieldValueID)
	require.Contains(t, requestedFieldsByScope[0], uint32(nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS))
	require.NotContains(t, requestedFieldsByScope[1], uint32(nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS))
}

func TestNVLinkFieldsCollectorCollectDoesNotPanicWhenMetricsBecomeEmpty(t *testing.T) {
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *mock.Device) {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			if len(fv) == 0 {
				panic("GetFieldValues called with empty fields")
			}
			for i := range fv {
				fv[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
			}
			return nvml.SUCCESS
		}
	}))

	collector := &nvlinkFieldsCollector{
		device: device,
		ports:  []int{1, 2},
		metrics: []nvlinkFieldValueMetric{
			{
				name:         "nvlink.tx.discards",
				fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS,
				metricType:   metrics.GaugeType,
			},
		},
	}

	var err error
	require.NotPanics(t, func() {
		_, err = collector.Collect()
	})
	require.ErrorIs(t, err, errUnsupportedDevice)
	require.ErrorContains(t, err, "no metrics to collect")
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

			// Run through RemoveDuplicateMetrics, same as the real check
			deduped := RemoveDuplicateMetrics(map[CollectorName][]*Metric{
				nvlinkFields: collected,
			})

			var nvlinkSpeed []*Metric
			for _, m := range deduped {
				if m.Name == "nvlink.speed" {
					nvlinkSpeed = append(nvlinkSpeed, m)
				}
			}

			require.Len(t, nvlinkSpeed, 1, "exactly one nvlink.speed metric should survive dedup")
			require.Equal(t, tt.expectPriority, nvlinkSpeed[0].Priority)
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
		switch metric.name {
		case "nvlink.errors.effective":
			foundNvlinkEffective = true
		}
	}

	require.False(t, foundNvlinkEffective, "nvlink.errors.effective should be removed when INVALID_ARGUMENT is explicitly mapped to unsupported")
}
