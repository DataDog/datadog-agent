// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package amd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
)

// fakeEventSource returns the queued events on the next flush.
type fakeEventSource struct {
	mu      sync.Mutex
	pending []amdgpu.Event
	syncErr error
	synced  [][]string
	closed  bool
}

func (f *fakeEventSource) Sync(devices []*amdgpu.Device) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var uuids []string
	for _, d := range devices {
		uuids = append(uuids, d.UUID)
	}
	f.synced = append(f.synced, uuids)
	return f.syncErr
}

func (f *fakeEventSource) Flush() ([]amdgpu.Event, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	events := f.pending
	f.pending = nil
	return events, 0
}

func (f *fakeEventSource) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeEventSource) queue(events ...amdgpu.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = append(f.pending, events...)
}

// errorMetric is an emitted errors.amdgpu sample, keyed by name and type.
type errorMetric struct {
	name, errType, origin string
}

func emittedErrors(t *testing.T, s *mocksender.MockSender) map[errorMetric]float64 {
	t.Helper()
	got := map[errorMetric]float64{}
	for _, call := range s.Calls {
		if call.Method != "GaugeWithTimestamp" && call.Method != "CountWithTimestamp" {
			continue
		}
		name := call.Arguments.String(0)
		if !strings.HasPrefix(name, "gpu.errors.amdgpu") {
			continue
		}
		tags := call.Arguments.Get(3).([]string)
		m := errorMetric{name: name}
		for _, tag := range tags {
			if v, ok := strings.CutPrefix(tag, "type:"); ok {
				m.errType = v
			}
			if v, ok := strings.CutPrefix(tag, "origin:"); ok {
				m.origin = v
			}
		}
		assert.Contains(t, tags, "gpu_uuid:"+testAMDUUID, name)
		got[m] = call.Arguments.Get(1).(float64)
	}
	return got
}

func setupErrorsCheck(t *testing.T, sysRoot string) (*Check, *mocksender.MockSender, *fakeEventSource) {
	check, sender := setupAMDCheck(t, sysRoot, map[string]any{"gpu.static_metrics_reporting_interval": 0})
	events := &fakeEventSource{}
	check.errors.events = events
	return check, sender, events
}

func TestAMDErrorsFromKFDEvents(t *testing.T) {
	check, sender, events := setupErrorsCheck(t, fakeAMDHost(t))

	require.NoError(t, check.Run())
	assert.Empty(t, emittedErrors(t, sender), "no error type is reported before one is seen")

	events.queue(
		amdgpu.Event{DeviceUUID: testAMDUUID, Type: amdgpu.EventVMFault, Message: "44535:python3"},
		amdgpu.Event{DeviceUUID: testAMDUUID, Type: amdgpu.EventVMFault, Message: "51000:other"},
		amdgpu.Event{DeviceUUID: testAMDUUID, Type: amdgpu.EventGPUPreReset, Message: "5 1"},
		amdgpu.Event{DeviceUUID: testAMDUUID, Type: amdgpu.EventGPUPostReset, Message: "5"},
		amdgpu.Event{DeviceUUID: "amd-other", Type: amdgpu.EventVMFault, Message: "1:x"}, // unknown device
	)
	sender.ResetCalls()
	require.NoError(t, check.Run())
	assert.Equal(t, map[errorMetric]float64{
		{"gpu.errors.amdgpu", "vm_fault", "driver"}:        2,
		{"gpu.errors.amdgpu.total", "vm_fault", "driver"}:  2,
		{"gpu.errors.amdgpu", "gpu_reset", "driver"}:       1,
		{"gpu.errors.amdgpu.total", "gpu_reset", "driver"}: 1,
	}, emittedErrors(t, sender))

	// Types seen before keep being reported, with a zero interval count.
	events.queue(amdgpu.Event{DeviceUUID: testAMDUUID, Type: amdgpu.EventThermalThrottle, Message: "3:0"})
	sender.ResetCalls()
	require.NoError(t, check.Run())
	assert.Equal(t, map[errorMetric]float64{
		{"gpu.errors.amdgpu", "vm_fault", "driver"}:                 0,
		{"gpu.errors.amdgpu.total", "vm_fault", "driver"}:           2,
		{"gpu.errors.amdgpu", "gpu_reset", "driver"}:                0,
		{"gpu.errors.amdgpu.total", "gpu_reset", "driver"}:          1,
		{"gpu.errors.amdgpu", "thermal_throttle", "hardware"}:       1,
		{"gpu.errors.amdgpu.total", "thermal_throttle", "hardware"}: 1,
	}, emittedErrors(t, sender))
}

