// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package telemetry

import (
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	mockMapName    = "mock_map"
	mockHelperName = "mock_helper"
)

type mockErrorsTelemetry struct {
	ebpfErrorsTelemetry
	mtx          sync.Mutex
	mapErrMap    map[telemetryIndex]MapErrTelemetry
	helperErrMap map[telemetryIndex]HelperErrTelemetry
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

func (m *mockErrorsTelemetry) forEachMapEntry(yield func(telemetryIndex, MapErrTelemetry) bool) {
	for i, telemetry := range m.mapErrMap {
		if !yield(i, telemetry) {
			return
		}
	}
}

func (m *mockErrorsTelemetry) forEachHelperEntry(yield func(telemetryIndex, HelperErrTelemetry) bool) {
	for i, telemetry := range m.helperErrMap {
		if !yield(i, telemetry) {
			return
		}
	}
}

// creates an error collector and replaces the telemetry field with a mock
func createTestCollector(telemetry ebpfErrorsTelemetry) prometheus.Collector {
	collector := NewEBPFErrorsCollector().(*EBPFErrorsCollector)
	if collector != nil {
		collector.T = telemetry
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
	mapEntries := map[telemetryIndex]MapErrTelemetry{
		{key: 1, name: mockMapName}: {Count: [64]uint64{mapErrorsMockValue}},
	}
	helperEntries := map[telemetryIndex]HelperErrTelemetry{
		{key: 2, name: mockHelperName}: {Count: [320]uint64{helperErrorsMockValue}},
	}

	//check expected metrics and labels
	expectedMetrics := []struct {
		value  float64
		labels []string
	}{
		{value: float64(mapErrorsMockValue), labels: []string{"errno 0", mockMapName}},
		{value: float64(helperErrorsMockValue), labels: []string{"errno 0", "bpf_probe_read", mockHelperName}},
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
	for i, promMetric := range metrics {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())

		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())
		assert.Equal(t, expectedMetrics[i].value, dtoMetric.GetCounter().GetValue(),
			"expected metric value %v, but got %v", expectedMetrics[i].value, dtoMetric.GetCounter().GetValue())
		for j, label := range dtoMetric.GetLabel() {
			assert.Equal(t, expectedMetrics[i].labels[j], label.GetValue(),
				"expected label %v, but got %v", expectedMetrics[i].labels[j], label.GetValue())
		}
	}
}

// TestEBPFErrorsCollector_DoubleCollect tests the case when the collector is called twice to validate the delta calculation of the Counter metric
func TestEBPFErrorsCollector_DoubleCollect(t *testing.T) {
	//skip this test on unsupported kernel versions
	if ok, _ := ebpfTelemetrySupported(); !ok {
		t.SkipNow()
	}
	mapErrorsMockValue, helperErrorsMockValue := uint64(20), uint64(100)
	deltaMapErrors, deltaHelperErrors := float64(80), float64(900)
	mapEntries := map[telemetryIndex]MapErrTelemetry{
		{key: 1, name: mockMapName}: {Count: [64]uint64{mapErrorsMockValue}},
	}
	helperEntries := map[telemetryIndex]HelperErrTelemetry{
		{key: 2, name: mockHelperName}: {Count: [320]uint64{helperErrorsMockValue}},
	}

	//in this test we expect the values to match the delta between second and first collects
	expectedMetrics := []struct {
		value  float64
		labels []string
	}{
		{value: deltaMapErrors, labels: []string{"errno 0", mockMapName}},
		{value: deltaHelperErrors, labels: []string{"errno 0", "bpf_probe_read", mockHelperName}},
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
	collector.(*EBPFErrorsCollector).T = &mockErrorsTelemetry{
		mapErrMap: map[telemetryIndex]MapErrTelemetry{
			{key: 1, name: mockMapName}: {Count: [64]uint64{mapErrorsMockValue + uint64(deltaMapErrors)}}},
		helperErrMap: map[telemetryIndex]HelperErrTelemetry{
			{key: 2, name: mockHelperName}: {Count: [320]uint64{helperErrorsMockValue + uint64(deltaHelperErrors)}}},
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

	for i, promMetric := range metrics {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())

		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())
		assert.Equal(t, expectedMetrics[i].value, dtoMetric.GetCounter().GetValue(),
			"expected metric value %v, but got %v", expectedMetrics[i].value, dtoMetric.GetCounter().GetValue())
	}
}
