// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package amd

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// gpuMetricsFields are the fields of a synthetic gpu_metrics table, with
// accumulated counters advanced by step periods of 15 s at 1 ms per
// accumulation period.
func gpuMetricsFields(step uint64) map[string][]uint64 {
	return map[string][]uint64{
		"system_clock_counter":     {1e12 + step*15e9},
		"accumulation_counter":     {1e6 + step*15000},
		"ppt_residency_acc":        {5e5 + step*12600}, // 84%
		"socket_thm_residency_acc": {1e3 + step*150},   // 1%
		"prochot_residency_acc":    {0},
		"pcie_replay_count_acc":    {3 + step},
		"xgmi_read_data_acc":       {math.MaxUint64, 1e6 + step*15e6, 2e6, 0, 0, 0, 0, 0},
		"xgmi_write_data_acc":      {math.MaxUint64, 1e6 + step*30e6, 2e6, 0, 0, 0, 0, 0},
		"xgmi_link_speed":          {32},
		"xgmi_link_width":          {16},
		"current_uclk":             {1300},
		// Graphics busy 23.75% of the time on average over the XCDs; HBM at 3.5%.
		"gfx_activity_acc": {1e8 + step*356250},
		"mem_activity_acc": {1e7 + step*52500},
	}
}

func parseTable(t *testing.T, fields map[string][]uint64) *amdgpu.GPUMetrics {
	t.Helper()
	gm, err := amdgpu.ParseGPUMetrics(amdgpu.GPUMetricsTable(t, fields))
	require.NoError(t, err)
	return gm
}

// u32 is the modulus of a 32-bit counter.
const u32 = uint64(math.MaxUint32) + 1

// sampleValues returns the values of metric samples, by name and tags.
func sampleValues(samples []nvidia.Sample) map[string]float64 {
	values := make(map[string]float64, len(samples))
	for _, s := range samples {
		m := s.(*nvidia.Metric)
		key := m.Name
		if tags := m.Tags(); len(tags) > 0 {
			key += "|" + strings.Join(tags, ",")
		}
		values[key] = m.Value
	}
	return values
}

func TestThrottleSamples(t *testing.T) {
	previous := parseTable(t, map[string][]uint64{
		"accumulation_counter":     {1000},
		"ppt_residency_acc":        {100},
		"socket_thm_residency_acc": {50},
		"prochot_residency_acc":    {10},
	})
	current := parseTable(t, map[string][]uint64{
		"accumulation_counter":     {2000},
		"ppt_residency_acc":        {940},
		"socket_thm_residency_acc": {50},
		"prochot_residency_acc":    {5}, // decreased: new baseline
	})
	samples, implausible := throttleSamples(previous, current)
	assert.False(t, implausible)
	values := sampleValues(samples)
	assert.Len(t, values, 2)
	requireValue(t, values, "clock.throttle_reasons.sw_power_cap", 0.84)
	requireValue(t, values, "clock.throttle_reasons.sw_thermal_slowdown", 0) // a real 0, not a dropped sample
	assert.NotContains(t, values, "clock.throttle_reasons.hw_slowdown", "decreased counter")

	overshoot := parseTable(t, map[string][]uint64{
		"accumulation_counter":     {2000},
		"ppt_residency_acc":        {1130}, // a small overshoot is clamped
		"socket_thm_residency_acc": {1250}, // implausible
	})
	samples, implausible = throttleSamples(previous, overshoot)
	assert.True(t, implausible)
	values = sampleValues(samples)
	requireValue(t, values, "clock.throttle_reasons.sw_power_cap", 1)
	assert.NotContains(t, values, "clock.throttle_reasons.sw_thermal_slowdown", "implausible")

	samples, implausible = throttleSamples(current, previous)
	assert.Empty(t, samples, "the accumulation counter decreased")
	assert.False(t, implausible)
	samples, _ = throttleSamples(current, current)
	assert.Empty(t, samples, "no accumulation period elapsed")
}