func TestAMDErrorsFromRASCounters(t *testing.T) {
	root := fakeAMDHost(t)
	rasDir := filepath.Join(root, "bus", "pci", "devices", "0000:c1:00.0", "ras")
	require.NoError(t, os.MkdirAll(rasDir, 0o755))
	writeRAS := func(ue, ce, fatal int) {
		require.NoError(t, os.WriteFile(filepath.Join(rasDir, "umc_err_count"),
			fmt.Appendf(nil, "ue: %d\nce: %d\nde: 0\n", ue, ce), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(rasDir, "event_state"),
			fmt.Appendf(nil, "current seqno: 0\nFatal Error: count:%d, last_seqno:0\nPoison Creation: count:0, last_seqno:0\nPoison Consumption: count:0, last_seqno:0\n", fatal), 0o644))
	}
	writeRAS(4, 10, 0) // errors before the Agent started are not counted
	check, sender, _ := setupErrorsCheck(t, root)

	require.NoError(t, check.Run())
	assert.Empty(t, emittedErrors(t, sender))

	writeRAS(5, 13, 1)
	sender.ResetCalls()
	require.NoError(t, check.Run())
	assert.Equal(t, map[errorMetric]float64{
		{"gpu.errors.amdgpu", "ras_uncorrectable", "hardware"}:       1,
		{"gpu.errors.amdgpu.total", "ras_uncorrectable", "hardware"}: 1,
		{"gpu.errors.amdgpu", "ras_correctable", "hardware"}:         3,
		{"gpu.errors.amdgpu.total", "ras_correctable", "hardware"}:   3,
		{"gpu.errors.amdgpu", "ras_fatal", "hardware"}:               1,
		{"gpu.errors.amdgpu.total", "ras_fatal", "hardware"}:         1,
	}, emittedErrors(t, sender))

	// Counters going down (driver reloaded) set a new baseline.
	writeRAS(0, 0, 0)
	sender.ResetCalls()
	require.NoError(t, check.Run())
	writeRAS(2, 0, 0)
	sender.ResetCalls()
	require.NoError(t, check.Run())
	got := emittedErrors(t, sender)
	assert.Equal(t, float64(2), got[errorMetric{"gpu.errors.amdgpu", "ras_uncorrectable", "hardware"}])
	assert.Equal(t, float64(3), got[errorMetric{"gpu.errors.amdgpu.total", "ras_uncorrectable", "hardware"}])
}

func TestAMDErrorsUnavailableEventsDoNotFailTheCheck(t *testing.T) {
	check, _, events := setupErrorsCheck(t, fakeAMDHost(t))
	events.syncErr = errors.New("open /dev/kfd: permission denied")
	require.NoError(t, check.Run())
	require.NoError(t, check.Run())
	assert.Len(t, events.synced, 2, "every run retries the subscription")
	assert.Equal(t, []string{testAMDUUID}, events.synced[0])
}

func TestAMDErrorsCancelClosesEvents(t *testing.T) {
	check, _, events := setupErrorsCheck(t, fakeAMDHost(t))
	check.Cancel()
	assert.True(t, events.closed)
}

func TestAMDErrorTagsMatchTagSpec(t *testing.T) {
	tagsSpec, err := gpuspec.LoadTagsSpec()
	require.NoError(t, err)
	types := []errorType{errVMFault, errGPUReset, errThermalThrottle, errRASUncorrected, errRASCorrected,
		errRASDeferred, errRASFatal, errPoisonCreated, errPoisonConsumed}
	for _, et := range types {
		assert.Regexp(t, tagsSpec.Tags["type"].Regex, et.name)
		assert.Regexp(t, tagsSpec.Tags["origin"].Regex, et.origin)
	}

	metricsSpec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)
	for _, name := range []string{errorsMetricName, errorsTotalMetricName} {
		spec, ok := metricsSpec.Metrics[name]
		require.True(t, ok, "%s is not in the metric spec", name)
		assert.True(t, slices.Contains(spec.CustomTags, "type"))
	}
}
