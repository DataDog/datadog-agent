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

// gpuMetricsFields are the fields of a synthetic gpu_metrics table whose
// calibration fields match fakeAMDHost (JunctionOnlyHwmon and
// MI300XAttributes), with accumulated counters advanced by step periods.
func gpuMetricsFields(step uint64) map[string][]uint64 {
	return map[string][]uint64{
		"temperature_hotspot":      {41},
		"curr_socket_power":        {142},
		"pcie_link_width":          {16},
		"current_uclk":             {1300},
		"system_clock_counter":     {1e12 + step*15e9}, // 15 s per step
		"accumulation_counter":     {1e6 + step*1000},
		"ppt_residency_acc":        {5e5 + step*840},
		"socket_thm_residency_acc": {1e3 + step*10},
		"prochot_residency_acc":    {0},
		"hbm_thm_residency_acc":    {0},
		"vr_thm_residency_acc":     {math.MaxUint32}, // unfilled
		"pcie_replay_count_acc":    {3 + step},
		"xgmi_read_data_acc":       {math.MaxUint64, 1e6 + step*15e6, 2e6, 0, 0, 0, 0, 0},
		"xgmi_write_data_acc":      {math.MaxUint64, 1e6 + step*30e6, 2e6, 0, 0, 0, 0, 0},
		// Graphics busy 23.75% of the time on average over the XCDs; HBM at 3.5%.
		"gfx_activity_acc": {1e8 + step*23750},
		"mem_activity_acc": {1e7 + step*3500},
	}
}

func parseTable(t *testing.T, fields map[string][]uint64) *amdgpu.GPUMetrics {
	t.Helper()
	gm, err := amdgpu.ParseGPUMetrics(amdgpu.GPUMetricsTable(t, fields))
	require.NoError(t, err)
	return gm
}

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
		"hbm_thm_residency_acc":    {0},
		"vr_thm_residency_acc":     {0},
	})
	current := parseTable(t, map[string][]uint64{
		"accumulation_counter":     {2000},
		"ppt_residency_acc":        {940},
		"socket_thm_residency_acc": {50},
		"prochot_residency_acc":    {5},    // decreased: new baseline
		"hbm_thm_residency_acc":    {1030}, // a small overshoot is clamped
		"vr_thm_residency_acc":     {1200}, // implausible
	})
	samples, implausible := throttleSamples(previous, current)
	assert.True(t, implausible)
	values := sampleValues(samples)
	assert.Len(t, values, 3)
	requireValue(t, values, "clock.throttle_reasons.sw_power_cap", 0.84)
	requireValue(t, values, "clock.throttle_reasons.sw_thermal_slowdown", 0) // a real 0, not a dropped sample
	requireValue(t, values, "clock.throttle_reasons.hbm_thermal", 1)
	assert.NotContains(t, values, "clock.throttle_reasons.hw_slowdown", "decreased counter")
	assert.NotContains(t, values, "clock.throttle_reasons.vr_thermal", "implausible")

	samples, implausible = throttleSamples(current, previous)
	assert.Empty(t, samples, "the accumulation counter decreased")
	assert.False(t, implausible)
	samples, _ = throttleSamples(current, current)
	assert.Empty(t, samples, "no accumulation period elapsed")
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

	// A link that becomes unfilled is not silently left out of the total.
	current = table(3e9, []uint64{unfilled, 3000, unfilled, 0, 0, 0, 0, 0}, []uint64{unfilled, 0, 500, 0, 0, 0, 0, 0})
	values = sampleValues(xgmiSamplesOnly(xgmiSamples(previous, current)))
	assert.Contains(t, values, "xgmi.throughput.data.rx|xgmi_link:1")
	assert.NotContains(t, values, "xgmi.throughput.data.rx.total")
	assert.Contains(t, values, "xgmi.throughput.data.tx.total")
}

func xgmiSamplesOnly(samples []nvidia.Sample, _ bool) []nvidia.Sample { return samples }

func samplesOnly(samples []nvidia.Sample, _ activity) []nvidia.Sample { return samples }

// matchingSysfs returns the sysfs values of fakeAMDHost that the tables of
// gpuMetricsFields match.
func matchingSysfs() amdgpu.Metrics {
	return amdgpu.Metrics{
		PowerMilliwatts:      amdgpu.Reading{Value: 142000, Valid: true},
		JunctionTemperatureC: amdgpu.Reading{Value: 41, Valid: true},
		PCIeLinkWidth:        amdgpu.Reading{Value: 16, Valid: true},
	}
}

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

