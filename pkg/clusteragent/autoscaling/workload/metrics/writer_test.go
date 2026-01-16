// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestNewSenderMetricsWriter(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)
	mockSender := mocksender.NewMockSender("")
	isLeader := func() bool { return true }

	writer := NewSenderMetricsWriter(store, mockSender, isLeader)

	assert.NotNil(t, writer)
	assert.NotNil(t, writer.store)
	assert.NotNil(t, writer.sender)
	assert.NotNil(t, writer.isLeader)
}

func TestWriteAll_Leader(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.gauge", Type: MetricTypeGauge, Value: 42.0, Tags: []string{"tag1:value1"}},
			{Name: "test.count", Type: MetricTypeCount, Value: 10.0, Tags: []string{"tag2:value2"}},
		}
	})

	// Add some metrics to the store
	store.Add("key1", &PodAutoscalerMetricsObject{})

	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", "test.gauge", 42.0, "", []string{"tag1:value1"}).Return()
	mockSender.On("Count", "test.count", 10.0, "", []string{"tag2:value2"}).Return()
	mockSender.On("Commit").Return()

	isLeader := func() bool { return true }
	writer := NewSenderMetricsWriter(store, mockSender, isLeader)

	err := writer.WriteAll()
	assert.NoError(t, err)

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Count", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWriteAll_NonLeader(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.gauge", Type: MetricTypeGauge, Value: 42.0, Tags: []string{"tag1:value1"}},
		}
	})

	store.Add("key1", &PodAutoscalerMetricsObject{})

	mockSender := mocksender.NewMockSender("")
	// Should not call any sender methods when not leader

	isLeader := func() bool { return false }
	writer := NewSenderMetricsWriter(store, mockSender, isLeader)

	err := writer.WriteAll()
	assert.NoError(t, err)

	// Assert no calls were made
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 0)
}

func TestWriteAll_EmptyStore(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)

	mockSender := mocksender.NewMockSender("")
	mockSender.On("Commit").Return()

	isLeader := func() bool { return true }
	writer := NewSenderMetricsWriter(store, mockSender, isLeader)

	err := writer.WriteAll()
	assert.NoError(t, err)

	// Should still commit even with no metrics
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWriteAll_MultipleMetrics(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric1", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
			{Name: "test.metric2", Type: MetricTypeGauge, Value: 2.0, Tags: []string{"test:tag"}},
			{Name: "test.metric3", Type: MetricTypeCount, Value: 3.0, Tags: []string{"test:tag"}},
		}
	})

	store.Add("key1", &PodAutoscalerMetricsObject{})
	store.Add("key2", &PodAutoscalerMetricsObject{})

	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	isLeader := func() bool { return true }
	writer := NewSenderMetricsWriter(store, mockSender, isLeader)

	err := writer.WriteAll()
	assert.NoError(t, err)

	// Should have 2 keys * 2 gauges each = 4 gauge calls
	// Should have 2 keys * 1 count each = 2 count calls
	mockSender.AssertNumberOfCalls(t, "Gauge", 4)
	mockSender.AssertNumberOfCalls(t, "Count", 2)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWriteAllPeriodically_CancelsOnContext(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	})

	store.Add("key1", &PodAutoscalerMetricsObject{})

	mockSender := mocksender.NewMockSender("")
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	isLeader := func() bool { return true }
	writer := NewSenderMetricsWriter(store, mockSender, isLeader)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the periodic writer
	done := make(chan bool)
	go func() {
		writer.WriteAllPeriodically(ctx, 100*time.Millisecond)
		done <- true
	}()

	// Wait a bit to let it run at least once
	time.Sleep(150 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for goroutine to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("WriteAllPeriodically did not stop after context cancellation")
	}

	// Should have been called at least once (immediately on start)
	assert.GreaterOrEqual(t, len(mockSender.Calls), 1)
}
