// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"maps"
	"math"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const sampleBufferSize = 2

// SM cycle counters, not yet defined in go-nvml. NVML reports them as cumulative raw counters summed over all
// the SMs of the device: nvmlGpmMetricsGet returns the counter value at Sample2 instead of the difference
// between Sample1 and Sample2, so we compute the difference ourselves.
const (
	gpmMetricSMCyclesElapsed nvml.GpmMetricId = 248
	gpmMetricSMCyclesActive  nvml.GpmMetricId = 249
)

type gpmCollector struct {
	lib                  ddnvml.SafeNVML
	device               ddnvml.Device
	parentPhysicalDevice *ddnvml.PhysicalDevice
	migInstanceID        int // only for collectors for MIG devices, contains the instance ID of the MIG device
	samples              [sampleBufferSize]nvml.GpmSample
	metricsToCollect     map[nvml.GpmMetricId]gpmMetric
	nextSampleToCollect  int
	emitLegacySMActive   bool
	deriveSMActive       bool // whether sm_active is derived from the SM cycle counters
	smCyclesPriority     MetricPriority
	prevSMCycles         smCyclesReading
}

// smCyclesReading holds the raw SM cycle counter values of a single GPM sample.
type smCyclesReading struct {
	elapsed float64
	active  float64
}

type gpmMetric struct {
	name       string
	metricType metrics.MetricType
}

var allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
	nvml.GPM_METRIC_GRAPHICS_UTIL: {
		name:       "gr_engine_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_SM_UTIL: {
		// Despite the name, this GPM metric returns the percentage of SMs that were in use, not whether any of them were
		// active in the interval like gr_engine_active does.
		name:       "sm_utilization",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_SM_OCCUPANCY: {
		name:       "sm_occupancy",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_INTEGER_UTIL: {
		name:       "integer_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_FP16_UTIL: {
		name:       "fp16_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_FP32_UTIL: {
		name:       "fp32_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_FP64_UTIL: {
		name:       "fp64_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_ANY_TENSOR_UTIL: {
		name:       "tensor_active",
		metricType: metrics.GaugeType,
	},
}

func newGPMCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	c, err := newGPMCollectorWithMetrics(device, maps.Clone(allGpmMetrics), deps)
	if err != nil {
		return nil, err
	}
	collector := c.(*gpmCollector)

	// sm_active is not derived from the SM cycle counters if the legacy sm_active is enabled, as it takes
	// precedence, nor for MIG devices, where the counters haven't been validated.
	_, isMig := device.(*ddnvml.MIGDevice)
	if !isMig && (deps == nil || !deps.Config.LegacySMActive) {
		// Low priority by default so that it's only a fallback. At low priority it ties with the ebpf sm_active,
		// which wins because RemoveDuplicateSamples prefers the collector whose name sorts first.
		collector.smCyclesPriority = Low
		if deps != nil && deps.Config.PreferSMCyclesSMActive {
			collector.smCyclesPriority = High
		}
		collector.initSMCycles()
	}

	return collector, nil
}

func newGPMCollectorWithMetrics(device ddnvml.Device, metricsToCollect map[nvml.GpmMetricId]gpmMetric, deps *CollectorDependencies) (c Collector, err error) {
	migDevice, isMig := device.(*ddnvml.MIGDevice)
	if isMig && migDevice.Parent == nil {
		return nil, errors.New("MIG device has no parent physical device")
	}

	// We don't query for device support because the API is broken in go-nvml 0.13.0

	// Clone the metrics map to avoid mutating the state
	clonedMetrics := maps.Clone(metricsToCollect)

	collector := &gpmCollector{
		device:           device,
		metricsToCollect: clonedMetrics,
	}
	if deps != nil {
		collector.emitLegacySMActive = deps.Config.LegacySMActive
	}

	if isMig {
		collector.parentPhysicalDevice = migDevice.Parent
		collector.migInstanceID = migDevice.MIGInstanceID
	}

	collector.lib, err = ddnvml.GetSafeNvmlLib()
	if err != nil {
		return nil, fmt.Errorf("failed to get NVML library: %w", err)
	}

	defer func() {
		if err != nil {
			// return all allocated samples to NVML if we fail after they have been allocated
			collector.freeSamples()
		}
	}()

	for i := 0; i < sampleBufferSize; i++ {
		sample, err := collector.lib.GpmSampleAlloc()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate GPM sample: %w", err)
		}
		collector.samples[i] = sample
	}

	err = collector.removeUnsupportedMetrics()
	if err != nil {
		if ddnvml.IsAPIUnsupportedOnDevice(err, device) {
			return nil, errUnsupportedDevice
		}
		return nil, fmt.Errorf("failed to remove unsupported metrics: %w", err)
	}

	if len(collector.metricsToCollect) == 0 {
		return nil, errUnsupportedDevice
	}

	return collector, nil
}

