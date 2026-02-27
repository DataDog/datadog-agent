// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metricsstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestWriteAll_Leader(t *testing.T) {
	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", "test.gauge", 42.0, "", []string{"tag1:value1"}).Return()
	mockSender.On("Count", "test.count", 10.0, "", []string{"tag2:value2"}).Return()
	mockSender.On("Commit").Return()

	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.gauge", Type: MetricTypeGauge, Value: 42.0, Tags: []string{"tag1:value1"}},
			{Name: "test.count", Type: MetricTypeCount, Value: 10.0, Tags: []string{"tag2:value2"}},
		}
	}, mockSender, func() bool { return true }, nil)

	store.Add("key1", nil)

	err := store.WriteAll()
	assert.NoError(t, err)

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Count", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWriteAll_NonLeader(t *testing.T) {
	mockSender := mocksender.NewMockSender("")

	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.gauge", Type: MetricTypeGauge, Value: 42.0, Tags: []string{"tag1:value1"}},
		}
	}, mockSender, func() bool { return false }, nil)

	store.Add("key1", nil)

	err := store.WriteAll()
	assert.NoError(t, err)

	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 0)
}

func TestWriteAll_EmptyStore(t *testing.T) {
	mockSender := mocksender.NewMockSender("")
	mockSender.On("Commit").Return()

	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, mockSender, func() bool { return true }, nil)

	err := store.WriteAll()
	assert.NoError(t, err)

	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWriteAll_MultipleMetrics(t *testing.T) {
	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric1", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
			{Name: "test.metric2", Type: MetricTypeGauge, Value: 2.0, Tags: []string{"test:tag"}},
			{Name: "test.metric3", Type: MetricTypeCount, Value: 3.0, Tags: []string{"test:tag"}},
		}
	}, mockSender, func() bool { return true }, nil)

	store.Add("key1", nil)
	store.Add("key2", nil)

	err := store.WriteAll()
	assert.NoError(t, err)

	// Should have 2 keys * 2 gauges each = 4 gauge calls
	// Should have 2 keys * 1 count each = 2 count calls
	mockSender.AssertNumberOfCalls(t, "Gauge", 4)
	mockSender.AssertNumberOfCalls(t, "Count", 2)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWriteAll_GlobalTagsAppended(t *testing.T) {
	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", "test.gauge", 1.0, "", []string{"metric:tag", "global:tag"}).Return()
	mockSender.On("Commit").Return()

	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.gauge", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"metric:tag"}},
		}
	}, mockSender, func() bool { return true }, func() []string { return []string{"global:tag"} })

	store.Add("key1", nil)

	err := store.WriteAll()
	assert.NoError(t, err)

	mockSender.AssertExpectations(t)
}

func TestWriteAllPeriodically_CancelsOnContext(t *testing.T) {
	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	}, mockSender, func() bool { return true }, nil)

	store.Add("key1", nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		store.WriteAllPeriodically(ctx, 100*time.Millisecond)
		done <- true
	}()

	// Wait a bit to let it run at least once
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("WriteAllPeriodically did not stop after context cancellation")
	}

	assert.GreaterOrEqual(t, len(mockSender.Calls), 1)
}
