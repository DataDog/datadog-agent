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
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const computeRate = true

type lastPoint struct {
	value     float64
	timestamp time.Time
}

type fieldsCollector struct {
	device       ddnvml.Device
	fieldMetrics []fieldValueMetric
	lastPoints   map[string]lastPoint
	now          func() time.Time
}

func newFieldsCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	c := &fieldsCollector{
		device:     device,
		lastPoints: make(map[string]lastPoint),
		now:        time.Now,
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
			c.fieldMetrics = nil
		}
		// Otherwise, do nothing and keep all metrics
		return
	}

	// Remove individual unsupported fields
	for _, val := range fieldValues {
		if val.NvmlReturn == uint32(nvml.ERROR_NOT_SUPPORTED) {
			c.fieldMetrics = slices.DeleteFunc(c.fieldMetrics, func(fm fieldValueMetric) bool {
				return fm.fieldValueID == val.FieldId
			})
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
func (c *fieldsCollector) Collect() ([]Metric, error) {
	now := c.now()
	fields, err := c.getFieldValues()
	if err != nil {
		return nil, err
	}

	metrics := make([]Metric, 0, len(c.fieldMetrics))
	for i, val := range fields {
		name := c.fieldMetrics[i].name
		if val.NvmlReturn != uint32(nvml.SUCCESS) {
			err = multierror.Append(err, fmt.Errorf("failed to get field value %s: %s", name, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		value, convErr := fieldValueToNumber[float64](nvml.ValueType(val.ValueType), val.Value)
		if convErr != nil {
			err = multierror.Append(err, fmt.Errorf("failed to convert field value %s: %w", name, convErr))
		}

		if c.fieldMetrics[i].computeRate {
			currPoint := lastPoint{
				value:     value,
				timestamp: now,
			}

			lastPoint, ok := c.lastPoints[name]
			c.lastPoints[name] = currPoint
			if !ok {
				// Compute rate only when we have a previous point
				continue
			}

			delta := currPoint.value - lastPoint.value
			seconds := now.Sub(lastPoint.timestamp).Seconds()
			if seconds <= 0 {
				continue
			}

			if delta < 0 {
				delta = 0
			}

			rate := float64(delta) / float64(seconds)
			value = rate
		}

		metrics = append(metrics, Metric{
			Name:     name,
			Value:    value,
			Type:     c.fieldMetrics[i].metricType,
			Priority: c.fieldMetrics[i].priority,
		})
	}

	return metrics, err
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
	scopeID     uint32
	metricType  metrics.MetricType
	computeRate bool
	priority    MetricPriority
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

	// -- NVLink throughput --
	// Despite NVIDIA calling these "throughput", they report cumulative bytes transferred,
	// so we compute the rate ourselves.
	// scopeId=MaxUint32 aggregates across all links (see nvml.h L2175-L2177).
	{name: "nvlink.throughput.data.rx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX, scopeID: math.MaxUint32, metricType: metrics.GaugeType, computeRate: computeRate},
	{name: "nvlink.throughput.data.tx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX, scopeID: math.MaxUint32, metricType: metrics.GaugeType, computeRate: computeRate},
	{name: "nvlink.throughput.raw.rx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX, scopeID: math.MaxUint32, metricType: metrics.GaugeType, computeRate: computeRate},
	{name: "nvlink.throughput.raw.tx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX, scopeID: math.MaxUint32, metricType: metrics.GaugeType, computeRate: computeRate},

	// -- NVLink speed --
	// MediumLow: newer field (164), uses scopeId=0 for link 0 speed. As we do not report per-link speeds, we assume all links are at the same speed.
	// Low (default): legacy SPEED_MBPS_COMMON (90), returns common speed across all active links.
	{name: "nvlink.speed", fieldValueID: nvml.FI_DEV_NVLINK_GET_SPEED, priority: MediumLow, metricType: metrics.GaugeType},
	{name: "nvlink.speed", fieldValueID: nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON, metricType: metrics.GaugeType},

	// -- NVLink connection info --
	{name: "nvlink.nvswitch_connected", fieldValueID: nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT, metricType: metrics.GaugeType},

	// -- NVLink error counters --
	{name: "nvlink.errors.crc.data", fieldValueID: nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.crc.flit", fieldValueID: nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.ecc", fieldValueID: nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.recovery", fieldValueID: nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.replay", fieldValueID: nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
}