// A 32-bit accumulator can wrap and end above its previous value over a long
// gap: no value is computed over more periods than it can hold.
func TestAccumulatorWrapOverLongGap(t *testing.T) {
	// 13.9 h of 1 ms periods at 100% busy: gfx_activity_acc wraps once.
	periods := uint64(50e6)
	previous := parseTable(t, map[string][]uint64{"accumulation_counter": {1e6}, "gfx_activity_acc": {1000}, "mem_activity_acc": {0}})
	current := parseTable(t, map[string][]uint64{"accumulation_counter": {1e6 + periods}, "gfx_activity_acc": {(1000 + 100*periods) % u32}, "mem_activity_acc": {0}})
	act, implausible := activityAverages(previous, current)
	assert.Equal(t, activity{}, act, "not a plausible 14 percent")
	assert.False(t, implausible)

	// 51 days throttled throughout: ppt_residency_acc wraps once.
	periods = 4.4e9
	previous = parseTable(t, map[string][]uint64{"accumulation_counter": {1e6}, "ppt_residency_acc": {0}})
	current = parseTable(t, map[string][]uint64{"accumulation_counter": {1e6 + periods}, "ppt_residency_acc": {periods % u32}})
	samples, implausible := throttleSamples(previous, current)
	assert.Empty(t, samples, "not a plausible 2 percent")
	assert.False(t, implausible)

	// The longest intervals still computed.
	periods = math.MaxUint32 / 100
	current = parseTable(t, map[string][]uint64{"accumulation_counter": {1e6 + periods}, "gfx_activity_acc": {1000 + 50*periods}, "mem_activity_acc": {0}})
	act, _ = activityAverages(parseTable(t, map[string][]uint64{"accumulation_counter": {1e6}, "gfx_activity_acc": {1000}, "mem_activity_acc": {0}}), current)
	requireReading(t, 50, act.graphics)
	current = parseTable(t, map[string][]uint64{"accumulation_counter": {1e6 + math.MaxUint32}, "ppt_residency_acc": {math.MaxUint32 / 2}})
	samples, _ = throttleSamples(previous, current)
	assert.Len(t, samples, 1)
}

// requireValue checks that a sample is present, so that a dropped sample is
// not mistaken for a 0, and has the given value.
func requireValue(t *testing.T, values map[string]float64, key string, want float64) {
	t.Helper()
	value, ok := values[key]
	if assert.True(t, ok, "%s not emitted", key) {
		assert.InDelta(t, want, value, 1e-9, key)
	}
}

