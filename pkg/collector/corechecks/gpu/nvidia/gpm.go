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
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const sampleBufferSize = 2

type gpmCollector struct {
	lib                  ddnvml.SafeNVML
	device               ddnvml.Device
	parentPhysicalDevice *ddnvml.PhysicalDevice
	migInstanceID        int // only for collectors for MIG devices, contains the instance ID of the MIG device
	samples              [sampleBufferSize]nvml.GpmSample
	metricsToCollect     map[nvml.GpmMetricId]gpmMetric
	nextSampleToCollect  int
	emitLegacySMActive   bool
	emitGrEngineSMActive bool
	// migMode is true for MIG devices and GPUs in MIG mode, where sm_active comes from SM_UTIL, or from
	// GRAPHICS_UTIL when SM_UTIL is not available
	migMode                  bool
	grEngineSMActivePriority MetricPriority
	// suspendedGraphicsUtil holds GRAPHICS_UTIL while it is not collected because NVML reported an impossible value
	// for it, until graphicsUtilResumeAt (see isImpossibleMIGGraphicsUtil)
	suspendedGraphicsUtil *gpmMetric
	graphicsUtilResumeAt  time.Time
	now                   func() time.Time
}

// graphicsUtilSuspension is how long GRAPHICS_UTIL is not collected on a MIG device after NVML reported an
// impossible value for it, before trying again: the cause, a MIG device of the GPU the Agent cannot open, can go
// away when that MIG device is destroyed.
const graphicsUtilSuspension = 10 * time.Minute

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

func newGPMCollector(device ddnvml.Device, deps *CollectorDependencies) (c Collector, err error) {
	return newGPMCollectorWithMetrics(device, maps.Clone(allGpmMetrics), deps)
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
		now:              time.Now,
	}
	if deps != nil {
		collector.emitLegacySMActive = deps.Config.LegacySMActive
		// GRAPHICS_UTIL is also reported as sm_active because it approximates the percentage of time during
		// which at least one SM is executing work, unlike SM_UTIL which averages utilization across all SMs.
		collector.emitGrEngineSMActive = !deps.Config.LegacySMActive
		// Keep this source below sampling (Medium) and eBPF (MediumLow) by default.
		collector.grEngineSMActivePriority = Low
		if deps.Config.PreferGrEngineSMActive {
			collector.grEngineSMActivePriority = High
		}
		// On MIG devices and GPUs in MIG mode, report sm_active from SM_UTIL as gpu.legacy_sm_active does:
		// GRAPHICS_UTIL reads 0 there without the MIG config capability (which the Agent should not need) and
		// about half on some profiles (1g.35gb on an H200), while SM_UTIL is correct without privileges and
		// matches the DCGM SM_ACTIVE of these devices.
		if _, _, inMIGMode := migGpuInstanceAccessTarget(device); inMIGMode {
			collector.migMode = true
			collector.emitLegacySMActive = true
			collector.emitGrEngineSMActive = false
			// The GRAPHICS_UTIL fallback stays below the other sources, whatever PreferGrEngineSMActive says
			collector.grEngineSMActivePriority = Low
		}
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
			// the driver does not support GPM, retrying cannot help
			if ddnvml.IsAPIUnsupportedOnDevice(err, device) {
				return nil, errUnsupportedDevice
			}
			return nil, fmt.Errorf("failed to allocate GPM sample: %w", err)
		}
		collector.samples[i] = sample
	}

	err = collector.removeUnsupportedMetrics()
	if err != nil {
		// The error is a join of per-metric errors, which can mix unsupported metrics with permission
		// failures; errors.As stops at the first NVML error, so a permission error would be hidden and
		// the collector marked as unsupported. Permissions can be recovered, so return them instead.
		if !ddnvml.IsNoPermission(err) && ddnvml.IsAPIUnsupportedOnDevice(err, device) {
			return nil, errUnsupportedDevice
		}
		return nil, fmt.Errorf("failed to remove unsupported metrics: %w", err)
	}

	collector.removeFalseMIGGraphicsUtil()

	if len(collector.metricsToCollect) == 0 {
		return nil, errUnsupportedDevice
	}

	return collector, nil
}

