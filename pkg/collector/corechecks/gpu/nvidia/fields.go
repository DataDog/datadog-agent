// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"fmt"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"
)

const fieldsCollectorName = "fields"

type fieldsCollector struct {
	device       nvml.Device
	tags         []string
	fieldMetrics []fieldValueMetric
}

func newFieldsCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	c := &fieldsCollector{
		device: device,
		tags:   tags,
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
	}

	ret := c.device.GetFieldValues(fields)
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return nil, errUnsupportedDevice
	} else if ret != nvml.SUCCESS {
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

		value, convErr := metricValueToDouble(val)
		if convErr != nil {
			err = multierror.Append(err, fmt.Errorf("failed to convert field value %s: %w", name, convErr))
		}

		metrics = append(metrics, Metric{Name: name, Value: value, Tags: c.tags})
	}

	return metrics, err
}

// Close cleans up any resources used by the collector (no-op for this collector).
func (c *fieldsCollector) Close() error {
	return nil
}

// Name returns the name of the collector.
func (c *fieldsCollector) Name() string {
	return fieldsCollectorName
}

// fieldValueMetric represents a metric that can be retrieved using the NVML
// FieldValues API, and associates a name for that metric
type fieldValueMetric struct {
	name         string
	fieldValueID uint32 // No specific type, but these are constants prefixed with FI_DEV in the nvml package
}

var metricNameToFieldID = []fieldValueMetric{
	{"memory.temperature", nvml.FI_DEV_MEMORY_TEMP},
	{"nvlink.bandwidth.c0", nvml.FI_DEV_NVLINK_BANDWIDTH_C0_TOTAL},
	{"nvlink.bandwidth.c1", nvml.FI_DEV_NVLINK_BANDWIDTH_C1_TOTAL},
	{"pci.replay_counter", nvml.FI_DEV_PCIE_REPLAY_COUNTER},
	{"slowdown_temperature", nvml.FI_DEV_PERF_POLICY_THERMAL},
}
