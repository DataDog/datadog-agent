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

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestNVLinkGPMCollectorGetOrCreateGpmCollector(t *testing.T) {
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	expectedRxMetricID := nvml.GPM_METRIC_NVLINK_L1_RX_PER_SEC
	expectedTxMetricID := nvml.GPM_METRIC_NVLINK_L1_TX_PER_SEC

	mockLib := &mockGpmNvml{
		metricsGetFunc: func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			require.Equal(t, uint32(2), metrics.NumMetrics)
			for i := range metrics.Metrics[:metrics.NumMetrics] {
				require.Contains(t, []uint32{uint32(expectedRxMetricID), uint32(expectedTxMetricID)}, metrics.Metrics[i].MetricId)
				metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
			}
			return nvml.SUCCESS
		},
	}
	mockDevice := &mockGpmDevice{
		gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1},
		uuid:       "test-uuid-nvlink-gpm",
	}

	safenvml.WithMockNVML(t, mockLib)

	collector := &nvlinkGpmCollector{
		perPortCollector: make(map[int]*gpmCollector),
		device:           mockDevice,
	}
	gpmCollector, err := collector.getOrCreateGpmCollector(1)
	require.NoError(t, err)
	require.Len(t, gpmCollector.metricsToCollect, 2)
	require.Equal(t, gpmMetric{name: "nvlink.throughput.data.rx", metricType: metrics.GaugeType}, gpmCollector.metricsToCollect[expectedRxMetricID])
	require.Equal(t, gpmMetric{name: "nvlink.throughput.data.tx", metricType: metrics.GaugeType}, gpmCollector.metricsToCollect[expectedTxMetricID])

	cachedCollector, err := collector.getOrCreateGpmCollector(1)
	require.NoError(t, err)
	require.Same(t, gpmCollector, cachedCollector)
}

func TestNVLinkGPMCollectorGetOrCreateGpmCollectorRejectsOutOfRangePort(t *testing.T) {
	collector := &nvlinkGpmCollector{
		perPortCollector: make(map[int]*gpmCollector),
	}

	gpmCollector, err := collector.getOrCreateGpmCollector(maxNvlinkPorts)
	require.Nil(t, gpmCollector)
	require.ErrorIs(t, err, errUnsupportedDevice)
	require.ErrorContains(t, err, "port 18 is out of range")
}

func TestNVLinkGPMCollectorGetPortMetricsConvertsValuesAndSetsPriority(t *testing.T) {
	rxMetricID := nvml.GpmMetricId(int(baseNvlinkRxGpm) + 2)
	txMetricID := nvml.GpmMetricId(int(baseNvlinkTxGpm) + 2)
	mockLib := &mockGpmNvml{
		metricsGetFunc: func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			for i := range metrics.Metrics[:metrics.NumMetrics] {
				metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
				switch nvml.GpmMetricId(metrics.Metrics[i].MetricId) {
				case rxMetricID:
					metrics.Metrics[i].Value = 1.5
				case txMetricID:
					metrics.Metrics[i].Value = 2.25
				default:
					t.Fatalf("unexpected GPM metric ID %d", metrics.Metrics[i].MetricId)
				}
			}
			return nvml.SUCCESS
		},
	}
	mockDevice := &mockGpmDevice{
		gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1},
		uuid:       "test-uuid-nvlink-gpm",
	}
	safenvml.WithMockNVML(t, mockLib)
	safeLib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)
	collector := &nvlinkGpmCollector{
		perPortCollector: map[int]*gpmCollector{
			1: {
				lib:     safeLib,
				device:  mockDevice,
				samples: [sampleBufferSize]nvml.GpmSample{&gpmSample{id: 1}, &gpmSample{id: 2}},
				metricsToCollect: map[nvml.GpmMetricId]gpmMetric{
					rxMetricID: {name: "nvlink.throughput.data.rx", metricType: metrics.GaugeType},
					txMetricID: {name: "nvlink.throughput.data.tx", metricType: metrics.GaugeType},
				},
			},
		},
		device: mockDevice,
	}

	collectedMetrics, err := collector.getPortMetrics(1)
	require.NoError(t, err)
	require.Len(t, collectedMetrics, 2)

	valuesByName := make(map[string]float64, len(collectedMetrics))
	for _, metric := range collectedMetrics {
		require.Equal(t, metrics.GaugeType, metric.Type)
		require.Equal(t, High, metric.Priority)
		valuesByName[metric.Name] = metric.Value
	}
	require.Equal(t, 1.5*1024, valuesByName["nvlink.throughput.data.rx"])
	require.Equal(t, 2.25*1024, valuesByName["nvlink.throughput.data.tx"])
}

func TestNVLinkGPMCollectorCollectReturnsPartialMetricsAndErrors(t *testing.T) {
	metricID := nvml.GpmMetricId(int(baseNvlinkRxGpm) + 2)
	mockLib := &mockGpmNvml{
		metricsGetFunc: func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			for i := range metrics.Metrics[:metrics.NumMetrics] {
				metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
				metrics.Metrics[i].Value = 1
			}
			return nvml.SUCCESS
		},
	}
	safenvml.WithMockNVML(t, mockLib)
	safeLib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	collector := &nvlinkGpmCollector{
		perPortCollector: map[int]*gpmCollector{
			1: {
				lib:     safeLib,
				device:  &mockGpmDevice{gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1}, uuid: "test-uuid-nvlink-gpm"},
				samples: [sampleBufferSize]nvml.GpmSample{&gpmSample{id: 1}, &gpmSample{id: 2}},
				metricsToCollect: map[nvml.GpmMetricId]gpmMetric{
					metricID: {name: "nvlink.throughput.data.rx", metricType: metrics.GaugeType},
				},
			},
			2: {
				lib: safeLib,
				device: &mockGpmDevice{
					gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1},
					GpmSampleGetFunc: func(_ nvml.GpmSample) error {
						return errors.New("sample unavailable")
					},
					uuid: "test-uuid-nvlink-gpm",
				},
				samples: [sampleBufferSize]nvml.GpmSample{&gpmSample{id: 3}, &gpmSample{id: 4}},
				metricsToCollect: map[nvml.GpmMetricId]gpmMetric{
					metricID: {name: "nvlink.throughput.data.rx", metricType: metrics.GaugeType},
				},
			},
		},
	}

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.ErrorContains(t, err, "collect metrics for port 2")
	require.ErrorContains(t, err, "sample unavailable")
	require.Len(t, collectedMetrics, 1)
	require.Equal(t, "nvlink.throughput.data.rx", collectedMetrics[0].Name)
}