func TestXGMISamples(t *testing.T) {
	table := func(clock uint64, read, write []uint64) *amdgpu.GPUMetrics {
		return parseTable(t, map[string][]uint64{
			"system_clock_counter": {clock},
			"xgmi_read_data_acc":   read,
			"xgmi_write_data_acc":  write,
		})
	}
	unfilled := uint64(math.MaxUint64)
	previous := table(1e9, []uint64{unfilled, 1000, 0, 0, 0, 0, 0, 0}, []uint64{unfilled, 0, 500, 0, 0, 0, 0, 0})

	current := table(3e9, []uint64{unfilled, 3000, 4000, 0, 0, 0, 0, 0}, []uint64{unfilled, 1000, 400, 0, 0, 0, 0, 0})
	samples, implausible := xgmiSamples(previous, current)
	assert.False(t, implausible)
	values := sampleValues(samples)
	requireValue(t, values, "xgmi.throughput.data.rx|xgmi_link:1", 1000)
	requireValue(t, values, "xgmi.throughput.data.rx|xgmi_link:2", 2000)
	requireValue(t, values, "xgmi.throughput.data.rx|xgmi_link:3", 0) // an idle link, not a dropped sample
	assert.NotContains(t, values, "xgmi.throughput.data.rx|xgmi_link:0", "unfilled link")
	requireValue(t, values, "xgmi.throughput.data.rx.total", 3000)
	requireValue(t, values, "xgmi.throughput.data.tx|xgmi_link:1", 500)
	assert.NotContains(t, values, "xgmi.throughput.data.tx|xgmi_link:2", "decreased counter")
	assert.NotContains(t, values, "xgmi.throughput.data.tx.total", "a total missing a link would undercount")

	current = table(3e9, []uint64{unfilled, 1000 + 3e9, 0, 0, 0, 0, 0, 0}, []uint64{unfilled, 0, 500, 0, 0, 0, 0, 0})
	samples, implausible = xgmiSamples(previous, current)
	assert.True(t, implausible)
	values = sampleValues(samples)
	assert.NotContains(t, values, "xgmi.throughput.data.rx|xgmi_link:1", "1.5 TB/s on one link")
	assert.NotContains(t, values, "xgmi.throughput.data.rx.total")
	assert.Contains(t, values, "xgmi.throughput.data.tx.total")

	samples, _ = xgmiSamples(current, previous)
	assert.Empty(t, samples, "the clock went back")

	// With a link speed and width, the bound is twice the link bandwidth: 128 GB/s at 32 Gb/s x16.
	linked := func(clock uint64, read uint64) *amdgpu.GPUMetrics {
		return parseTable(t, map[string][]uint64{
			"system_clock_counter": {clock},
			"xgmi_read_data_acc":   {read, 0, 0, 0, 0, 0, 0, 0},
			"xgmi_link_speed":      {32},
			"xgmi_link_width":      {16},
		})
	}
	samples, implausible = xgmiSamples(linked(1e9, 0), linked(2e9, 120e6))
	assert.False(t, implausible)
	requireValue(t, sampleValues(samples), "xgmi.throughput.data.rx|xgmi_link:0", 120e6)
	samples, implausible = xgmiSamples(linked(1e9, 0), linked(2e9, 130e6))
	assert.True(t, implausible)
	assert.NotContains(t, sampleValues(samples), "xgmi.throughput.data.rx|xgmi_link:0")
	for name, gm := range map[string]*amdgpu.GPUMetrics{
		"absent":     parseTable(t, map[string][]uint64{"xgmi_link_speed": {32}}),
		"zero width": parseTable(t, map[string][]uint64{"xgmi_link_speed": {32}, "xgmi_link_width": {0}}),
		"unfilled":   parseTable(t, map[string][]uint64{"xgmi_link_speed": {math.MaxUint16}, "xgmi_link_width": {16}}),
	} {
		assert.Equal(t, float64(maxXGMILinkKBPerSecond), xgmiLinkBound(gm), name)
	}

	// A link that becomes unfilled is not silently left out of the total.
	current = table(3e9, []uint64{unfilled, 3000, unfilled, 0, 0, 0, 0, 0}, []uint64{unfilled, 0, 500, 0, 0, 0, 0, 0})
	values = sampleValues(xgmiSamplesOnly(xgmiSamples(previous, current)))
	assert.Contains(t, values, "xgmi.throughput.data.rx|xgmi_link:1")
	assert.NotContains(t, values, "xgmi.throughput.data.rx.total")
	assert.Contains(t, values, "xgmi.throughput.data.tx.total")
}

func xgmiSamplesOnly(samples []nvidia.Sample, _ bool) []nvidia.Sample { return samples }

func samplesOnly(samples []nvidia.Sample, _ activity) []nvidia.Sample { return samples }

// requireReading checks that a reading is valid and has the given value.
func requireReading(t *testing.T, want float64, r amdgpu.Reading) {
	t.Helper()
	if assert.True(t, r.Valid) {
		assert.InDelta(t, want, r.Value, 1e-9)
	}
}

func TestActivityAverages(t *testing.T) {
	table := func(acc, gfx, mem uint64) *amdgpu.GPUMetrics {
		return parseTable(t, map[string][]uint64{
			"accumulation_counter": {acc},
			"gfx_activity_acc":     {gfx},
			"mem_activity_acc":     {mem},
		})
	}
	previous := table(1000, 100, 0)

	// 15000 periods: the socket mean over the XCDs is 28%, HBM at 3.5%.
	current := table(16000, 100+15000*28, 15000*3.5)
	act, implausible := activityAverages(previous, current)
	assert.False(t, implausible)
	requireReading(t, 28, act.graphics)
	requireReading(t, 3.5, act.memory)

	// A small overshoot is clamped to 100; a larger one is implausible.
	act, implausible = activityAverages(previous, table(16000, 100+15000*104, 15000*106))
	assert.True(t, implausible)
	requireReading(t, 100, act.graphics)
	assert.False(t, act.memory.Valid)

	act, implausible = activityAverages(previous, table(16000, 50, 0))
	assert.False(t, act.graphics.Valid, "decreased: new baseline")
	assert.True(t, act.memory.Valid)
	assert.False(t, implausible)
	act, _ = activityAverages(current, previous)
	assert.Equal(t, activity{}, act, "the accumulation counter went back")
	act, _ = activityAverages(current, current)
	assert.Equal(t, activity{}, act, "no accumulation period elapsed")
}