func TestGPUMetricsCollectorCalibration(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	read := func(step uint64, powerW float64) amdgpu.Metrics {
		return amdgpu.Metrics{
			GPUMetrics:      parseTable(t, gpuMetricsFields(step)),
			PowerMilliwatts: amdgpu.Reading{Value: powerW * 1000, Valid: true},
			XGMIHiveSize:    2,
		}
	}
	names := func(samples []nvidia.Sample) []string {
		var out []string
		for _, s := range samples {
			out = append(out, s.(*nvidia.Metric).Name)
		}
		return out
	}
	samplesAt := func(elapsed time.Duration, m amdgpu.Metrics) []string {
		samples, _ := c.samples(dev, &m, true, start.Add(elapsed))
		return names(samples)
	}

	assert.Equal(t, []string{"pci.replay_counter"}, samplesAt(0, read(0, 142)), "the first table is a baseline")
	assert.Contains(t, samplesAt(15*time.Second, read(1, 142)), "clock.throttle_reasons.sw_power_cap")
	assert.Contains(t, samplesAt(30*time.Second, read(2, 900)), "xgmi.throughput.data.rx.total", "calibration is not repeated within the recheck interval")
	assert.Equal(t, 3.0, countTelemetry(t, tm, "gpu_metrics_reads", map[string]string{"device": "amd-1", "version": "1.9"}))

	assert.Empty(t, samplesAt(11*time.Minute, read(3, 900)), "a recheck mismatch withdraws trust")
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "calibration_mismatch"}))
	assert.Empty(t, samplesAt(11*time.Minute+15*time.Second, read(4, 142)), "not retried before the retry interval")
	assert.Equal(t, []string{"pci.replay_counter"}, samplesAt(12*time.Minute+15*time.Second, read(5, 142)), "trusted again, with a new baseline")
	assert.Contains(t, samplesAt(12*time.Minute+30*time.Second, read(6, 142)), "clock.throttle_reasons.sw_power_cap")

	// A new version is calibrated again and starts a new baseline.
	m := read(7, 142)
	m.GPUMetrics.Version.Content = 10
	assert.Equal(t, []string{"pci.replay_counter"}, samplesAt(12*time.Minute+45*time.Second, m))
	m = read(8, 900)
	m.GPUMetrics.Version.Content = 9
	assert.Empty(t, samplesAt(13*time.Minute, m), "a version change is calibrated at once")
	assert.Equal(t, 2.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "calibration_mismatch"}))
}

func TestGPUMetricsCollectorRejectedTables(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	now := time.Unix(1000, 0)

	unsupported, err := amdgpu.ParseGPUMetrics([]byte{120, 0, 1, 8})
	require.ErrorIs(t, err, amdgpu.ErrGPUMetricsUnsupported)
	m := matchingSysfs()
	m.GPUMetrics, m.GPUMetricsErr = unsupported, err
	assert.Empty(t, samplesOnly(c.samples(dev, &m, true, now)))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_reads", map[string]string{"version": "1.8"}))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "unsupported_version"}))
	assert.False(t, c.readDue(dev.UUID, now.Add(15*time.Second)), "an unsupported table is not read on every run")
	assert.True(t, c.readDue(dev.UUID, now.Add(gpuMetricsRetryInterval)), "but retried on the retry schedule")

	now = now.Add(gpuMetricsRetryInterval)
	malformed, err := amdgpu.ParseGPUMetrics([]byte{9, 0, 1, 9, 0, 0, 0, 0, 0})
	require.ErrorIs(t, err, amdgpu.ErrGPUMetricsMalformed)
	m = matchingSysfs()
	m.GPUMetrics, m.GPUMetricsErr = malformed, err
	assert.Empty(t, samplesOnly(c.samples(dev, &m, true, now)))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "malformed"}))
	assert.False(t, c.readDue(dev.UUID, now.Add(15*time.Second)))

	other := &amdgpu.Device{UUID: "amd-2"}
	assert.Empty(t, samplesOnly(c.samples(other, &amdgpu.Metrics{}, true, now)), "no gpu_metrics attribute")
	assert.NotContains(t, c.devices, other.UUID)
	assert.True(t, c.readDue(other.UUID, now))

	// Without any sysfs value to compare with, a table is not trusted.
	m = amdgpu.Metrics{GPUMetrics: parseTable(t, gpuMetricsFields(0))}
	now = now.Add(gpuMetricsRetryInterval)
	assert.Empty(t, samplesOnly(c.samples(dev, &m, true, now)))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "calibration_unavailable"}))
	assert.False(t, c.readDue(dev.UUID, now.Add(15*time.Second)), "untrusted, between retries")
	assert.Empty(t, samplesOnly(c.samples(dev, &amdgpu.Metrics{}, false, now.Add(15*time.Second))), "a skipped read changes nothing")

	// Fields with an unexpected encoding are skipped and counted.
	fields := gpuMetricsFields(0)
	fields["xgmi_read_data_acc"] = fields["xgmi_read_data_acc"][:4]
	m = matchingSysfs()
	m.GPUMetrics, m.XGMIHiveSize = parseTable(t, fields), 2
	assert.NotEmpty(t, samplesOnly(c.samples(dev, &m, true, now.Add(gpuMetricsRetryInterval))))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "invalid_field"}))
	assert.True(t, c.readDue(dev.UUID, now.Add(gpuMetricsRetryInterval+15*time.Second)), "trusted tables are read on every run")

	c.forget(nil)
	assert.Empty(t, c.devices)
}

