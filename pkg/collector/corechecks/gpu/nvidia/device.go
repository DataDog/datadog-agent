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

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

const deviceCollectorName = "device"

var allDeviceMetrics = []deviceMetric{
	{"pci.throughput.tx", getTxPciThroughput},
	{"pci.throughput.rx", getRxPciThroughput},
	{"decoder_utiliation", getDecoderUtilization},
	{"dram_active", getDramActive},
	{"encoder_utilization", getEncoderUtilization},
	{"fan_speed", getFanSpeed},
	{"power.management_limit", getPowerManagementLimit},
	{"power.usage", getPowerUsage},
	{"performance_state", getPerformanceState},
	{"clock_speed.sm", getSMClockSpeed},
	{"clock_speed.memory", getMemoryClockSpeed},
	{"clock_speed.graphics", getGraphicsClockSpeed},
	{"clock_speed.video", getVideoClockSpeed},
	{"temperature", getTemperature},
	{"total_energy_consumption", getTotalEnergyConsumption},
}

type deviceCollector struct {
	device        nvml.Device
	tags          []string
	metricGetters []deviceMetric
}

func newDeviceCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	c := &deviceCollector{
		device: device,
		tags:   tags,
	}
	c.metricGetters = append(c.metricGetters, allDeviceMetrics...) // copy all metrics to avoid modifying the original slice

	c.removeUnsupportedGetters()
	if len(c.metricGetters) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *deviceCollector) removeUnsupportedGetters() {
	metricsToRemove := common.StringSet{}
	for _, metric := range c.metricGetters {
		_, ret := metric.getter(c.device)
		if ret == nvml.ERROR_NOT_SUPPORTED {
			metricsToRemove.Add(metric.name)
		}
	}

	for toRemove := range metricsToRemove {
		c.metricGetters = slices.DeleteFunc(c.metricGetters, func(m deviceMetric) bool {
			return m.name == toRemove
		})
	}
}

// deviceMetricGetter is a function type that receives a NVML device and returns one or more values
type deviceMetricGetter func(nvml.Device) (float64, nvml.Return)

// deviceMetric represents a metric that can be collected from an NVML device, using the NVML
// API on that specific device.
type deviceMetric struct {
	name   string
	getter deviceMetricGetter
}

// Collect collects all the metrics from the given NVML device.
func (c *deviceCollector) Collect() ([]Metric, error) {
	var err error

	values := make([]Metric, 0, len(c.metricGetters)) // preallocate to reduce allocations
	for _, metric := range c.metricGetters {
		value, ret := metric.getter(c.device)
		if ret != nvml.SUCCESS {
			err = multierror.Append(err, fmt.Errorf("failed to get metric %s: %s", metric.name, nvml.ErrorString(ret)))
			continue
		}

		values = append(values, Metric{
			Name:  metric.name,
			Value: value,
			Tags:  c.tags,
		})

	}

	return values, err
}

// Close closes the collector (no-op for this collector).
func (c *deviceCollector) Close() error {
	return nil
}

// Name returns the name of the collector.
func (c *deviceCollector) Name() string {
	return deviceCollectorName
}

func getRxPciThroughput(dev nvml.Device) (float64, nvml.Return) {
	tput, ret := dev.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES)
	return float64(tput), ret
}

func getTxPciThroughput(dev nvml.Device) (float64, nvml.Return) {
	tput, ret := dev.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES)
	return float64(tput), ret
}

func getDecoderUtilization(dev nvml.Device) (float64, nvml.Return) {
	util, _, ret := dev.GetDecoderUtilization()
	return float64(util), ret
}

func getDramActive(dev nvml.Device) (float64, nvml.Return) {
	util, ret := dev.GetUtilizationRates()
	return float64(util.Memory), ret
}

func getEncoderUtilization(dev nvml.Device) (float64, nvml.Return) {
	util, _, ret := dev.GetEncoderUtilization()
	return float64(util), ret
}

func getFanSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetFanSpeed()
	return float64(speed), ret
}

func getPowerManagementLimit(dev nvml.Device) (float64, nvml.Return) {
	limit, ret := dev.GetPowerManagementLimit()
	return float64(limit), ret
}

func getPowerUsage(dev nvml.Device) (float64, nvml.Return) {
	power, ret := dev.GetPowerUsage()
	return float64(power), ret
}

func getPerformanceState(dev nvml.Device) (float64, nvml.Return) {
	state, ret := dev.GetPerformanceState()
	return float64(state), ret
}

func getSMClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_SM)
	return float64(speed), ret
}

func getMemoryClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_MEM)
	return float64(speed), ret
}

func getGraphicsClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_GRAPHICS)
	return float64(speed), ret
}

func getVideoClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_VIDEO)
	return float64(speed), ret
}

func getTemperature(dev nvml.Device) (float64, nvml.Return) {
	temp, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU)
	return float64(temp), ret
}

func getTotalEnergyConsumption(dev nvml.Device) (float64, nvml.Return) {
	energy, ret := dev.GetTotalEnergyConsumption()
	return float64(energy), ret
}