func TestChooseActivity(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	snapshot := amdgpu.Metrics{GPUBusyPercent: amdgpu.Reading{Value: 96, Valid: true}, MemoryBusyPercent: amdgpu.Reading{Value: 4, Valid: true}}

	act := c.chooseActivity("amd-1", &snapshot, activity{graphics: amdgpu.Reading{Value: 28.5, Valid: true}})
	assert.Equal(t, activity{graphics: amdgpu.Reading{Value: 28.5, Valid: true}, memory: amdgpu.Reading{Value: 4, Valid: true}}, act)
	act = c.chooseActivity("amd-1", &amdgpu.Metrics{}, activity{})
	assert.Equal(t, activity{}, act, "nothing to emit")

	assert.Equal(t, 1.0, countTelemetry(t, tm, "activity_source", map[string]string{"metric": "gr_engine_active", "source": "accumulator"}))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "activity_source", map[string]string{"metric": "dram_active", "source": "snapshot"}))
	assert.Equal(t, 0.0, countTelemetry(t, tm, "activity_source", map[string]string{"metric": "gr_engine_active", "source": "snapshot"}))
}

// countTelemetry returns the value of a counter of the check, by tag values.
func countTelemetry(t *testing.T, tm telemetry.Mock, name string, tags map[string]string) float64 {
	t.Helper()
	metrics, err := tm.GetCountMetric(CheckName, name)
	if err != nil {
		return 0
	}
	total := 0.0
	for _, m := range metrics {
		match := true
		for k, v := range tags {
			match = match && m.Tags()[k] == v
		}
		if match {
			total += m.Value()
		}
	}
	return total
}

func readAt(t *testing.T, fields map[string][]uint64) *amdgpu.Metrics {
	t.Helper()
	return &amdgpu.Metrics{GPUMetrics: parseTable(t, fields), XGMIHiveSize: 2}
}

// A graphics activity that contradicts GFX_BUSY_ACC is dropped, and the
// interval average falls back to the sysfs snapshot.
func TestGPUMetricsCollectorGFXBusyInvariant(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	read := func(step uint64, busyOffset uint64) *amdgpu.Metrics {
		fields := gpuMetricsFields(step)
		mean := fields["gfx_activity_acc"][0] + busyOffset
		fields["gfx_busy_acc"] = []uint64{mean - 50, mean + 50, mean, mean, mean, mean, mean, mean}
		m := readAt(t, fields)
		m.GPUBusyPercent = amdgpu.Reading{Value: 61, Valid: true}
		return m
	}
	c.samples(dev, read(0, 0), true, start)
	m := read(1, 0)
	_, act := c.samples(dev, m, true, start.Add(15*time.Second))
	requireReading(t, 23.75, c.chooseActivity(dev.UUID, m, act).graphics)

	m = read(2, 1e6)
	samples, act := c.samples(dev, m, true, start.Add(30*time.Second))
	assert.Contains(t, metricNames(samples), "clock.throttle_reasons.sw_power_cap", "only the graphics activity is dropped")
	requireReading(t, 61, c.chooseActivity(dev.UUID, m, act).graphics)
	requireReading(t, 3.5, act.memory)
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "implausible_value"}))
}