// A failed read or a counter reset forces a new calibration and baseline,
// without waiting for the recheck interval.
func TestGPUMetricsCollectorRecalibrates(t *testing.T) {
	tm := testutil.GetTelemetryMock(t)
	c := newGPUMetricsCollector(tm)
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	read := func(step uint64, powerW float64) *amdgpu.Metrics {
		m := matchingSysfs()
		m.GPUMetrics = parseTable(t, gpuMetricsFields(step))
		m.PowerMilliwatts.Value = powerW * 1000
		return &m
	}
	names := func(samples []nvidia.Sample, _ activity) []string {
		var out []string
		for _, s := range samples {
			out = append(out, s.(*nvidia.Metric).Name)
		}
		return out
	}

	require.Equal(t, []string{"pci.replay_counter"}, names(c.samples(dev, read(5, 142), true, start)))
	require.Contains(t, names(c.samples(dev, read(6, 142), true, start.Add(15*time.Second))), "clock.throttle_reasons.sw_power_cap")

	// The read fails, then sysfs disagrees: the table is calibrated again at once.
	assert.Empty(t, names(c.samples(dev, &amdgpu.Metrics{}, true, start.Add(30*time.Second))))
	assert.Empty(t, names(c.samples(dev, read(7, 900), true, start.Add(45*time.Second))))
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "calibration_mismatch"}))

	// Trusted again at the next retry, then the counters go back.
	start = start.Add(45*time.Second + gpuMetricsRetryInterval)
	require.Equal(t, []string{"pci.replay_counter"}, names(c.samples(dev, read(8, 142), true, start)))
	assert.Empty(t, names(c.samples(dev, read(2, 900), true, start.Add(15*time.Second))), "a reset is calibrated again at once")
	assert.Equal(t, 1.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "counter_reset"}))
	assert.Equal(t, 2.0, countTelemetry(t, tm, "gpu_metrics_failures", map[string]string{"reason": "calibration_mismatch"}))
}

func TestGPUMetricsXGMIRequiresAHivePeer(t *testing.T) {
	c := newGPUMetricsCollector(testutil.GetTelemetryMock(t))
	dev := &amdgpu.Device{UUID: "amd-1"}
	start := time.Unix(1000, 0)
	for step := range uint64(2) {
		m := matchingSysfs()
		m.GPUMetrics, m.XGMIHiveSize = parseTable(t, gpuMetricsFields(step)), 1
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
		"gpu.clock.throttle_reasons.hbm_thermal":         0,
		"gpu.pci.replay_counter":                         4,
		"gpu.gr_engine_active":                           23.75, // the socket mean over the interval
		"gpu.sm_active":                                  23.75,
		"gpu.dram_active":                                3.5,
	} {
		require.Len(t, gauges[name], 1, name)
		assert.InDelta(t, value, gauges[name][0].Arguments.Get(1), 1e-9, name)
		assert.Contains(t, gauges[name][0].Arguments.Get(3), "gpu_uuid:"+testAMDUUID, name)
	}
	assert.NotContains(t, gauges, "gpu.clock.throttle_reasons.vr_thermal", "unfilled counter")
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
