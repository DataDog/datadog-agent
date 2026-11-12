// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package amd

import (
	"errors"
	"math"
	"strconv"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// gpuMetricsRetryInterval is how often a gpu_metrics table that was not
	// decoded is read again.
	gpuMetricsRetryInterval = time.Minute
	// maxXGMILinkKBPerSecond bounds a plausible XGMI rate on one link and in
	// one direction when the table has no link speed and width: 1 TB/s, over
	// 10 times the bandwidth of an MI355X link.
	maxXGMILinkKBPerSecond = 1e9
	// xgmiLinkMargin is the factor over the link bandwidth, from its speed and
	// width, that a plausible rate stays under (KB or KiB, firmware timing).
	xgmiLinkMargin = 2
	// accumulatorOvershoot is the excess over 100% tolerated, and clamped, in
	// a throttling fraction or an activity average: the accumulators and the
	// accumulation counter are not updated atomically.
	accumulatorOvershoot = 0.05
)

// gpuMetricsFailure is the reason a gpu_metrics table, or part of it, was not used.
type gpuMetricsFailure string

const (
	failureUnsupportedVersion gpuMetricsFailure = "unsupported_version"
	failureMalformed          gpuMetricsFailure = "malformed"
	failureUnexpectedEncoding gpuMetricsFailure = "unexpected_encoding"
	failureImplausibleValue   gpuMetricsFailure = "implausible_value"
	failureCounterReset       gpuMetricsFailure = "counter_reset"
)

// throttleReason maps a gpu_metrics residency counter to the NVIDIA throttle
// reason metric of the same cause.
type throttleReason struct {
	metric    string
	residency func(*amdgpu.GPUMetrics) amdgpu.Counter
}

var throttleReasons = []throttleReason{
	{"clock.throttle_reasons.sw_power_cap", func(g *amdgpu.GPUMetrics) amdgpu.Counter { return g.PPTResidency }},
	{"clock.throttle_reasons.sw_thermal_slowdown", func(g *amdgpu.GPUMetrics) amdgpu.Counter { return g.SocketThermalResidency }},
	{"clock.throttle_reasons.hw_slowdown", func(g *amdgpu.GPUMetrics) amdgpu.Counter { return g.ProchotResidency }},
}

type gpuMetricsTelemetry struct {
	reads          telemetry.Counter // reads counts the gpu_metrics tables read, by device and version
	failures       telemetry.Counter // failures counts the gpu_metrics tables not used, or partly used, by device and reason
	activitySource telemetry.Counter // activitySource counts the activity values emitted, by device, metric and source
}

func newGPUMetricsTelemetry(tm telemetry.Component) gpuMetricsTelemetry {
	return gpuMetricsTelemetry{
		reads:    tm.NewCounter(CheckName, "gpu_metrics_reads", []string{"device", "version"}, "Number of gpu_metrics tables read from AMD GPUs, by version"),
		failures: tm.NewCounter(CheckName, "gpu_metrics_failures", []string{"device", "reason"}, "Number of gpu_metrics tables of AMD GPUs not used, or partly used, by reason"),
		activitySource: tm.NewCounter(CheckName, "activity_source", []string{"device", "metric", "source"},
			"Number of AMD GPU activity values emitted, by source: accumulator for interval averages from gpu_metrics, snapshot for sysfs busy percentages"),
	}
}

// gpuMetricsDevice is the gpu_metrics state of one device.
type gpuMetricsDevice struct {
	version    amdgpu.GPUMetricsVersion // version is the version of the last table read
	rejectedAt time.Time                // rejectedAt is the time the last table was not decoded; zero when it was
	previous   *amdgpu.GPUMetrics       // previous is the last decoded table, the baseline of accumulated counters
	logged     string                   // logged is the decoding error last logged, to log changes only
}

// gpuMetricsCollector turns the gpu_metrics tables of AMD GPUs into XGMI,
// throttling, PCIe replay and activity values. A table is used only when its
// version is decoded, its structure is consistent and its used fields have
// expected encodings; its values are used only when they satisfy invariants
// between the fields of the table and between consecutive tables.
type gpuMetricsCollector struct {
	devices   map[string]*gpuMetricsDevice // devices holds the state of each device, by UUID
	telemetry gpuMetricsTelemetry
}