func TestGPUMetricsCollectorRejectedTables(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	now := time.Unix(1000, 0)

	unsupported, err := amdgpu.ParseGPUMetrics([]byte{120, 0, 1, 8})
	require.ErrorIs(t, err, amdgpu.ErrGPUMetricsUnsupported)
	m := amdgpu.Metrics{GPUMetrics: unsupported, GPUMetricsErr: err}
	assert.Empty(t, samplesOnly(c.samples(dev, &m, true, now)))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_reads", map[string]string{"version": "1.8"}))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "unsupported_version"}))
	assert.False(t, c.readDue(dev.UUID, now.Add(15*time.Second)), "an unsupported table is not read on every run")
	assert.True(t, c.readDue(dev.UUID, now.Add(gpuMetricsRetryInterval)), "but retried on the retry schedule")

	now = now.Add(gpuMetricsRetryInterval)
	malformed, err := amdgpu.ParseGPUMetrics([]byte{9, 0, 1, 9, 0, 0, 0, 0, 0})
	require.ErrorIs(t, err, amdgpu.ErrGPUMetricsMalformed)
	m = amdgpu.Metrics{GPUMetrics: malformed, GPUMetricsErr: err}
	assert.Empty(t, samplesOnly(c.samples(dev, &m, true, now)))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "malformed"}))
	assert.False(t, c.readDue(dev.UUID, now.Add(15*time.Second)))
	assert.Empty(t, samplesOnly(c.samples(dev, &amdgpu.Metrics{}, false, now.Add(15*time.Second))), "a skipped read changes nothing")

	other := &amdgpu.Device{UUID: "amd-2"}
	assert.Empty(t, samplesOnly(c.samples(other, &amdgpu.Metrics{}, true, now)), "no gpu_metrics attribute")
	assert.NotContains(t, c.devices, other.UUID)
	assert.True(t, c.readDue(other.UUID, now))

	// A field with an unexpected unit rejects the table, and the snapshot is used.
	table := amdgpu.GPUMetricsTable(t, gpuMetricsFields(0))
	now = now.Add(gpuMetricsRetryInterval)
	m = amdgpu.Metrics{GPUBusyPercent: amdgpu.Reading{Value: 37, Valid: true}}
	m.GPUMetrics, m.GPUMetricsErr = amdgpu.ParseGPUMetrics(amdgpu.GPUMetricsTableWithUnit(t, table, "pcie_replay_count_acc", 3))
	require.ErrorIs(t, m.GPUMetricsErr, amdgpu.ErrGPUMetricsUnexpectedEncoding)
	samples, act := c.samples(dev, &m, true, now)
	assert.Empty(t, samples)
	assert.Equal(t, 37.0, c.chooseActivity(dev.UUID, &m, act).graphics.Value, "the snapshot is used")
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "unexpected_encoding"}))
	assert.False(t, c.readDue(dev.UUID, now.Add(15*time.Second)))
	assert.Equal(t, m.GPUMetricsErr.Error(), c.devices[dev.UUID].logged, "logged once until it changes")

	// Another instance count only skips the field.
	fields := gpuMetricsFields(1)
	fields["xgmi_read_data_acc"] = fields["xgmi_read_data_acc"][:4]
	now = now.Add(gpuMetricsRetryInterval)
	assert.NotEmpty(t, samplesOnly(c.samples(dev, readAt(t, fields), true, now)), "used once the encodings are expected")
	assert.True(t, c.readDue(dev.UUID, now.Add(15*time.Second)), "decoded tables are read on every run")
	assert.Empty(t, c.devices[dev.UUID].logged)

	c.forget(nil)
	assert.Empty(t, c.devices)
}

// A failed read or a counter reset starts a new baseline.
func TestGPUMetricsCollectorBaselineResets(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	names := func(samples []nvidia.Sample, _ activity) []string { return metricNames(samples) }

	require.Equal(t, []string{"pci.replay_counter"}, names(c.samples(dev, readAt(t, gpuMetricsFields(5)), true, start)))
	require.Contains(t, names(c.samples(dev, readAt(t, gpuMetricsFields(6)), true, start.Add(15*time.Second))), "clock.throttle_reasons.sw_power_cap")

	assert.Empty(t, names(c.samples(dev, &amdgpu.Metrics{}, true, start.Add(30*time.Second))), "the read fails")
	assert.Equal(t, []string{"pci.replay_counter"}, names(c.samples(dev, readAt(t, gpuMetricsFields(8)), true, start.Add(45*time.Second))), "a new baseline")
	assert.Equal(t, []string{"pci.replay_counter"}, names(c.samples(dev, readAt(t, gpuMetricsFields(2)), true, start.Add(60*time.Second))), "the counters went back")
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "counter_reset"}))
	assert.Contains(t, names(c.samples(dev, readAt(t, gpuMetricsFields(3)), true, start.Add(75*time.Second))), "clock.throttle_reasons.sw_power_cap")

	// A new version starts a new baseline.
	m := readAt(t, gpuMetricsFields(4))
	m.GPUMetrics.Version.Content = 10
	assert.Equal(t, []string{"pci.replay_counter"}, names(c.samples(dev, m, true, start.Add(90*time.Second))))
	assert.Zero(t, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "implausible_value"}))
}

