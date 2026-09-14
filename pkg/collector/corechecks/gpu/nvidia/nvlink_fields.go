// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"maps"

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

func (m *nvlinkFieldValueMetric) scopeForPort(port int) uint32 {
	if m.forceScopeIDValue != nil {
		return *m.forceScopeIDValue
	}

	return uint32(port - 1)
}

func intToPointer(i uint32) *uint32 {
	return &i
}

var nvlinkFieldsMetrics = map[uint32]nvlinkFieldValueMetric{
	// -- NVLink throughput --
	// Despite NVIDIA calling these "throughput", they report cumulative bytes transferred,
	// so we compute the rate ourselves.
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX: {name: "nvlink.throughput.data.rx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX: {name: "nvlink.throughput.data.tx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX:  {name: "nvlink.throughput.raw.rx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX:  {name: "nvlink.throughput.raw.tx", fieldValueID: nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX, addTotalMetric: true, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},

	// Alternative throughput fields
	nvml.FI_DEV_NVLINK_COUNT_RCV_BYTES:  {name: "nvlink.throughput.data.rx", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_BYTES, addTotalMetric: true, metricType: metrics.GaugeType, priority: Medium, rateCalculationMode: PerSecondRateCalculation},
	nvml.FI_DEV_NVLINK_COUNT_XMIT_BYTES: {name: "nvlink.throughput.data.tx", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_BYTES, addTotalMetric: true, metricType: metrics.GaugeType, priority: Medium, rateCalculationMode: PerSecondRateCalculation},

	// -- NVLink speed --
	// MediumLow: newer field (164), uses per-link speeds. Older field return the same per-link speed for all links, lower priority (default).
	nvml.FI_DEV_NVLINK_GET_SPEED:         {name: "nvlink.speed", fieldValueID: nvml.FI_DEV_NVLINK_GET_SPEED, priority: MediumLow, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON: {name: "nvlink.speed", fieldValueID: nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON, metricType: metrics.GaugeType, forceScopeIDValue: intToPointer(0)},

	// -- NVLink error counters --
	nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL:            {name: "nvlink.errors.crc.data", fieldValueID: nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL:            {name: "nvlink.errors.crc.flit", fieldValueID: nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL:            {name: "nvlink.errors.ecc", fieldValueID: nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL:            {name: "nvlink.errors.recovery", fieldValueID: nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL:              {name: "nvlink.errors.replay", fieldValueID: nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_RCV_PACKETS:                     {name: "nvlink.rx.packets", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_PACKETS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_XMIT_PACKETS:                    {name: "nvlink.tx.packets", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_PACKETS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS:                   {name: "nvlink.tx.discards", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_MALFORMED_PACKET_ERRORS:         {name: "nvlink.errors.malformed.packet", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_MALFORMED_PACKET_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_BUFFER_OVERRUN_ERRORS:           {name: "nvlink.errors.buffer.overrun", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_BUFFER_OVERRUN_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS:                      {name: "nvlink.errors.rx", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_RCV_REMOTE_ERRORS:               {name: "nvlink.errors.rx.remote", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_REMOTE_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_RCV_GENERAL_ERRORS:              {name: "nvlink.errors.rx.general", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_RCV_GENERAL_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS:     {name: "nvlink.errors.local.link.integrity", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS: {name: "nvlink.recovery.events.successful", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS:     {name: "nvlink.recovery.events.failed", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS:                {name: "nvlink.errors.effective", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS, markUnsupportedOnInvalidArgument: true, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_BER:                   {name: "nvlink.ber.effective", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_BER, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_SYMBOL_ERRORS:                   {name: "nvlink.errors.symbol", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_SYMBOL_ERRORS, metricType: metrics.GaugeType},
	nvml.FI_DEV_NVLINK_COUNT_SYMBOL_BER:                      {name: "nvlink.ber.symbol", fieldValueID: nvml.FI_DEV_NVLINK_COUNT_SYMBOL_BER, metricType: metrics.GaugeType},
}

type nvlinkFieldsCollector struct {
	device ddnvml.Device
	// metrics maps field value ID to the metric to generate for it. It is used
	// only during collector initialization to discover and remove unsupported
	// field IDs. Collect uses requests instead.
	metrics map[uint32]nvlinkFieldValueMetric
	// requests maps a port to the requests for that port.
	requests []nvlinkFieldValueRequest
}

// nvlinkFieldValueRequest associates one NVML field-value query with its metric
// definition. Port needs to be stored separately as the scope might
type nvlinkFieldValueRequest struct {
	field  nvml.FieldValue
	metric nvlinkFieldValueMetric
	// ports is the list of port numbers for the request, for tagging
	ports []int
}

func newNVLinkFieldsCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return newNVLinkFieldsCollectorWithMetrics(device, nvlinkFieldsMetrics)
}

func newNVLinkFieldsCollectorWithMetrics(device ddnvml.Device, metrics map[uint32]nvlinkFieldValueMetric) (*nvlinkFieldsCollector, error) {
	c := &nvlinkFieldsCollector{
		device: device,
	}

	c.metrics = maps.Clone(metrics)

	_, err := getSupportedNvlinkPorts(device, c.discoverPortMetrics)
	if err != nil {
		return nil, fmt.Errorf("get supported NVLink ports: %w", err)
	}

	return c, nil
}

// Device returns the device this collector monitors.
func (c *nvlinkFieldsCollector) Device() ddnvml.Device {
	return c.device
}

func (c *nvlinkFieldsCollector) Name() CollectorName {
	return nvlinkFields
}

func (c *nvlinkFieldsCollector) Collect() ([]Sample, error) {
	if len(c.requests) == 0 {
		return nil, fmt.Errorf("%w: no metrics to collect", errUnsupportedDevice)
	}

	// Prepare the totals map with the field value IDs of the metrics that require a total calculation.
	// We need to do this with the field value IDs to avoid issues with duplicates (different fields providing the same metric)
	totals := make(map[uint32]float64)

	fields := make([]nvml.FieldValue, len(c.requests))
	for i, request := range c.requests {
		fields[i] = request.field
	}

	if err := c.device.GetFieldValues(fields); err != nil {
		return nil, err
	}

	var samples []Sample
	var errs []error
	for i, val := range fields {
		request := c.requests[i]
		fieldValueMetric := request.metric

		if val.NvmlReturn != uint32(nvml.SUCCESS) {
			errs = append(errs, fmt.Errorf("failed to get field value %s for ports %v: %s", fieldValueMetric.name, request.ports, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		value, convErr := fieldValueToNumber[float64](nvml.ValueType(val.ValueType), val.Value)
		if convErr != nil {
			errs = append(errs, fmt.Errorf("failed to convert field value %s: %w", fieldValueMetric.name, convErr))
			continue
		}

		for _, port := range request.ports {
			samples = append(samples, &Metric{
				baseSample:          baseSample{priority: fieldValueMetric.priority, tags: []string{nvlinkPortTag(port)}},
				Name:                fieldValueMetric.name,
				Value:               value,
				Type:                fieldValueMetric.metricType,
				RateCalculationMode: fieldValueMetric.rateCalculationMode,
			})
		}

		if fieldValueMetric.addTotalMetric {
			totals[fieldValueMetric.fieldValueID] += value
		}
	}

	for _, metric := range c.metrics {
		if !metric.addTotalMetric {
			continue
		}

		total, ok := totals[metric.fieldValueID]
		if !ok {
			// No value got added to this metric, so we skip it for consistency. That way,
			// we only emit the total metric if there's any value. If there was a temporary
			// failure or something, both the per-port and the total metric would be missing.
			// and interpolation can kick in, instead of showing no values for the per-port
			// metrics and a zero in the total.
			continue
		}

		samples = append(samples, &Metric{
			baseSample:          baseSample{priority: metric.priority},
			Name:                metric.name + ".total",
			Value:               total,
			Type:                metric.metricType,
			RateCalculationMode: metric.rateCalculationMode,
		})
	}

	return samples, errors.Join(errs...)
}

func (c *nvlinkFieldsCollector) discoverPortMetrics(port int) ([]Sample, error) {
	if len(c.metrics) == 0 {
		return nil, fmt.Errorf("%w: no metrics to collect", errUnsupportedDevice)
	}

	var fields []nvml.FieldValue
	for fieldValueID, metric := range c.metrics {
		fields = append(fields, nvml.FieldValue{
			FieldId: fieldValueID,
			ScopeId: metric.scopeForPort(port),
		})
	}

	if err := c.device.GetFieldValues(fields); err != nil {
		return nil, err
	}

	var errs []error
	addedRequests := 0
	for _, val := range fields {
		fieldValueMetric, ok := c.metrics[val.FieldId]
		if !ok {
			errs = append(errs, fmt.Errorf("unexpected field value ID %d", val.FieldId))
			continue
		}

		// Check first if the field returned unsupported. If it's not supported, we remove
		// this metric from the collector, even if it's after a later run. The assumption here
		// is that unsupported fields are returned from the start, and their status does not change.
		// This way, we avoid having different functions to collect metrics and to check for support.
		// We also assume that if a field is not supported for a port, it's not supported for any other port.
		if val.NvmlReturn == uint32(nvml.ERROR_NOT_SUPPORTED) || (val.NvmlReturn == uint32(nvml.ERROR_INVALID_ARGUMENT) && fieldValueMetric.markUnsupportedOnInvalidArgument) {
			log.Warnf("nvlink: fields collector removing metric %s for port %d because it's not supported, error: %s", fieldValueMetric.name, port, nvml.ErrorString(nvml.Return(val.NvmlReturn)))
			delete(c.metrics, val.FieldId)
			continue
		} else if val.NvmlReturn != uint32(nvml.SUCCESS) {
			errs = append(errs, fmt.Errorf("failed to get field value %s for port %d: %s", fieldValueMetric.name, port, nvml.ErrorString(nvml.Return(val.NvmlReturn))))
			continue
		}

		c.addRequest(fieldValueMetric, port)
		addedRequests++
	}

	if addedRequests == 0 {
		// All metrics were removed, so we return an error to indicate that the device is unsupported.
		return nil, fmt.Errorf("%w: no metrics to collect", errUnsupportedDevice)
	}

	return nil, errors.Join(errs...)
}

// addRequest adds a request for a metric to the collector. If the request already exists, it adds the port to the existing request.
func (c *nvlinkFieldsCollector) addRequest(metric nvlinkFieldValueMetric, port int) {
	fieldValue := nvml.FieldValue{FieldId: metric.fieldValueID, ScopeId: metric.scopeForPort(port)}
	for i := range c.requests {
		if c.requests[i].field.FieldId == fieldValue.FieldId && c.requests[i].field.ScopeId == fieldValue.ScopeId {
			c.requests[i].ports = append(c.requests[i].ports, port)
			return
		}
	}

	c.requests = append(c.requests, nvlinkFieldValueRequest{
		field:  fieldValue,
		metric: metric,
		ports:  []int{port},
	})
}
