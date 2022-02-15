// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedMetricsFromFunctionLogOutOfMemory(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()
	go GenerateEnhancedMetricsFromFunctionLog("JavaScript heap out of memory", reportLogTime, tags, metricsChan)

	generatedMetrics := <-metricsChan
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       OutOfMemoryMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}})
	generatedMetrics = <-metricsChan
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       errorsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}})
}

func TestGenerateEnhancedMetricsFromFunctionLogNoMetric(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample, 1)
	tags := []string{"functionname:test-function"}

	go GenerateEnhancedMetricsFromFunctionLog("Task timed out after 30.03 seconds", time.Now(), tags, metricsChan)

	assert.Equal(t, len(metricsChan), 0)
}

func TestGenerateEnhancedMetricsFromReportLogColdStart(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()
	go GenerateEnhancedMetricsFromReportLog(100.0, 1000.0, 800.0, 1024.0, 256.0, reportLogTime, tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       maxMemoryUsedMetric,
		Value:      256.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       memorySizeMetric,
		Value:      1024.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       billedDurationMetric,
		Value:      0.80,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       durationMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(800.0, 1024.0),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       initDurationMetric,
		Value:      0.1,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}})
}

func TestGenerateEnhancedMetricsFromReportLogNoColdStart(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()

	go GenerateEnhancedMetricsFromReportLog(0, 1000.0, 800.0, 1024.0, 256.0, reportLogTime, tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       maxMemoryUsedMetric,
		Value:      256.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       memorySizeMetric,
		Value:      1024.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       billedDurationMetric,
		Value:      0.80,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       durationMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}, {
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(800.0, 1024.0),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()),
	}})
}

func TestSendTimeoutEnhancedMetric(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}

	go SendTimeoutEnhancedMetric(tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       timeoutsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		// compare the generated timestamp to itself because we can't know its value
		Timestamp: generatedMetrics[0].Timestamp,
	}})
}

func TestSendInvocationEnhancedMetric(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}

	go SendInvocationEnhancedMetric(tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       invocationsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		// compare the generated timestamp to itself because we can't know its value
		Timestamp: generatedMetrics[0].Timestamp,
	}})
}

func TestSendOutOfMemoryEnhancedMetric(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	mockTime := time.Now()
	go SendOutOfMemoryEnhancedMetric(tags, mockTime, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       OutOfMemoryMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(mockTime.UnixNano()),
	}})
}

func TestSendErrorsEnhancedMetric(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	mockTime := time.Now()
	go SendErrorsEnhancedMetric(tags, mockTime, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       errorsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(mockTime.UnixNano()),
	}})
}

func TestCalculateEstimatedCost(t *testing.T) {
	// Latest Lambda pricing and billing examples from https://aws.amazon.com/lambda/pricing/
	const freeTierComputeCost = lambdaPricePerGbSecond * 400000
	const freeTierRequestCost = baseLambdaInvocationPrice * 1000000
	const freeTierCostAdjustment = freeTierComputeCost + freeTierRequestCost

	// Example 1: If you allocated 512MB of memory to your function, executed it 3 million times in one month,
	// and it ran for 1 second each time, your charges would be $18.74
	estimatedCost := 3000000.0 * calculateEstimatedCost(1000.0, 512.0)
	assert.InDelta(t, 18.74, estimatedCost-freeTierCostAdjustment, 0.01)

	// Example 2: If you allocated 128MB of memory to your function, executed it 30 million times in one month,
	// and it ran for 200ms each time, your charges would be $11.63
	estimatedCost = 30000000.0 * calculateEstimatedCost(200.0, 128.0)
	assert.InDelta(t, 11.63, estimatedCost-freeTierCostAdjustment, 0.01)
}

func TestGenerateRuntimeDurationMetricNoStartDate(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	startTime := time.Time{}
	endTime := time.Now()
	go GenerateRuntimeDurationMetric(startTime, endTime, "myStatus", tags, metricsChan)
	select {
	case <-metricsChan:
		assert.Fail(t, "This should not happen since the channel should be empty")
	default:
		// nothing to do here
	}
}

func TestGenerateRuntimeDurationMetricNoEndDate(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	startTime := time.Now()
	endTime := time.Time{}
	go GenerateRuntimeDurationMetric(startTime, endTime, "myStatus", tags, metricsChan)
	select {
	case <-metricsChan:
		assert.Fail(t, "This should not happen since the channel should be empty")
	default:
		// nothing to do here
	}
}

func TestGenerateRuntimeDurationMetricOK(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	startTime := time.Date(2020, 01, 01, 01, 01, 01, 500000000, time.UTC)
	endTime := time.Date(2020, 01, 01, 01, 01, 01, 653000000, time.UTC) //153 ms later
	go GenerateRuntimeDurationMetric(startTime, endTime, "myStatus", tags, metricsChan)
	generatedMetrics := <-metricsChan
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       runtimeDurationMetric,
		Value:      153,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()),
	}})

}