// Moving only the system clock back is a reset too.
func TestGPUMetricsCollectorSystemClockReset(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	read := func(step uint64, clockNs uint64) *amdgpu.Metrics {
		fields := gpuMetricsFields(step)
		fields["system_clock_counter"] = []uint64{clockNs}
		return readAt(t, fields)
	}
	c.samples(dev, read(1, 2e12), true, start)
	samples, _ := c.samples(dev, read(2, 1e12), true, start.Add(15*time.Second))
	assert.Equal(t, []string{"pci.replay_counter"}, metricNames(samples), "a new baseline")
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "counter_reset"}))
}

// A device that disappears and returns is baselined again.
func TestGPUMetricsCollectorForget(t *testing.T) {
	c := newGPUMetricsCollector(testutil.GetTelemetryMock(t))
	dev := &amdgpu.Device{UUID: "amd-1"}
	other := &amdgpu.Device{UUID: "amd-2"}
	start := time.Unix(1000, 0)
	c.samples(dev, readAt(t, gpuMetricsFields(0)), true, start)
	c.samples(other, readAt(t, gpuMetricsFields(0)), true, start)
	c.forget([]*amdgpu.Device{other})
	assert.NotContains(t, c.devices, dev.UUID)
	assert.Contains(t, c.devices, other.UUID)

	samples, act := c.samples(dev, readAt(t, gpuMetricsFields(1)), true, start.Add(15*time.Second))
	assert.Equal(t, []string{"pci.replay_counter"}, metricNames(samples), "no baseline after a return")
	assert.Equal(t, activity{}, act)
	samples, _ = c.samples(other, readAt(t, gpuMetricsFields(1)), true, start.Add(15*time.Second))
	assert.Contains(t, metricNames(samples), "clock.throttle_reasons.sw_power_cap", "a present device keeps its baseline")
}

// The interval math on Linux 7.3 tables, whose accumulators are U64 and whose
// activity accumulators have no unit. The firmware residency counters are
// 32-bit, so they wrap inside their U64 fields.
func TestGPUMetricsCollectorLinux73(t *testing.T) {
	c := newGPUMetricsCollector(testutil.GetTelemetryMock(t))
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	read := func(step uint64) *amdgpu.Metrics {
		fields := gpuMetricsFields(step)
		fields["accumulation_counter"] = []uint64{1<<40 + step*15000}
		fields["ppt_residency_acc"] = []uint64{(math.MaxUint32 - 20000 + step*12600) % u32}
		fields["socket_thm_residency_acc"] = []uint64{2e9 + step*150}
		fields["gfx_activity_acc"] = []uint64{1<<45 + step*356250}
		gm, err := amdgpu.ParseGPUMetrics(amdgpu.GPUMetricsTableLinux73(t, fields))
		require.NoError(t, err)
		return &amdgpu.Metrics{GPUMetrics: gm}
	}
	c.samples(dev, read(0), true, start)
	samples, act := c.samples(dev, read(1), true, start.Add(15*time.Second))
	values := sampleValues(samples)
	requireValue(t, values, "clock.throttle_reasons.sw_power_cap", 0.84)
	requireValue(t, values, "clock.throttle_reasons.sw_thermal_slowdown", 0.01)
	requireValue(t, values, "clock.throttle_reasons.hw_slowdown", 0)
	requireValue(t, values, "pci.replay_counter", 4)
	requireReading(t, 23.75, act.graphics)
	requireReading(t, 3.5, act.memory)

	// The PPT residency wraps at 2^32: no sample for that interval, then a new baseline.
	samples, _ = c.samples(dev, read(2), true, start.Add(30*time.Second))
	values = sampleValues(samples)
	assert.NotContains(t, values, "clock.throttle_reasons.sw_power_cap")
	requireValue(t, values, "clock.throttle_reasons.sw_thermal_slowdown", 0.01)
	samples, _ = c.samples(dev, read(3), true, start.Add(45*time.Second))
	requireValue(t, sampleValues(samples), "clock.throttle_reasons.sw_power_cap", 0.84)
}