func (c *gpmCollector) removeUnsupportedMetrics() error {
	// Now collect two samples and try to get the metrics, to discard any unsupported ones.
	for i := 0; i < 2; i++ {
		err := c.collectSample()
		if err != nil {
			fmt.Printf("failed to collect GPM sample: %s\n", err)
			return err
		}
	}

	metrics, err := c.calculateGpmMetrics()
	if err != nil {
		return err
	}

	for i := uint32(0); i < metrics.NumMetrics; i++ {
		if metrics.Metrics[i].NvmlReturn != uint32(nvml.SUCCESS) {
			log.Warnf("failed to get GPM metric %d on device %s: %s\n", metrics.Metrics[i].MetricId, c.device.GetDeviceInfo().UUID, nvml.ErrorString(nvml.Return(metrics.Metrics[i].NvmlReturn)))
			delete(c.metricsToCollect, nvml.GpmMetricId(metrics.Metrics[i].MetricId))
		}
	}

	return nil
}

func (c *gpmCollector) collectSample() error {
	sample := c.samples[c.nextSampleToCollect]

	var err error
	if c.parentPhysicalDevice != nil {
		err = c.parentPhysicalDevice.GpmMigSampleGet(c.migInstanceID, sample)
	} else {
		err = c.device.GpmSampleGet(sample)
	}

	if err != nil {
		return fmt.Errorf("failed to collect GPM sample: %w", err)
	}

	c.nextSampleToCollect = (c.nextSampleToCollect + 1) % sampleBufferSize
	return nil
}

func (c *gpmCollector) freeSamples() {
	for _, sample := range c.samples {
		if sample != nil {
			_ = c.lib.GpmSampleFree(sample)
		}
	}
}

// getLastTwoSamples returns the last two samples collected (first and second return values)
// example: lastSample, secondToLastSample = getLastTwoSamples
func (c *gpmCollector) getLastTwoSamples() (nvml.GpmSample, nvml.GpmSample) {
	// Treat c.samples as a circular buffer, so we can get the last two samples by using the current index
	// and subtracting from that.
	// add sampleBufferSize to avoid negative indices.
	lastCollected := (c.nextSampleToCollect - 1 + sampleBufferSize) % sampleBufferSize
	secondLastCollected := (c.nextSampleToCollect - 2 + sampleBufferSize) % sampleBufferSize

	return c.samples[lastCollected], c.samples[secondLastCollected]
}

func (c *gpmCollector) calculateGpmMetrics() (*nvml.GpmMetricsGetType, error) {
	lastSample, secondToLastSample := c.getLastTwoSamples()
	metricsGet := &nvml.GpmMetricsGetType{
		NumMetrics: uint32(len(c.metricsToCollect)),
		Version:    nvml.GPM_METRICS_GET_VERSION,
		Sample1:    secondToLastSample,
		Sample2:    lastSample,
	}

	metricIndex := 0
	var errs []error
	for metricID := range c.metricsToCollect {
		metric, err := c.getGpmMetric(metricID, secondToLastSample, lastSample)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		metricsGet.Metrics[metricIndex] = metric
		metricIndex++
	}

	return metricsGet, errors.Join(errs...)
}

// getGpmMetric queries a single GPM metric between the two given samples. The per-metric NVML return code
// is not checked, callers must inspect NvmlReturn.
func (c *gpmCollector) getGpmMetric(metricID nvml.GpmMetricId, sample1, sample2 nvml.GpmSample) (nvml.GpmMetric, error) {
	// WORKAROUND: go-nvml's GpmMetricsGetType.Metrics array has a memory-layout
	// mismatch that corrupts elements past index 0 when NumMetrics > 1. Query each
	// metric in its own call via Metrics[0] until the upstream fix lands.
	singleMetricGet := &nvml.GpmMetricsGetType{
		NumMetrics: 1,
		Version:    nvml.GPM_METRICS_GET_VERSION,
		Sample1:    sample1,
		Sample2:    sample2,
	}
	singleMetricGet.Metrics[0] = nvml.GpmMetric{
		MetricId:   uint32(metricID),
		NvmlReturn: uint32(nvml.ERROR_UNKNOWN), // initialize to a sentinel value to ensure NVML has actually modified the value
	}

	if err := c.lib.GpmMetricsGet(singleMetricGet); err != nil {
		return nvml.GpmMetric{}, fmt.Errorf("failed to get GPM metric %d: %w", metricID, err)
	}

	return singleMetricGet.Metrics[0], nil
}

// readSMCycles returns the raw SM cycle counters at the last collected sample.
func (c *gpmCollector) readSMCycles() (smCyclesReading, error) {
	lastSample, secondToLastSample := c.getLastTwoSamples()

	var values [2]float64
	for i, metricID := range [2]nvml.GpmMetricId{gpmMetricSMCyclesElapsed, gpmMetricSMCyclesActive} {
		metric, err := c.getGpmMetric(metricID, secondToLastSample, lastSample)
		if err != nil {
			return smCyclesReading{}, err
		}
		if err := ddnvml.NewNvmlAPIErrorOrNil(fmt.Sprintf("GpmMetricsGet(%d)", metricID), nvml.Return(metric.NvmlReturn)); err != nil {
			return smCyclesReading{}, err
		}
		if math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
			return smCyclesReading{}, fmt.Errorf("invalid value %f for GPM metric %d", metric.Value, metricID)
		}
		values[i] = metric.Value
	}

	return smCyclesReading{elapsed: values[0], active: values[1]}, nil
}