// migGpuInstanceAccessCheck returns nil if NVML can read the given GPU instance of a GPU in MIG mode, or an
// NVML_ERROR_NOT_FOUND error if it does not exist. NVML checks the permission first: without the MIG config
// capability it returns NVML_ERROR_NO_PERMISSION for any GPU instance ID, existing or not (measured on an H200
// with driver 580), so the check does not need an existing MIG device. It is a variable so that tests can replace it.
var migGpuInstanceAccessCheck = func(parent *ddnvml.PhysicalDevice, gpuInstanceID int) error {
	return parent.CheckGpuInstanceAccess(gpuInstanceID)
}

// loggedFalseMIGGraphicsUtil holds the UUIDs of the GPUs in MIG mode for which removeFalseMIGGraphicsUtil logged
// that it removed GRAPHICS_UTIL, to log it once per GPU rather than once per MIG device.
var loggedFalseMIGGraphicsUtil sync.Map

// removeFalseMIGGraphicsUtil stops collecting GRAPHICS_UTIL, and so the sm_active reported from it, on MIG
// devices and on GPUs in MIG mode when NVML cannot read the GPU instances. Without the MIG config capability
// (CAP_SYS_ADMIN or /dev/nvidia-caps/nvidia-cap1) NVML returns GRAPHICS_UTIL successfully but as 0, even on a
// loaded MIG device, while the other GPM metrics stay correct. The GPU instance lookup needs that same
// capability, so it fails with NO_PERMISSION exactly when the value is wrong. Reporting nothing is better
// than a 0 that makes a busy GPU look idle; sm_active can still come from the eBPF sources, if enabled.
func (c *gpmCollector) removeFalseMIGGraphicsUtil() {
	if _, ok := c.metricsToCollect[nvml.GPM_METRIC_GRAPHICS_UTIL]; !ok {
		return
	}
	parent, gpuInstanceID, ok := migGpuInstanceAccessTarget(c.device)
	if !ok {
		return
	}
	// Any other error (e.g. the API is missing from the driver) is inconclusive, so keep the metric.
	err := migGpuInstanceAccessCheck(parent, gpuInstanceID)
	if !ddnvml.IsNoPermission(err) {
		return
	}

	delete(c.metricsToCollect, nvml.GPM_METRIC_GRAPHICS_UTIL)
	if _, logged := loggedFalseMIGGraphicsUtil.LoadOrStore(parent.GetDeviceInfo().UUID, struct{}{}); !logged {
		log.Infof("not reporting gr_engine_active from GPM for the MIG devices of GPU %s and for the GPU itself: NVML reports them as 0 on MIG without the MIG config capability (CAP_SYS_ADMIN or /dev/nvidia-caps/nvidia-cap1): %v", parent.GetDeviceInfo().UUID, err)
	}
}

// migGpuInstanceAccessTarget returns the GPU in MIG mode and a GPU instance ID to check the access to the GPU
// instances for a MIG device or a GPU in MIG mode. ok is false for other devices.
func migGpuInstanceAccessTarget(device ddnvml.Device) (parent *ddnvml.PhysicalDevice, gpuInstanceID int, ok bool) {
	switch d := device.(type) {
	case *ddnvml.MIGDevice:
		return d.Parent, d.MIGInstanceID, d.Parent != nil
	case *ddnvml.PhysicalDevice:
		if len(d.MIGChildren) > 0 && d.MIGChildren[0] != nil {
			return d, d.MIGChildren[0].MIGInstanceID, true
		}
		if d.HasMIGFeatureEnabled {
			// NVML checks the permission before looking the GPU instance up, so any ID works without MIG devices
			return d, 0, true
		}
	}
	return nil, 0, false
}