func newGPUMetricsCollector(tm telemetry.Component) *gpuMetricsCollector {
	return &gpuMetricsCollector{
		devices:   make(map[string]*gpuMetricsDevice),
		telemetry: newGPUMetricsTelemetry(tm),
	}
}

// readDue reports whether the gpu_metrics table of a device should be read at
// now. A table that was not decoded is only read again after the retry interval.
func (c *gpuMetricsCollector) readDue(deviceUUID string, now time.Time) bool {
	state := c.devices[deviceUUID]
	return state == nil || state.rejectedAt.IsZero() || now.Sub(state.rejectedAt) >= gpuMetricsRetryInterval
}

// samples returns the gpu_metrics samples of a device from a read of its
// telemetry made at now, and the interval averages of its activity, which are
// invalid when the table cannot provide them. read reports whether the read
// included the gpu_metrics table.
func (c *gpuMetricsCollector) samples(dev *amdgpu.Device, m *amdgpu.Metrics, read bool, now time.Time) ([]nvidia.Sample, activity) {
	if !read {
		return nil, activity{}
	}
	gm := m.GPUMetrics
	state := c.devices[dev.UUID]
	if gm == nil && m.GPUMetricsErr == nil {
		if state != nil {
			state.previous = nil // the device had a table: the read failed or was blocked
		}
		return nil, activity{} // no gpu_metrics attribute
	}
	if state == nil {
		state = &gpuMetricsDevice{}
		c.devices[dev.UUID] = state
	}
	if gm != nil {
		c.telemetry.reads.Inc(dev.UUID, gm.Version.String())
	}

	if m.GPUMetricsErr != nil {
		reason := failureMalformed
		switch {
		case errors.Is(m.GPUMetricsErr, amdgpu.ErrGPUMetricsUnsupported):
			reason = failureUnsupportedVersion
		case errors.Is(m.GPUMetricsErr, amdgpu.ErrGPUMetricsUnexpectedEncoding):
			reason = failureUnexpectedEncoding
		}
		// Not read again before the retry interval.
		state.previous, state.rejectedAt = nil, now
		if gm != nil {
			state.version = gm.Version
		}
		c.telemetry.failures.Inc(dev.UUID, string(reason))
		if msg := m.GPUMetricsErr.Error(); state.logged != msg {
			state.logged = msg
			if reason == failureUnexpectedEncoding {
				log.Infof("not using the gpu_metrics of AMD GPU %s: %v", dev.UUID, m.GPUMetricsErr)
			} else {
				log.Debugf("not using the gpu_metrics of AMD GPU %s: %v", dev.UUID, m.GPUMetricsErr)
			}
		}
		return nil, activity{}
	}

	if state.logged != "" || gm.Version != state.version {
		log.Debugf("using gpu_metrics %s of AMD GPU %s", gm.Version, dev.UUID)
	}
	if gm.Version != state.version {
		state.previous = nil // counters of another layout are no baseline
	}
	state.version, state.rejectedAt, state.logged = gm.Version, time.Time{}, ""
	if regressed(state.previous, gm) {
		// The driver or the firmware restarted: start a new baseline.
		state.previous = nil
		c.telemetry.failures.Inc(dev.UUID, string(failureCounterReset))
	}

	previous := state.previous
	state.previous = gm
	samples, act, implausible := intervalSamples(previous, gm, m.XGMIHiveSize >= 2)
	if implausible || len(gm.Implausible) > 0 {
		c.telemetry.failures.Inc(dev.UUID, string(failureImplausibleValue))
	}
	return samples, act
}

// intervalSamples returns the samples and the activity averages of a decoded
// table, with previous as the baseline of its accumulated counters, or nil for
// the first table. implausible reports values dropped by an invariant.
func intervalSamples(previous, gm *amdgpu.GPUMetrics, xgmi bool) (samples []nvidia.Sample, act activity, implausible bool) {
	if gm.PCIeReplayCount.Valid {
		// A cumulative firmware counter, reported as a gauge like the NVML one.
		samples = append(samples, nvidia.NewMetric("pci.replay_counter", float64(gm.PCIeReplayCount.Value), metrics.GaugeType, nvidia.Low, nil, nil))
	}
	if previous == nil {
		return samples, activity{}, false // the first table is the baseline of accumulated counters
	}
	throttle, implausibleThrottle := throttleSamples(previous, gm)
	samples = append(samples, throttle...)
	implausibleXGMI := false
	if xgmi {
		var linkSamples []nvidia.Sample
		linkSamples, implausibleXGMI = xgmiSamples(previous, gm)
		samples = append(samples, linkSamples...)
	}
	act, implausibleActivity := activityAverages(previous, gm)
	return samples, act, implausibleThrottle || implausibleXGMI || implausibleActivity
}

