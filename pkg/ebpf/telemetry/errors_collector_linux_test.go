// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package telemetry

import (
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	mockMapName   = "mock_map"
	mockProbeName = "kprobe__ebpf"
)

type telemetryIndex struct {
	tKey    telemetryKey
	eBPFKey uint64
}

type MockMapName struct {
	n string
}

func (m *MockMapName) String() string {
	return m.n
}

func (m *MockMapName) Type() names.ResourceType {
	return names.MapType
}

type MockProgramName struct {
	n string
}

func (m *MockProgramName) String() string {
	return m.n
}

func (m *MockProgramName) Type() names.ResourceType {
	return names.ProgramType
}

type mockErrorsTelemetry struct {
	ebpfErrorsTelemetry
	mtx          sync.Mutex
	mapErrMap    map[telemetryIndex]mapErrTelemetry
	helperErrMap map[telemetryIndex]helperErrTelemetry
}

func (m *mockErrorsTelemetry) Lock() {
	m.mtx.Lock()
}

func (m *mockErrorsTelemetry) Unlock() {
	m.mtx.Unlock()
}

func (m *mockErrorsTelemetry) isInitialized() bool {
	return m.mapErrMap != nil && m.helperErrMap != nil
}

func (m *mockErrorsTelemetry) forEachMapErrorEntryInMaps(yield func(telemetryKey, uint64, mapErrTelemetry) bool) {
	for i, telemetry := range m.mapErrMap {
		if !yield(i.tKey, i.eBPFKey, telemetry) {
			return
		}
	}
}

func (m *mockErrorsTelemetry) forEachHelperErrorEntryInMaps(yield func(telemetryKey, uint64, helperErrTelemetry) bool) {
	for i, telemetry := range m.helperErrMap {
		if !yield(i.tKey, i.eBPFKey, telemetry) {
			return
		}
	}
}

// creates an error collector and replaces the telemetry field with a mock
func createTestCollector(telemetry ebpfErrorsTelemetry) prometheus.Collector {
	collector := NewEBPFErrorsCollector().(*EBPFErrorsCollector)
	if collector != nil {
		collector.t = telemetry
	}
	return collector
}

func TestEBPFErrorsCollector_NotInitialized(t *testing.T) {
	//skip this test on unsupported kernel versions
	if ok, _ := ebpfTelemetrySupported(); !ok {
		t.SkipNow()
	}
	telemetry := &mockErrorsTelemetry{
		mapErrMap:    nil,
		helperErrMap: nil,
	}
	collector := createTestCollector(telemetry)
	assert.NotNil(t, collector, "expected collector to be created")

	ch := make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	//we shouldn't have any metrics collected, since the mock telemetry object is not initialized
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	assert.Equal(t, 0, len(metrics), "expected %d metrics, but got %d", 0, len(metrics))
}

func TestEBPFErrorsCollector_SingleCollect(t *testing.T) {
	//skip this test on unsupported kernel versions
	if ok, _ := ebpfTelemetrySupported(); !ok {
		t.SkipNow()
	}
	mapErrorsMockValue, helperErrorsMockValue := uint64(20), uint64(100)
	//create mock telemetry objects (since we don't want to trigger full ebpf subsystem)
	mapEntries := map[telemetryIndex]mapErrTelemetry{
		{
			eBPFKey: 1,
			tKey: telemetryKey{
				resourceName: &MockMapName{n: mockMapName},
				moduleName:   names.NewModuleName("m1"),
			},
		}: {Count: [64]uint64{mapErrorsMockValue}},
		{
			eBPFKey: 2,
			tKey: telemetryKey{
				resourceName: &MockMapName{n: mockMapName},
				moduleName:   names.NewModuleName("m2"),
			},
		}: {Count: [64]uint64{mapErrorsMockValue}},
	}

	helperEntries := map[telemetryIndex]helperErrTelemetry{
		{
			eBPFKey: 2,
			tKey: telemetryKey{
				resourceName: &MockProgramName{n: mockProbeName},
				moduleName:   names.NewModuleName("m3"),
			},
		}: {Count: [320]uint64{helperErrorsMockValue}},
		{
			eBPFKey: 3,
			tKey: telemetryKey{
				resourceName: &MockProgramName{n: mockProbeName},
				moduleName:   names.NewModuleName("m4"),
			},
		}: {Count: [320]uint64{helperErrorsMockValue}},
	}

	//check expected metrics and labels
	expectedMetrics := []struct {
		value      float64
		labels     map[string]string
		discovered bool
	}{
		{value: float64(mapErrorsMockValue), labels: map[string]string{"error": "errno 0", "map_name": mockMapName, "module": "m1"}},
		{value: float64(mapErrorsMockValue), labels: map[string]string{"error": "errno 0", "map_name": mockMapName, "module": "m2"}},
		{value: float64(helperErrorsMockValue), labels: map[string]string{"error": "errno 0", "helper": "bpf_probe_read", "probe_name": mockProbeName, "module": "m3"}},
		{value: float64(helperErrorsMockValue), labels: map[string]string{"error": "errno 0", "helper": "bpf_probe_read", "probe_name": mockProbeName, "module": "m4"}},
	}

	telemetry := &mockErrorsTelemetry{
		mapErrMap:    mapEntries,
		helperErrMap: helperEntries,
	}
	//use mock telemetry instead of real ebpfTelemetry to avoid triggering eBPF APIs
	collector := createTestCollector(telemetry)
	assert.NotNil(t, collector, "expected collector to be created")

	ch := make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	assert.Equal(t, len(expectedMetrics), len(metrics), "received unexpected number of metrics")

	//parse received metrics to compare the values and labels
	for _, promMetric := range metrics {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())

		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

		for j, expected := range expectedMetrics {
			if expected.discovered {
				continue
			}
			expectedMetrics[j].discovered = true
			if expected.value != dtoMetric.GetCounter().GetValue() {
				expectedMetrics[j].discovered = false
				continue
			}

			for _, label := range dtoMetric.GetLabel() {
				if expected.labels[label.GetName()] != label.GetValue() {
					expectedMetrics[j].discovered = false
					continue
				}
			}
		}
	}

	for _, expected := range expectedMetrics {
		require.True(t, expected.discovered, "expected metric (%v %v) not found", expected.value, expected)
	}
}

