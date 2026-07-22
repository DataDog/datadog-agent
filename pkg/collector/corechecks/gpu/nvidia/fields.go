// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type fieldsCollector struct {
	device       ddnvml.Device
	fieldMetrics []fieldValueMetric
}

func newFieldsCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	c := &fieldsCollector{
		device: device,
	}
	c.fieldMetrics = append(c.fieldMetrics, allFieldMetrics...) // copy all metrics to avoid modifying the original slice

	// Remove any unsupported fields, we also want to check if we have any fields left
	// to avoid doing unnecessary work
	c.removeUnsupportedMetrics()
	if len(c.fieldMetrics) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *fieldsCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *fieldsCollector) removeUnsupportedMetrics() {
	fieldValues, err := c.getFieldValues()
	if err != nil {
		// If the entire field values API is unsupported, remove all metrics
		if ddnvml.IsAPIUnsupportedOnDevice(err, c.device) {
			log.Debugf("GPU fields collector removing all field metrics for device %s because GetFieldValues is unsupported", c.DeviceUUID())
			c.fieldMetrics = nil
		}
		// Otherwise, do nothing and keep all metrics
		return
	}

	// Remove individual unsupported fields
	for _, val := range fieldValues {
		if val.NvmlReturn == uint32(nvml.ERROR_NOT_SUPPORTED) || (val.NvmlReturn == uint32(nvml.ERROR_INVALID_ARGUMENT)) {
			fieldValueIdx := slices.IndexFunc(c.fieldMetrics, func(fm fieldValueMetric) bool {
				return fm.fieldValueID == val.FieldId
			})
			if fieldValueIdx == -1 {
				log.Warnf("Unexpected field ID %d returned for device %s (scope_id=%d): return value is %s",
					val.FieldId,
					c.DeviceUUID(),
					val.ScopeId,
					nvml.ErrorString(nvml.Return(val.NvmlReturn)),
				)
				continue
			}

			fieldMetric := c.fieldMetrics[fieldValueIdx]
			if val.NvmlReturn == uint32(nvml.ERROR_INVALID_ARGUMENT) && !fieldMetric.markUnsupportedOnInvalidArgument {
				continue
			}

			log.Debugf("GPU fields collector removing unsupported metric %s for device %s (field_id=%d scope_id=%d)",
				fieldMetric.name,
				c.DeviceUUID(),
				fieldMetric.fieldValueID,
				fieldMetric.scopeID,
			)

			c.fieldMetrics = slices.Delete(c.fieldMetrics, fieldValueIdx, fieldValueIdx+1)
		}
	}
}

func (c *fieldsCollector) getFieldValues() ([]nvml.FieldValue, error) {
	fields := make([]nvml.FieldValue, len(c.fieldMetrics))
	for i, metric := range c.fieldMetrics {
		fields[i].FieldId = metric.fieldValueID
		fields[i].ScopeId = metric.scopeID
	}

	err := c.device.GetFieldValues(fields)
	if err != nil {
		return nil, err
	}

	return fields, nil
}

// Collect collects all the metrics from the given NVML device.
func (c *fieldsCollector) Collect() ([]*Metric, error) {
	fields, err := c.getFieldValues()
	if err != nil {
		return nil, err
	}

	metrics := make([]Metric, 0, len(c.fieldMetrics))
	var errs []error
	for i, val := range fields {
		name := c.fieldMetrics[i].name
		if val.NvmlReturn != uint32(nvml.SUCCESS) {
			errs = append(errs, fmt.Errorf("failed to get field value %s: %s", name, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		value, convErr := fieldValueToNumber[float64](nvml.ValueType(val.ValueType), val.Value)
		if convErr != nil {
			errs = append(errs, fmt.Errorf("failed to convert field value %s: %w", name, convErr))
		}

		metrics = append(metrics, Metric{
			Name:                name,
			Value:               value,
			Type:                c.fieldMetrics[i].metricType,
			Priority:            c.fieldMetrics[i].priority,
			RateCalculationMode: c.fieldMetrics[i].rateCalculationMode,
		})
	}

	return metricValuesToPointers(metrics), errors.Join(errs...)
}

// Name returns the name of the collector.
func (c *fieldsCollector) Name() CollectorName {
	return field
}

// fieldValueMetric represents a metric that can be retrieved using the NVML
// FieldValues API, and associates a name for that metric.
// When multiple field IDs can emit the same metric name, priority determines
// which one is preferred: higher priority wins. Duplicate resolution is handled
// by RemoveDuplicateMetrics at collection time.
type fieldValueMetric struct {
	name         string
	fieldValueID uint32 // No specific type, but these are constants prefixed with FI_DEV in the nvml package
	// some fields require scopeID to be filled for the GetFieldValues to work properly
	// (e.g: https://github.com/NVIDIA/nvidia-settings/blob/main/src/nvml.h#L2175-L2177)
	scopeID uint32
	// Some fields on older architectures return INVALID_ARGUMENT immediately
	// instead of cleanly reporting ERROR_NOT_SUPPORTED. Mark those fields here
	// so collector initialization can treat INVALID_ARGUMENT as unsupported.
	markUnsupportedOnInvalidArgument bool
	metricType                       metrics.MetricType
	rateCalculationMode              RateCalculationMode
	priority                         MetricPriority
}

// allFieldMetrics lists all candidate field-value metrics. When multiple entries
// share the same metric name, they are alternatives for the same logical metric;
// the highest-priority one is selected by RemoveDuplicateMetrics at collection time.
//
// Low (default) = legacy fields (pre-NVLink5), MediumLow = newer per-link fields
// introduced with NVLink5/Blackwell (field IDs 164+). The newer fields use
// scopeId to specify the link index and support >12 links.
var allFieldMetrics = []fieldValueMetric{
	// -- Non-NVLink metrics (no alternatives) --
	{name: "memory.temperature", fieldValueID: nvml.FI_DEV_MEMORY_TEMP, metricType: metrics.GaugeType},
	{name: "pci.replay_counter", fieldValueID: nvml.FI_DEV_PCIE_REPLAY_COUNTER, metricType: metrics.GaugeType},
	{name: "slowdown_temperature", fieldValueID: nvml.FI_DEV_PERF_POLICY_THERMAL, metricType: metrics.GaugeType},

	// -- C2C link error counters --
	{name: "c2c.errors.interrupt", fieldValueID: nvml.FI_DEV_C2C_LINK_ERROR_INTR, markUnsupportedOnInvalidArgument: true, metricType: metrics.GaugeType},
	{name: "c2c.errors.replay", fieldValueID: nvml.FI_DEV_C2C_LINK_ERROR_REPLAY, markUnsupportedOnInvalidArgument: true, metricType: metrics.GaugeType},
	{name: "c2c.errors.replay.b2b", fieldValueID: nvml.FI_DEV_C2C_LINK_ERROR_REPLAY_B2B, markUnsupportedOnInvalidArgument: true, metricType: metrics.GaugeType},

	// -- NVSwitch connection --
	{name: "nvlink.nvswitch_connected", fieldValueID: nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT, metricType: metrics.GaugeType},
}