// regressed reports whether the accumulation counter or the system clock went
// back between two tables, which a driver or firmware restart does.
func regressed(previous, current *amdgpu.GPUMetrics) bool {
	if previous == nil {
		return false
	}
	back := func(prev, cur amdgpu.Counter) bool { return prev.Valid && cur.Valid && cur.Value < prev.Value }
	return back(previous.AccumulationCounter, current.AccumulationCounter) || back(previous.SystemClockNs, current.SystemClockNs)
}

// increase returns the increase of a counter between two tables. ok is false
// when either value is invalid or the counter decreased (a reset or a wrap).
func increase(prev, cur amdgpu.Counter) (delta uint64, ok bool) {
	if !prev.Valid || !cur.Valid || cur.Value < prev.Value {
		return 0, false
	}
	return cur.Value - prev.Value, true
}

// fraction returns part/whole clamped to 1. ok is false when it exceeds 1 by
// more than accumulatorOvershoot.
func fraction(part, whole uint64) (float64, bool) {
	f := float64(part) / float64(whole)
	if f > 1+accumulatorOvershoot {
		return 0, false
	}
	return min(f, 1), true
}

// activity holds the interval averages of the activity of a device, in
// percent. A value is invalid when it cannot be computed for the interval.
type activity struct {
	graphics amdgpu.Reading // graphics is the socket's graphics activity, the mean over its XCDs, for gr_engine_active and sm_active
	memory   amdgpu.Reading // memory is the HBM bandwidth utilization, for dram_active
}

// activityAverages returns the activity of a device between two tables: the
// increase of each activity accumulator divided by the increase of the
// accumulation counter. The graphics activity is the socket mean over its
// XCDs, the same statistic as gpu_busy_percent. A decreased accumulator (a
// reset or a 32-bit wrap) or an average over 100% beyond the tolerated
// overshoot leaves the value invalid; implausible reports the latter.
func activityAverages(previous, current *amdgpu.GPUMetrics) (act activity, implausible bool) {
	periods, ok := increase(previous.AccumulationCounter, current.AccumulationCounter)
	// Over a longer interval a 32-bit accumulator can wrap and still end higher.
	if !ok || periods == 0 || periods > math.MaxUint32/100 {
		return activity{}, false
	}
	average := func(prev, cur amdgpu.Counter) amdgpu.Reading {
		percent, ok := increase(prev, cur)
		if !ok {
			return amdgpu.Reading{}
		}
		f, ok := fraction(percent, 100*periods)
		if !ok {
			implausible = true
			return amdgpu.Reading{}
		}
		return amdgpu.Reading{Value: 100 * f, Valid: true}
	}
	act.graphics = average(previous.GFXActivityAcc, current.GFXActivityAcc)
	act.memory = average(previous.MemActivityAcc, current.MemActivityAcc)
	return act, implausible
}

// chooseActivity returns the activity metrics of a device: the interval
// averages from gpu_metrics when available, otherwise the busy percentages
// sampled from sysfs, so that the metrics are never missing because of
// gpu_metrics. It counts the source of each metric in telemetry.
func (c *gpuMetricsCollector) chooseActivity(deviceUUID string, m *amdgpu.Metrics, averages activity) activity {
	choose := func(metric string, average, snapshot amdgpu.Reading) amdgpu.Reading {
		switch {
		case average.Valid:
			c.telemetry.activitySource.Inc(deviceUUID, metric, "accumulator")
			return average
		case snapshot.Valid:
			c.telemetry.activitySource.Inc(deviceUUID, metric, "snapshot")
		}
		return snapshot
	}
	return activity{
		graphics: choose("gr_engine_active", averages.graphics, m.GPUBusyPercent),
		memory:   choose("dram_active", averages.memory, m.MemoryBusyPercent),
	}
}

