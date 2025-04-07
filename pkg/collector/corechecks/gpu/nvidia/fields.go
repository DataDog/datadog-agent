// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"math"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type fieldsCollector struct {
	device       nvml.Device
	fieldMetrics []fieldValueMetric
}

func newFieldsCollector(device nvml.Device) (Collector, error) {
	c := &fieldsCollector{
		device: device,
	}
	c.fieldMetrics = append(c.fieldMetrics, metricNameToFieldID...) // copy all metrics to avoid modifying the original slice

	// Remove any unsupported fields, we also want to check if we have any fields left
	// to avoid doing unnecessary work
	err := c.removeUnsupportedFields()
	if err != nil {
		return nil, err
	}
	if len(c.fieldMetrics) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *fieldsCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

func (c *fieldsCollector) removeUnsupportedFields() error {
	fieldValues, err := c.getFieldValues()
	if err != nil {
		return err
	}

	for _, val := range fieldValues {
		if val.NvmlReturn == uint32(nvml.ERROR_NOT_SUPPORTED) {
			c.fieldMetrics = slices.DeleteFunc(c.fieldMetrics, func(fm fieldValueMetric) bool {
				return fm.fieldValueID == val.FieldId
			})
		}
	}

	return nil
}

func (c *fieldsCollector) getFieldValues() ([]nvml.FieldValue, error) {
	fields := make([]nvml.FieldValue, len(c.fieldMetrics))
	for i, metric := range c.fieldMetrics {
		fields[i].FieldId = metric.fieldValueID
		fields[i].ScopeId = metric.scopeID
	}

	ret := c.device.GetFieldValues(fields)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get field values: %s", nvml.ErrorString(ret))
	}

	return fields, nil
}

// Collect collects all the metrics from the given NVML device.
func (c *fieldsCollector) Collect() ([]Metric, error) {
	fields, err := c.getFieldValues()
	if err != nil {
		return nil, err
	}

	metrics := make([]Metric, 0, len(c.fieldMetrics))
	for i, val := range fields {
		name := metricNameToFieldID[i].name
		if val.NvmlReturn != uint32(nvml.SUCCESS) {
			err = multierror.Append(err, fmt.Errorf("failed to get field value %s: %s", name, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		value, convErr := fieldValueToNumber[float64](nvml.ValueType(val.ValueType), val.Value)
		if convErr != nil {
			err = multierror.Append(err, fmt.Errorf("failed to convert field value %s: %w", name, convErr))
		}

		metrics = append(metrics, Metric{
			Name:  name,
			Value: value,
			Type:  metricNameToFieldID[i].metricType},
		)
	}

	return metrics, err
}

// Name returns the name of the collector.
func (c *fieldsCollector) Name() CollectorName {
	return field
}

// fieldValueMetric represents a metric that can be retrieved using the NVML
// FieldValues API, and associates a name for that metric
type fieldValueMetric struct {
	name         string
	fieldValueID uint32 // No specific type, but these are constants prefixed with FI_DEV in the nvml package
	// some fields require scopeID to be filled for the GetFieldValues to work properly
	// (e.g: https://github.com/NVIDIA/nvidia-settings/blob/main/src/nvml.h#L2175-L2177)
	scopeID    uint32
	metricType metrics.MetricType
}

var metricNameToFieldID = []fieldValueMetric{
	{"memory.temperature", nvml.FI_DEV_MEMORY_TEMP, 0, metrics.GaugeType},
	// we don't want to use bandwidth fields as they are deprecated:
	// https://github.com/NVIDIA/nvidia-settings/blob/main/src/nvml.h#L2049-L2057
	// uint_max to collect the aggregated value summed up across all links (ref: https://github.com/NVIDIA/nvidia-settings/blob/main/src/nvml.h#L2175-L2177)
	{"nvlink.throughput.data.rx", nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX, math.MaxUint32, metrics.GaugeType},
	{"nvlink.throughput.data.tx", nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX, math.MaxUint32, metrics.GaugeType},
	{"nvlink.throughput.raw.rx", nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX, math.MaxUint32, metrics.GaugeType},
	{"nvlink.throughput.raw.tx", nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX, math.MaxUint32, metrics.GaugeType},
	{"nvlink.speed", nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON, 0, metrics.GaugeType},
	{"nvlink.nvswitch_connected", nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT, 0, metrics.GaugeType},
	{"nvlink.errors.crc.data", nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL, 0, metrics.CountType},
	{"nvlink.errors.crc.flit", nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL, 0, metrics.CountType},
	{"nvlink.errors.ecc", nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL, 0, metrics.CountType},
	{"nvlink.errors.recovery", nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL, 0, metrics.CountType},
	{"nvlink.errors.replay", nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL, 0, metrics.CountType},
	{"pci.replay_counter", nvml.FI_DEV_PCIE_REPLAY_COUNTER, 0, metrics.CountType},
	{"slowdown_temperature", nvml.FI_DEV_PERF_POLICY_THERMAL, 0, metrics.GaugeType},
}