// initSMCycles checks whether the SM cycle counters are supported, using the samples collected when the
// collector is created, and stores the first reading. sm_active is not derived from the counters if they are
// not supported, but the rest of the GPM metrics are still collected.
func (c *gpmCollector) initSMCycles() {
	reading, err := c.readSMCycles()
	if err != nil {
		log.Infof("not deriving sm_active from the GPM SM cycle counters on device %s: %s", c.device.GetDeviceInfo().UUID, err)
		return
	}
	c.deriveSMActive = true
	c.prevSMCycles = reading
}

// collectSMCyclesSMActive returns sm_active for the interval between the previous reading and the last collected
// sample, as the percentage of elapsed SM cycles in which the SMs were active. This is the same value as
// GPM_METRIC_SM_UTIL (activity averaged over all SMs), not the percentage of time any SM was active. It returns nil
// if the value cannot be computed for this interval.
func (c *gpmCollector) collectSMCyclesSMActive() (*Metric, error) {
	reading, err := c.readSMCycles()
	if err != nil {
		// The counters are cumulative, so the next value is computed against the previous reading.
		return nil, fmt.Errorf("failed to read SM cycle counters: %w", err)
	}

	previous := c.prevSMCycles
	c.prevSMCycles = reading
	elapsed := reading.elapsed - previous.elapsed
	active := reading.active - previous.active

	// NVML returns the counters as float64, which can't represent them exactly above 2^53: the deltas can then be
	// off by up to 2 units in the last place of the counters, so a fully active interval can report slightly more
	// active than elapsed cycles.
	maxRoundingError := 2 * (math.Nextafter(reading.elapsed, math.Inf(1)) - reading.elapsed)
	if elapsed <= 0 || active < 0 || active > elapsed+maxRoundingError {
		// No SM cycles elapsed, or the counters were reset (e.g. GPU reset or driver reload): skip this interval.
		log.Debugf("invalid SM cycle counter deltas on device %s (elapsed %f, active %f), skipping sm_active", c.device.GetDeviceInfo().UUID, elapsed, active)
		return nil, nil
	}

	return &Metric{
		baseSample: baseSample{priority: c.smCyclesPriority},
		Name:       "sm_active",
		Value:      100 * min(active, elapsed) / elapsed,
		Type:       metrics.GaugeType,
	}, nil
}

// Device returns the device this collector monitors.
func (c *gpmCollector) Device() ddnvml.Device {
	return c.device
}

func (c *gpmCollector) Name() CollectorName {
	return gpm
}

func (c *gpmCollector) Collect() ([]Sample, error) {
	err := c.collectSample()
	if err != nil {
		return nil, fmt.Errorf("failed to collect GPM sample: %w", err)
	}

	gpmMetrics, err := c.calculateGpmMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM metrics: %w", err)
	}

	metricCapacity := len(c.metricsToCollect)
	if c.emitLegacySMActive {
		metricCapacity++
	}
	if c.deriveSMActive {
		metricCapacity++
	}
	samples := make([]Sample, 0, metricCapacity)
	var errs []error
	for i := uint32(0); i < gpmMetrics.NumMetrics; i++ {
		metric := gpmMetrics.Metrics[i]
		if metric.NvmlReturn != uint32(nvml.SUCCESS) {
			errs = append(errs, fmt.Errorf("failed to get GPM metric %d: %s", metric.MetricId, nvml.ErrorString(nvml.Return(metric.NvmlReturn))))
			continue
		}

		metricData, ok := c.metricsToCollect[nvml.GpmMetricId(metric.MetricId)]
		if !ok {
			errs = append(errs, fmt.Errorf("unknown metric ID %d: %s", metric.MetricId, nvml.ErrorString(nvml.Return(metric.NvmlReturn))))
			continue
		}

		samples = append(samples, &Metric{
			baseSample: baseSample{priority: High}, // All GPM metrics have priority over other collectors
			Name:       metricData.name,
			Value:      metric.Value,
			Type:       metricData.metricType,
		})
		if c.emitLegacySMActive && nvml.GpmMetricId(metric.MetricId) == nvml.GPM_METRIC_SM_UTIL {
			samples = append(samples, &Metric{
				baseSample: baseSample{priority: High},
				Name:       "sm_active",
				Value:      metric.Value,
				Type:       metricData.metricType,
			})
		}
	}

	if c.deriveSMActive {
		smActive, err := c.collectSMCyclesSMActive()
		if err != nil {
			errs = append(errs, err)
		} else if smActive != nil {
			samples = append(samples, smActive)
		}
	}

	return samples, errors.Join(errs...)
}