func (c *gpmCollector) removeUnsupportedMetrics() error {
	// Now collect two samples and try to get the metrics, to discard any unsupported ones.
	for i := 0; i < 2; i++ {
		err := c.collectSample()
		if err != nil {
			return err
		}
	}

	metrics, err := c.calculateGpmMetrics()
	if err != nil {
		return err
	}

	permissionDenied := false
	for i := uint32(0); i < metrics.NumMetrics; i++ {
		if metrics.Metrics[i].NvmlReturn != uint32(nvml.SUCCESS) {
			log.Warnf("failed to get GPM metric %d on device %s: %s\n", metrics.Metrics[i].MetricId, c.device.GetDeviceInfo().UUID, nvml.ErrorString(nvml.Return(metrics.Metrics[i].NvmlReturn)))
			delete(c.metricsToCollect, nvml.GpmMetricId(metrics.Metrics[i].MetricId))
			permissionDenied = permissionDenied || metrics.Metrics[i].NvmlReturn == uint32(nvml.ERROR_NO_PERMISSION)
		}
	}

	// permissions can be recovered, so the collector is not reported as unsupported
	if len(c.metricsToCollect) == 0 && permissionDenied {
		return &ddnvml.NvmlAPIError{APIName: "GpmMetricsGet", NvmlErrorCode: nvml.ERROR_NO_PERMISSION}
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
		// WORKAROUND: go-nvml's GpmMetricsGetType.Metrics array has a memory-layout
		// mismatch that corrupts elements past index 0 when NumMetrics > 1. Query each
		// metric in its own call via Metrics[0] until the upstream fix lands.
		singleMetricGet := &nvml.GpmMetricsGetType{
			NumMetrics: 1,
			Version:    nvml.GPM_METRICS_GET_VERSION,
			Sample1:    secondToLastSample,
			Sample2:    lastSample,
		}
		singleMetricGet.Metrics[0] = nvml.GpmMetric{
			MetricId:   uint32(metricID),
			NvmlReturn: uint32(nvml.ERROR_UNKNOWN), // initialize to a sentinel value to ensure NVML has actually modified the value
		}

		err := c.lib.GpmMetricsGet(singleMetricGet)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get GPM metric %d: %w", metricID, err))
			continue
		}

		metricsGet.Metrics[metricIndex] = singleMetricGet.Metrics[0]
		metricIndex++
	}

	return metricsGet, errors.Join(errs...)
}

// isImpossibleMIGGraphicsUtil returns true when, on a MIG device or a GPU in MIG mode, GRAPHICS_UTIL is 0 while
// SM_UTIL is not: SMs cannot be in use while the graphics engine is idle. NVML reports such a 0 for some MIG
// devices when the Agent cannot open all the MIG devices of the GPU (e.g. one created after the Agent container
// started), even with the MIG config capability, so the GPU instance access check cannot detect it. GRAPHICS_UTIL
// below SM_UTIL but above 0 is legitimate on some profiles (1g.35gb on an H200 reads about half), so only 0 is.
func (c *gpmCollector) isImpossibleMIGGraphicsUtil(gpmMetrics *nvml.GpmMetricsGetType) bool {
	if slicingModeTag(c.device) == "none" {
		return false
	}
	var graphicsUtil, smUtil float64
	var hasGraphicsUtil, hasSMUtil bool
	for i := uint32(0); i < gpmMetrics.NumMetrics; i++ {
		metric := gpmMetrics.Metrics[i]
		if metric.NvmlReturn != uint32(nvml.SUCCESS) {
			continue
		}
		switch nvml.GpmMetricId(metric.MetricId) {
		case nvml.GPM_METRIC_GRAPHICS_UTIL:
			graphicsUtil, hasGraphicsUtil = metric.Value, true
		case nvml.GPM_METRIC_SM_UTIL:
			smUtil, hasSMUtil = metric.Value, true
		}
	}
	// Below 1%, SM_UTIL can be rounding noise of an idle MIG device
	return hasGraphicsUtil && hasSMUtil && graphicsUtil == 0 && smUtil >= minImpossibleSMUtil
}

