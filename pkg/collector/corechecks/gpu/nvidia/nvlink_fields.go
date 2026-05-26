// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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

// nvlinkFieldValueMetric represents a metric that can be retrieved using the NVML
// FieldValues API for NVLink-specific metrics.
type nvlinkFieldValueMetric struct {
	name         string
	fieldValueID uint32 // No specific type, but these are constants prefixed with FI_DEV in the nvml package
	// Some fields on older architectures return INVALID_ARGUMENT immediately
	// instead of cleanly reporting ERROR_NOT_SUPPORTED. Mark those fields here
	// so collector initialization can treat INVALID_ARGUMENT as unsupported.
	markUnsupportedOnInvalidArgument bool
	metricType                       metrics.MetricType
	rateCalculationMode              RateCalculationMode
	priority                         MetricPriority
	addTotalMetric                   bool
	forceScopeIDValue                *uint32
}

func intToPointer(i uint32) *uint32 {
	return &i
}

var nvlinkFieldsMetrics = []nvlinkFieldValueMetric{
	// -- NVLink throughput --
	// Despite NVIDIA calling these "throughput", they report cumulative bytes transferred,
	// so we compute the rate ourselves.
	{name: "nvlink.throughput.data.rx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	{name: "nvlink.throughput.data.tx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	{name: "nvlink.throughput.raw.rx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	{name: "nvlink.throughput.raw.tx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},

	// Alternative throughput fields
	{name: "nvlink.throughput.data.rx", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_BYTES, metricType: metrics.GaugeType, priority: Medium, rateCalculationMode: PerSecondRateCalculation},
	{name: "nvlink.throughput.data.tx", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_BYTES, metricType: metrics.GaugeType, priority: Medium, rateCalculationMode: PerSecondRateCalculation},

	// -- NVLink speed --
	// MediumLow: newer field (164), uses per-link speeds. Older field return the same per-link speed for all links, lower priority (default).
	{name: "nvlink.speed", fieldValueID: nvml.FI_DEV_NVLINK_GET_SPEED, priority: MediumLow, metricType: metrics.GaugeType},
	{name: "nvlink.speed", fieldValueID: nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON, metricType: metrics.GaugeType, forceScopeIDValue: intToPointer(0)},

	// -- NVLink connection info --
	{name: "nvlink.nvswitch_connected", fieldValueID: nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT, metricType: metrics.GaugeType},

	// -- NVLink error counters --
	{name: "nvlink.errors.crc.data", fieldValueID: nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.crc.flit", fieldValueID: nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.ecc", fieldValueID: nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.recovery", fieldValueID: nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.errors.replay", fieldValueID: nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	{name: "nvlink.rx.packets", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_PACKETS, metricType: metrics.GaugeType},
	{name: "nvlink.tx.packets", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_PACKETS, metricType: metrics.GaugeType},
	{name: "nvlink.tx.discards", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.malformed.packet", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_MALFORMED_PACKET_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.buffer.overrun", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_BUFFER_OVERRUN_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.rx", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.rx.remote", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_REMOTE_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.rx.general", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_GENERAL_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.local.link.integrity", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.recovery.events.successful", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS, metricType: metrics.GaugeType},
	{name: "nvlink.recovery.events.failed", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS, metricType: metrics.GaugeType},
	{name: "nvlink.errors.effective", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS, markUnsupportedOnInvalidArgument: true, metricType: metrics.GaugeType},
	{name: "nvlink.ber.effective", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_BER, metricType: metrics.GaugeType},
	{name: "nvlink.errors.symbol", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_SYMBOL_ERRORS, metricType: metrics.GaugeType},
	{name: "nvlink.ber.symbol", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_SYMBOL_BER, metricType: metrics.GaugeType},
}

type nvlinkFieldsCollector struct {
	device  ddnvml.Device
	metrics []nvlinkFieldValueMetric
	ports   []int
}

func newNVLinkFieldsCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	c := &nvlinkFieldsCollector{
		device: device,
	}

	c.metrics = append(c.metrics, nvlinkFieldsMetrics...) // copy all metrics to avoid modifying the original slice

	var err error
	c.ports, err = getSupportedNvlinkPorts(device, c.getPortMetrics)
	if err != nil {
		return nil, fmt.Errorf("get supported NVLink ports: %w", err)
	}

	fmt.Printf("nvlink: fields collector ports: %v\n", c.ports)

	return c, nil
}

func (c *nvlinkFieldsCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *nvlinkFieldsCollector) Name() CollectorName {
	return nvlinkFields
}

func (c *nvlinkFieldsCollector) Collect() ([]*Metric, error) {
	var metrics []*Metric
	var errs []error
	for _, port := range c.ports {
		portMetrics, err := c.getPortMetrics(port)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get port %d metrics: %w", port, err))
			fmt.Printf("nvlink: fields collector error getting port %d metrics: %s\n", port, err)
			continue
		}

		metrics = append(metrics, portMetrics...)
	}

	// Add up values for metrics that require a total calculation.
	totals := make(map[string]float64)
	for _, metric := range c.metrics {
		if metric.addTotalMetric {
			totals[metric.name] = 0
		}
	}

	for _, metric := range metrics {
		if _, ok := totals[metric.Name]; ok {
			totals[metric.Name] += metric.Value
		}
	}

	for _, metric := range c.metrics {
		if !metric.addTotalMetric {
			continue
		}

		metrics = append(metrics, &Metric{
			Name:                metric.name + ".total",
			Value:               totals[metric.name],
			Type:                metric.metricType,
			Priority:            metric.priority,
			RateCalculationMode: metric.rateCalculationMode,
		})
	}

	return metrics, errors.Join(errs...)
}

func (c *nvlinkFieldsCollector) getPortMetrics(port int) ([]*Metric, error) {
	fields := make([]nvml.FieldValue, len(c.metrics))
	for i, metric := range c.metrics {
		fields[i].FieldId = metric.fieldValueID

		if metric.forceScopeIDValue != nil {
			fields[i].ScopeId = *metric.forceScopeIDValue
		} else {
			fields[i].ScopeId = uint32(port - 1)
		}
	}

	if err := c.device.GetFieldValues(fields); err != nil {
		return nil, err
	}

	portTag := nvlinkPortTag(port)
	var metrics []*Metric
	var errs []error
	for _, val := range fields {
		metricIdx := slices.IndexFunc(c.metrics, func(m nvlinkFieldValueMetric) bool {
			return m.fieldValueID == val.FieldId
		})
		if metricIdx == -1 {
			errs = append(errs, fmt.Errorf("unexpected field value ID %d", val.FieldId))
			continue
		}

		fieldValueMetric := c.metrics[metricIdx]

		// Check first if the field returned unsupported. If it's not supported, we remove
		// this metric from the collector, even if it's after a later run. The assumption here
		// is that unsupported fields are returned from the start, and their status does not change.
		// This way, we avoid having different functions to collect metrics and to check for support.
		if val.NvmlReturn == uint32(nvml.ERROR_NOT_SUPPORTED) || (val.NvmlReturn == uint32(nvml.ERROR_INVALID_ARGUMENT) && fieldValueMetric.markUnsupportedOnInvalidArgument) {
			c.metrics = slices.Delete(c.metrics, metricIdx, metricIdx+1)
			log.Warnf("nvlink: fields collector removing metric %s for port %d because it's not supported, error: %s", fieldValueMetric.name, port, nvml.ErrorString(nvml.Return(val.NvmlReturn)))
			continue
		} else if val.NvmlReturn != uint32(nvml.SUCCESS) {
			errs = append(errs, fmt.Errorf("failed to get field value %s for port %d: %s", fieldValueMetric.name, port, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		value, convErr := fieldValueToNumber[float64](nvml.ValueType(val.ValueType), val.Value)
		if convErr != nil {
			errs = append(errs, fmt.Errorf("failed to convert field value %s: %w", fieldValueMetric.name, convErr))
			continue
		}

		metrics = append(metrics, &Metric{
			Name:                fieldValueMetric.name,
			Value:               value,
			Type:                fieldValueMetric.metricType,
			Priority:            fieldValueMetric.priority,
			RateCalculationMode: fieldValueMetric.rateCalculationMode,
			Tags:                []string{portTag},
		})
	}

	return metrics, errors.Join(errs...)
}
