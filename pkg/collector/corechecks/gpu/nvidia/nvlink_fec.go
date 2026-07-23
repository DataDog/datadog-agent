// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"math"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	nvlinkFECHistoryMetricName          = "nvlink.errors.fec"
	nvlinkFECNoErrorsMetricName         = "nvlink.errors.fec.none"
	nvlinkFECLightErrorsMetricName      = "nvlink.errors.fec.light"
	nvlinkFECHeavyErrorsMetricName      = "nvlink.errors.fec.heavy"
	nvlinkFECLightErrorThresholdConfig  = "gpu.nvlink.fec_light_error_threshold"
	defaultNVLinkFECLightErrorThreshold = 3
)

var nvlinkFECHistoryFieldIDs = []uint32{
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_0,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_1,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_2,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_3,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_4,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_5,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_6,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_7,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_8,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_9,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_10,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_11,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_12,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_13,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_14,
	nvml.FI_DEV_NVLINK_COUNT_FEC_HISTORY_15,
}

type nvlinkFECCollector struct {
	device                    ddnvml.Device
	ports                     []int
	lightErrorBucketThreshold int
}

func newNVLinkFECCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	lightErrorBucketThreshold := defaultNVLinkFECLightErrorThreshold
	if deps != nil && deps.Config != nil && deps.Config.GetInt(nvlinkFECLightErrorThresholdConfig) > 0 {
		if v := deps.Config.GetInt(nvlinkFECLightErrorThresholdConfig); v > 0 {
			lightErrorBucketThreshold = v
		}
	}

	c := &nvlinkFECCollector{
		device:                    device,
		lightErrorBucketThreshold: lightErrorBucketThreshold,
	}

	ports, err := getSupportedNvlinkPorts(device, c.getPortMetrics)
	if err != nil {
		return nil, err
	}

	c.ports = ports

	return c, nil
}

func (c *nvlinkFECCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *nvlinkFECCollector) Name() CollectorName {
	return nvlinkFEC
}

func (c *nvlinkFECCollector) Collect() ([]*Metric, error) {
	var (
		allMetrics []*Metric
		multiErr   []error
	)

	for _, port := range c.ports {
		metrics, err := c.getPortMetrics(port)
		allMetrics = append(allMetrics, metrics...)
		if err != nil {
			multiErr = append(multiErr, fmt.Errorf("get port metrics for port %d: %w", port, err))
			continue
		}
	}

	return allMetrics, errors.Join(multiErr...)
}

func (c *nvlinkFECCollector) getPortMetrics(port int) ([]*Metric, error) {
	fields := make([]nvml.FieldValue, len(nvlinkFECHistoryFieldIDs))
	scopeID := uint32(port - 1)
	for i, fieldID := range nvlinkFECHistoryFieldIDs {
		fields[i] = nvml.FieldValue{
			FieldId: fieldID,
			ScopeId: scopeID,
		}
	}

	if err := c.device.GetFieldValues(fields); err != nil {
		return nil, fmt.Errorf("get FEC history field values for scope %d: %w", scopeID, err)
	}

	var fecMetrics []*Metric
	var multiErr []error
	fecSeverityCounts := make([]float64, 3)
	for bucket, fieldValue := range fields {
		if fieldValue.NvmlReturn != uint32(nvml.SUCCESS) {
			multiErr = append(multiErr, ddnvml.NewNvmlAPIErrorOrNil(fmt.Sprintf("GetFieldValues(field=%d, scope=%d)", fieldValue.FieldId, scopeID), nvml.Return(fieldValue.NvmlReturn)))
			continue
		}

		count, err := fieldValueToNumber[uint64](nvml.ValueType(fieldValue.ValueType), fieldValue.Value)
		if err != nil {
			multiErr = append(multiErr, fmt.Errorf("convert FEC history field %d for scope %d: %w", fieldValue.FieldId, scopeID, err))
			continue
		}
		if count > math.MaxInt64 {
			multiErr = append(multiErr, fmt.Errorf("FEC history field %d for scope %d exceeds int64: %d", fieldValue.FieldId, scopeID, count))
			continue
		}

		histBounds := [2]float64{float64(bucket), float64(bucket)}
		metric := &Metric{
			Name:     nvlinkFECHistoryMetricName,
			Type:     metrics.HistogramType,
			Value:    float64(count),
			Priority: Medium,
			Tags:     []string{nvlinkPortTag(port)},
			HistogramBucket: &Bucket{
				Bounds:    histBounds,
				Monotonic: true,
			},
		}

		fecMetrics = append(fecMetrics, metric)
		switch {
		case bucket == 0:
			fecSeverityCounts[0] += float64(count)
		case bucket <= c.lightErrorBucketThreshold:
			fecSeverityCounts[1] += float64(count)
		default:
			fecSeverityCounts[2] += float64(count)
		}
	}

	// If we have partial errors we can't emit the grouped metrics as they're not complete.
	if len(multiErr) == 0 {
		fecMetrics = append(fecMetrics, c.fecSeverityMetrics(port, fecSeverityCounts)...)
	}

	return fecMetrics, errors.Join(multiErr...)
}

func (c *nvlinkFECCollector) fecSeverityMetrics(port int, counts []float64) []*Metric {
	return []*Metric{
		c.fecSeverityMetric(nvlinkFECNoErrorsMetricName, port, counts[0]),
		c.fecSeverityMetric(nvlinkFECLightErrorsMetricName, port, counts[1]),
		c.fecSeverityMetric(nvlinkFECHeavyErrorsMetricName, port, counts[2]),
	}
}

func (c *nvlinkFECCollector) fecSeverityMetric(name string, port int, count float64) *Metric {
	return &Metric{
		Name:                name,
		Type:                metrics.GaugeType,
		Value:               count,
		Priority:            Medium,
		Tags:                []string{nvlinkPortTag(port)},
		RateCalculationMode: PerSecondRateCalculation,
	}
}