// hasSuccessfulGpmMetric returns true if the GPM sample has a successful value for the metric, which is collected.
func hasSuccessfulGpmMetric(gpmMetrics *nvml.GpmMetricsGetType, id nvml.GpmMetricId, collected map[nvml.GpmMetricId]gpmMetric) bool {
	if _, ok := collected[id]; !ok {
		return false
	}
	for i := uint32(0); i < gpmMetrics.NumMetrics; i++ {
		if nvml.GpmMetricId(gpmMetrics.Metrics[i].MetricId) == id {
			return gpmMetrics.Metrics[i].NvmlReturn == uint32(nvml.SUCCESS)
		}
	}
	return false
}

// minImpossibleSMUtil is the SM_UTIL from which a GRAPHICS_UTIL of 0 is considered impossible
const minImpossibleSMUtil = 1

// clock returns the current time
func (c *gpmCollector) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

// Device returns the device this collector monitors.
func (c *gpmCollector) Device() ddnvml.Device {
	return c.device
}

func (c *gpmCollector) Name() CollectorName {
	return gpm
}

func (c *gpmCollector) Collect() ([]Sample, error) {
	if c.suspendedGraphicsUtil != nil && !c.clock().Before(c.graphicsUtilResumeAt) {
		c.metricsToCollect[nvml.GPM_METRIC_GRAPHICS_UTIL] = *c.suspendedGraphicsUtil
		c.suspendedGraphicsUtil = nil
	}

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
	if c.emitGrEngineSMActive || c.migMode {
		metricCapacity++
	}
	samples := make([]Sample, 0, metricCapacity)
	skipGraphicsUtil := c.isImpossibleMIGGraphicsUtil(gpmMetrics)
	if skipGraphicsUtil {
		// The cause (a MIG device of the GPU the Agent cannot open) usually lasts until that MIG device is destroyed
		// or the Agent restarts, and dropping only the intervals with load would leave a series that reads 0 when
		// idle and nothing when busy, which looks idle once aggregated: stop collecting the metric for a while.
		suspended := c.metricsToCollect[nvml.GPM_METRIC_GRAPHICS_UTIL]
		c.suspendedGraphicsUtil = &suspended
		c.graphicsUtilResumeAt = c.clock().Add(graphicsUtilSuspension)
		delete(c.metricsToCollect, nvml.GPM_METRIC_GRAPHICS_UTIL)
		if parent, _, ok := migGpuInstanceAccessTarget(c.device); ok {
			if _, logged := loggedFalseMIGGraphicsUtil.LoadOrStore(parent.GetDeviceInfo().UUID+"/impossible", struct{}{}); !logged {
				log.Infof("not reporting gr_engine_active from GPM for device %s and the other MIG devices of GPU %s where NVML reports the graphics engine idle while SMs are in use, which happens when the Agent cannot open all the MIG devices of the GPU", c.device.GetDeviceInfo().UUID, parent.GetDeviceInfo().UUID)
			}
		}
	}
	// On MIG, sm_active comes from SM_UTIL; fall back to GRAPHICS_UTIL (when collected and not dropped above) when
	// SM_UTIL is not available for this sample
	emitGrEngineSMActive := c.emitGrEngineSMActive || (c.migMode && !hasSuccessfulGpmMetric(gpmMetrics, nvml.GPM_METRIC_SM_UTIL, c.metricsToCollect))
	var errs []error
	for i := uint32(0); i < gpmMetrics.NumMetrics; i++ {
		metric := gpmMetrics.Metrics[i]
		if metric.NvmlReturn != uint32(nvml.SUCCESS) {
			errs = append(errs, fmt.Errorf("failed to get GPM metric %d: %s", metric.MetricId, nvml.ErrorString(nvml.Return(metric.NvmlReturn))))
			continue
		}
		if skipGraphicsUtil && nvml.GpmMetricId(metric.MetricId) == nvml.GPM_METRIC_GRAPHICS_UTIL {
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
		if emitGrEngineSMActive && nvml.GpmMetricId(metric.MetricId) == nvml.GPM_METRIC_GRAPHICS_UTIL {
			samples = append(samples, &Metric{
				baseSample: baseSample{priority: c.grEngineSMActivePriority},
				Name:       "sm_active",
				Value:      metric.Value,
				Type:       metricData.metricType,
			})
		}
	}

	return samples, errors.Join(errs...)
}
