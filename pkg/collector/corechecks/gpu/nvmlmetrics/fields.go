// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvmlmetrics

import (
	"bytes"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"
)

const fieldsMetricsCollectorName = "fields"

type fieldsMetricsCollector struct {
}

func newFieldsMetricsCollector(_ nvml.Interface, _ []nvml.Device) (subsystemCollector, error) {
	return &fieldsMetricsCollector{}, nil
}

// fieldValueMetric represents a metric that can be collected from an NVML device, using the NVML
type fieldValueMetric struct {
	name         string
	fieldValueId uint32 // No specific type, but these are constants prefixed withFI_DEV
}

func (coll *fieldsMetricsCollector) collectMetrics(dev nvml.Device) ([]Metric, error) {
	var err error

	vals := make([]nvml.FieldValue, 0, len(allfieldValueMetrics))

	for i, metric := range allfieldValueMetrics {
		vals[i].FieldId = metric.fieldValueId
	}

	ret := dev.GetFieldValues(vals)
	metrics := make([]Metric, 0, len(allfieldValueMetrics))
	for i, val := range vals {
		name := allfieldValueMetrics[i].name
		if val.NvmlReturn != uint32(nvml.SUCCESS) {
			err = multierror.Append(err, fmt.Errorf("failed to get field value %s: %s", name, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		value, err := metricValueToDouble(val)
		if err != nil {
			err = multierror.Append(err, fmt.Errorf("failed to convert field value %s: %s", name, err))
		}

		metrics = append(metrics, Metric{Name: name, Value: value})
	}

	return metrics, ret
}

func (coll *fieldsMetricsCollector) close() error {
	return nil
}

func (coll *fieldsMetricsCollector) name() string {
	return fieldsMetricsCollectorName
}

func metricValueToDouble(val nvml.FieldValue) (float64, error) {
	reader := bytes.NewReader(val.Value[:])

	switch nvml.ValueType(val.ValueType) {
	case nvml.VALUE_TYPE_DOUBLE:
		return readDoubleFromBuffer[float64](reader)
	case nvml.VALUE_TYPE_UNSIGNED_INT:
		return readDoubleFromBuffer[uint32](reader)
	case nvml.VALUE_TYPE_UNSIGNED_LONG:
	case nvml.VALUE_TYPE_UNSIGNED_LONG_LONG:
		return readDoubleFromBuffer[uint64](reader)
	case nvml.VALUE_TYPE_SIGNED_LONG_LONG: // No typo, there's no SIGNED_LONG in the NVML API
		return readDoubleFromBuffer[int64](reader)
	case nvml.VALUE_TYPE_SIGNED_INT:
		return readDoubleFromBuffer[int32](reader)
	}

	return 0, fmt.Errorf("unsupported value type %d", val.ValueType)
}

var allfieldValueMetrics = []fieldValueMetric{
	{"memory.temperature", nvml.FI_DEV_MEMORY_TEMP},
	{"nvlink.bandwidth.c0", nvml.FI_DEV_NVLINK_BANDWIDTH_C0_TOTAL},
	{"nvlink.bandwidth.c1", nvml.FI_DEV_NVLINK_BANDWIDTH_C1_TOTAL},
	{"pci.replay_counter", nvml.FI_DEV_PCIE_REPLAY_COUNTER},
	{"slowdown_temperature", nvml.FI_DEV_PERF_POLICY_THERMAL},
}