// throttleSamples returns the fraction of the interval between two tables
// during which the device was throttled for each reason. A counter that
// decreased (a reset or a wrap of a 32-bit counter) starts a new baseline
// without a sample. implausible reports a fraction above 1 beyond the
// tolerated overshoot.
func throttleSamples(previous, current *amdgpu.GPUMetrics) (samples []nvidia.Sample, implausible bool) {
	periods, ok := increase(previous.AccumulationCounter, current.AccumulationCounter)
	// Over a longer interval a 32-bit residency counter can wrap and still end higher.
	if !ok || periods == 0 || periods > math.MaxUint32 {
		return nil, false
	}
	for _, reason := range throttleReasons {
		throttled, ok := increase(reason.residency(previous), reason.residency(current))
		if !ok {
			continue
		}
		f, ok := fraction(throttled, periods)
		if !ok {
			implausible = true
			continue
		}
		samples = append(samples, nvidia.NewMetric(reason.metric, f, metrics.GaugeType, nvidia.Low, nil, nil))
	}
	return samples, implausible
}

// xgmiSamples returns the per-link and total XGMI data throughput, in KB/s,
// between two tables. Links unreported in both tables are ignored. A link
// reported in one table only, whose counter decreased, or whose rate exceeds
// the bound of xgmiLinkBound, has no sample; the totals are then omitted too,
// as they would undercount. implausible reports a rate over the bound.
func xgmiSamples(previous, current *amdgpu.GPUMetrics) (samples []nvidia.Sample, implausible bool) {
	elapsed, ok := increase(previous.SystemClockNs, current.SystemClockNs)
	if !ok || elapsed == 0 {
		return nil, false
	}
	seconds := float64(elapsed) / float64(time.Second)
	bound := xgmiLinkBound(current)

	direction := func(name string, previous, current []amdgpu.Counter) {
		if len(previous) != len(current) {
			return
		}
		total, complete, emitted := 0.0, true, false
		for link := range current {
			prev, cur := previous[link], current[link]
			if !prev.Valid && !cur.Valid {
				continue // link not reported
			}
			kb, ok := increase(prev, cur)
			if !ok {
				complete = false // reported in one table only, or decreased
				continue
			}
			rate := float64(kb) / seconds
			if rate > bound {
				implausible, complete = true, false
				continue
			}
			samples = append(samples, nvidia.NewMetric(name, rate, metrics.GaugeType, nvidia.Low, []string{"xgmi_link:" + strconv.Itoa(link)}, nil))
			total += rate
			emitted = true
		}
		if emitted && complete {
			samples = append(samples, nvidia.NewMetric(name+".total", total, metrics.GaugeType, nvidia.Low, nil, nil))
		}
	}
	direction("xgmi.throughput.data.rx", previous.XGMIReadKB, current.XGMIReadKB)
	direction("xgmi.throughput.data.tx", previous.XGMIWriteKB, current.XGMIWriteKB)
	return samples, implausible
}

// xgmiLinkBound returns the highest plausible rate, in KB/s, on one XGMI link
// and in one direction: xgmiLinkMargin times the bandwidth of its speed (Gb/s
// per lane) and width, or maxXGMILinkKBPerSecond without a sane speed and width.
func xgmiLinkBound(gm *amdgpu.GPUMetrics) float64 {
	speed, width := gm.XGMILinkSpeedGTs, gm.XGMILinkWidth
	if !speed.Valid || !width.Valid || speed.Value == 0 || speed.Value > 1000 || width.Value == 0 || width.Value > 64 {
		return maxXGMILinkKBPerSecond
	}
	const kbPerGbit = 1e9 / 8 / 1000
	return xgmiLinkMargin * float64(speed.Value*width.Value) * kbPerGbit
}

// forget drops the state of devices that are no longer present, so that a
// returning device is baselined again.
func (c *gpuMetricsCollector) forget(devices []*amdgpu.Device) {
	present := make(map[string]struct{}, len(devices))
	for _, dev := range devices {
		present[dev.UUID] = struct{}{}
	}
	for uuid := range c.devices {
		if _, ok := present[uuid]; !ok {
			delete(c.devices, uuid)
		}
	}
}