// TestEBPFErrorsCollector_DoubleCollect tests the case when the collector is called twice to validate the delta calculation of the Counter metric
func TestEBPFErrorsCollector_DoubleCollect(t *testing.T) {
	//skip this test on unsupported kernel versions
	if ok, _ := ebpfTelemetrySupported(); !ok {
		t.SkipNow()
	}
	mapErrorsMockValue1, helperErrorsMockValue1 := uint64(20), uint64(100)
	mapErrorsMockValue2, helperErrorsMockValue2 := uint64(100), uint64(1000)

	mapEntries := map[telemetryIndex]mapErrTelemetry{
		{
			eBPFKey: 1,
			tKey: telemetryKey{
				resourceName: &MockMapName{n: mockMapName},
				moduleName:   names.NewModuleName("m1"),
			},
		}: {Count: [64]uint64{mapErrorsMockValue1}},
		{
			eBPFKey: 2,
			tKey: telemetryKey{
				resourceName: &MockMapName{n: mockMapName},
				moduleName:   names.NewModuleName("m2"),
			},
		}: {Count: [64]uint64{mapErrorsMockValue1}},
	}

	helperEntries := map[telemetryIndex]helperErrTelemetry{
		{
			eBPFKey: 2,
			tKey: telemetryKey{
				resourceName: &MockProgramName{n: mockProbeName},
				moduleName:   names.NewModuleName("m3"),
			},
		}: {Count: [320]uint64{helperErrorsMockValue1}},
		{
			eBPFKey: 3,
			tKey: telemetryKey{
				resourceName: &MockProgramName{n: mockProbeName},
				moduleName:   names.NewModuleName("m4"),
			},
		}: {Count: [320]uint64{helperErrorsMockValue1}},
	}

	//in this test we expect the values to match the delta between second and first collects
	expectedMetrics := []struct {
		value      float64
		labels     map[string]string
		discovered bool
	}{
		{value: float64(mapErrorsMockValue2), labels: map[string]string{"error": "errno 0", "map_name": mockMapName, "module": "m1"}},
		{value: float64(mapErrorsMockValue2), labels: map[string]string{"error": "errno 0", "map_name": mockMapName, "module": "m1"}},
		{value: float64(helperErrorsMockValue2), labels: map[string]string{"error": "errno 0", "helper": "bpf_probe_read", "probe_name": mockProbeName, "module": "m3"}},
		{value: float64(helperErrorsMockValue2), labels: map[string]string{"error": "errno 0", "helper": "bpf_probe_read", "probe_name": mockProbeName, "module": "m4"}},
	}

	telemetry := &mockErrorsTelemetry{
		mapErrMap:    mapEntries,
		helperErrMap: helperEntries,
	}
	collector := createTestCollector(telemetry)
	assert.NotNil(t, collector, "expected collector to be created")

	ch := make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	//make sure the channel is emptied and closed before we trigger the second collect
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	assert.Equal(t, len(expectedMetrics), len(metrics), "received unexpected number of metrics")

	//increase the counters of the mock telemetry object before second collect
	collector.(*EBPFErrorsCollector).t = &mockErrorsTelemetry{
		mapErrMap: map[telemetryIndex]mapErrTelemetry{
			{
				eBPFKey: 1,
				tKey: telemetryKey{
					resourceName: &MockMapName{n: mockMapName},
					moduleName:   names.NewModuleName("m1"),
				},
			}: {Count: [64]uint64{mapErrorsMockValue2}},
			{
				eBPFKey: 2,
				tKey: telemetryKey{
					resourceName: &MockMapName{n: mockMapName},
					moduleName:   names.NewModuleName("m2"),
				},
			}: {Count: [64]uint64{mapErrorsMockValue2}},
		},
		helperErrMap: map[telemetryIndex]helperErrTelemetry{
			{
				eBPFKey: 2,
				tKey: telemetryKey{
					resourceName: &MockProgramName{n: mockProbeName},
					moduleName:   names.NewModuleName("m3"),
				},
			}: {Count: [320]uint64{helperErrorsMockValue2}},
			{
				eBPFKey: 3,
				tKey: telemetryKey{
					resourceName: &MockProgramName{n: mockProbeName},
					moduleName:   names.NewModuleName("m4"),
				},
			}: {Count: [320]uint64{helperErrorsMockValue2}},
		},
	}

	ch = make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	metrics = nil
	for m := range ch {
		metrics = append(metrics, m)
	}
	assert.Equal(t, len(expectedMetrics), len(metrics), "received unexpected number of metrics")

	for _, promMetric := range metrics {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())

		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

		for j, expected := range expectedMetrics {
			if expected.discovered {
				continue
			}

			expectedMetrics[j].discovered = true
			if expected.value != dtoMetric.GetCounter().GetValue() {
				expectedMetrics[j].discovered = false
				continue
			}

			for _, label := range dtoMetric.GetLabel() {
				if expected.labels[label.GetName()] != label.GetValue() {
					expectedMetrics[j].discovered = false
					continue
				}

			}
		}
	}

	for _, expected := range expectedMetrics {
		require.True(t, expected.discovered, "expected metric (%v %v) not found", expected.value, expected)
	}
}