// On SMU 15.0.8 the throttle fractions and the activity averages are emitted;
// XGMI (one instance) and CURRENT_UCLK (2 AIDs) are skipped.
func TestGPUMetricsCollectorSMU1508(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	read := func(step uint64) *amdgpu.Metrics {
		fields := gpuMetricsFields(step)
		fields["xgmi_read_data_acc"] = []uint64{1e6 + step*15e6}
		fields["xgmi_write_data_acc"] = []uint64{1e6 + step*15e6}
		fields["current_uclk"] = []uint64{1300, 1300}
		gm, err := amdgpu.ParseGPUMetrics(amdgpu.GPUMetricsTableLinux73(t, fields))
		require.NoError(t, err)
		return &amdgpu.Metrics{GPUMetrics: gm, XGMIHiveSize: 8}
	}
	c.samples(dev, read(0), true, start)
	samples, act := c.samples(dev, read(1), true, start.Add(15*time.Second))
	values := sampleValues(samples)
	requireValue(t, values, "clock.throttle_reasons.sw_power_cap", 0.84)
	requireValue(t, values, "clock.throttle_reasons.sw_thermal_slowdown", 0.01)
	requireValue(t, values, "pci.replay_counter", 4)
	requireReading(t, 23.75, act.graphics)
	requireReading(t, 3.5, act.memory)
	for name := range values {
		assert.NotContains(t, name, "xgmi")
	}
	assert.Zero(t, countTelemetry(t, tm, "gpu_metrics_failures", nil))
}

func metricNames(samples []nvidia.Sample) []string {
	var out []string
	for _, s := range samples {
		out = append(out, s.(*nvidia.Metric).Name)
	}
	return out
}

func TestGPUMetricsXGMIRequiresAHivePeer(t *testing.T) {
	c := newGPUMetricsCollector(testutil.GetTelemetryMock(t))
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	for step := range uint64(2) {
		m := amdgpu.Metrics{GPUMetrics: parseTable(t, gpuMetricsFields(step)), XGMIHiveSize: 1}
		for _, s := range samplesOnly(c.samples(dev, &m, true, start.Add(time.Duration(step)*15*time.Second))) {
			assert.NotContains(t, s.(*nvidia.Metric).Name, "xgmi", "a GPU alone in its hive")
		}
	}
}

// TestAMDGPUMetricsEmitted runs the check on a host whose gpu_metrics table
// advances between runs, and checks the emitted metrics against the spec.
func TestAMDGPUMetricsEmitted(t *testing.T) {
	fs := amdgpu.NewFakeSysfs(t)
	fs.SetDriverVersion("6.14.14")
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amdgpu.MI300XAttributes("00c0ffee00c0ffee"))
	fs.AddHwmon(devDir, "hwmon0", amdgpu.JunctionOnlyHwmon())
	fs.AddCard("card0", devDir)
	writeTable := func(step uint64) {
		require.NoError(t, os.WriteFile(filepath.Join(devDir, "gpu_metrics"), amdgpu.GPUMetricsTable(t, gpuMetricsFields(step)), 0o644))
	}
	writeTable(0)
	check, mockSender := setupAMDCheck(t, fs.Root, nil)

	require.NoError(t, check.Run())
	gauges := emittedGauges(mockSender)
	require.Len(t, gauges["gpu.pci.replay_counter"], 1)
	assert.Equal(t, 3.0, gauges["gpu.pci.replay_counter"][0].Arguments.Get(1))
	assert.NotContains(t, gauges, "gpu.clock.throttle_reasons.sw_power_cap", "the first table is a baseline")
	for name, value := range map[string]float64{"gpu.gr_engine_active": 37, "gpu.sm_active": 37, "gpu.dram_active": 12} {
		require.Len(t, gauges[name], 1, name)
		assert.Equal(t, value, gauges[name][0].Arguments.Get(1), "%s: sysfs snapshot without a baseline", name)
	}

	writeTable(1)
	mockSender.ResetCalls()
	require.NoError(t, check.Run())
	gauges = emittedGauges(mockSender)
	for name, value := range map[string]float64{
		"gpu.clock.throttle_reasons.sw_power_cap":        0.84,
		"gpu.clock.throttle_reasons.sw_thermal_slowdown": 0.01,
		"gpu.clock.throttle_reasons.hw_slowdown":         0,
		"gpu.pci.replay_counter":                         4,
		"gpu.gr_engine_active":                           23.75, // the socket mean over the interval
		"gpu.sm_active":                                  23.75,
		"gpu.dram_active":                                3.5,
	} {
		require.Len(t, gauges[name], 1, name)
		assert.InDelta(t, value, gauges[name][0].Arguments.Get(1), 1e-9, name)
		assert.Contains(t, gauges[name][0].Arguments.Get(3), "gpu_uuid:"+testAMDUUID, name)
	}
	assert.NotContains(t, gauges, "gpu.clock.throttle_reasons.vr_thermal")
	assert.NotContains(t, gauges, "gpu.clock.throttle_reasons.hbm_thermal")
	for name := range gauges {
		assert.NotContains(t, name, "xgmi", "not in an XGMI hive")
	}

	// Join a hive of two GPUs.
	hive := filepath.Join(devDir, "xgmi_hive_info")
	require.NoError(t, os.MkdirAll(hive, 0o755))
	require.NoError(t, os.Symlink(devDir, filepath.Join(hive, "node1")))
	require.NoError(t, os.Symlink(devDir, filepath.Join(hive, "node2")))
	writeTable(2)
	mockSender.ResetCalls()
	require.NoError(t, check.Run())
	gauges = emittedGauges(mockSender)
	require.Len(t, gauges["gpu.xgmi.throughput.data.rx"], 7, "the 7 filled links")
	require.Len(t, gauges["gpu.xgmi.throughput.data.rx.total"], 1)
	assert.InDelta(t, 1e6, gauges["gpu.xgmi.throughput.data.rx.total"][0].Arguments.Get(1), 1e-6, "15e6 KB in 15 s")
	require.Len(t, gauges["gpu.xgmi.throughput.data.tx.total"], 1)
	assert.InDelta(t, 2e6, gauges["gpu.xgmi.throughput.data.tx.total"][0].Arguments.Get(1), 1e-6)

	// Check the values and tags of the gpu_metrics series against the spec.
	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	for name, observations := range gpu.GetEmittedGPUMetrics(mockSender) {
		if !strings.Contains(name, "throttle_reasons") && !strings.Contains(name, "xgmi") && !strings.Contains(name, "replay_counter") {
			continue
		}
		metricSpec, ok := specs.Metrics.Metrics[name]
		require.True(t, ok, "%s is not in the spec", name)
		expectedTags, _, err := gpuspec.ExpectedTagsForMetric(specs.Tags, metricSpec)
		require.NoError(t, err, name)
		for _, observation := range observations {
			require.NotNil(t, observation.Value, name)
			assert.NoError(t, metricSpec.Validator.ValidateStaticValue(*observation.Value), name)
			tags := gpuspec.TagsToKeyValues(observation.Tags)
			for tagName, values := range tags {
				tagSpec, ok := expectedTags[tagName]
				if !assert.True(t, ok, "%s has unexpected tag %s", name, tagName) {
					continue
				}
				for _, value := range values {
					if tagSpec.Regex != nil {
						assert.Regexp(t, tagSpec.Regex, value, "%s tag %s", name, tagName)
					}
				}
			}
			for _, tagset := range metricSpec.Tagsets {
				if tagset == "device" {
					continue // device tags vary by vendor and come from discovery
				}
				for _, tagName := range specs.Tags.Tagsets[tagset].Tags {
					assert.Contains(t, tags, tagName, "%s is missing tag %s", name, tagName)
				}
			}
		}
	}
}
