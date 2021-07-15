// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedMetricsFromFunctionLogOutOfMemory(t *testing.T) {
	outOfMemoryLog := aws.LogMessage{
		Type:         aws.LogTypeFunction,
		StringRecord: "JavaScript heap out of memory",
		Time:         time.Now(),
	}
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}

	go generateEnhancedMetricsFromFunctionLog(outOfMemoryLog, tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.out_of_memory",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(outOfMemoryLog.Time.UnixNano()),
	}})
}

func TestGenerateEnhancedMetricsFromFunctionLogNoMetric(t *testing.T) {
	outOfMemoryLog := aws.LogMessage{
		Type:         aws.LogTypeFunction,
		StringRecord: "Task timed out after 30.03 seconds",
		Time:         time.Now(),
	}
	metricsChan := make(chan []metrics.MetricSample, 1)
	tags := []string{"functionname:test-function"}

	go generateEnhancedMetricsFromFunctionLog(outOfMemoryLog, tags, metricsChan)

	assert.Equal(t, len(metricsChan), 0)
}

func TestGenerateEnhancedMetricsFromReportLogColdStart(t *testing.T) {
	reportLog := aws.LogMessage{
		Type: aws.LogTypePlatformReport,
		Time: time.Now(),
		ObjectRecord: aws.PlatformObjectRecord{
			Metrics: aws.ReportLogMetrics{
				DurationMs:       1000.0,
				BilledDurationMs: 800.0,
				MemorySizeMB:     1024.0,
				MaxMemoryUsedMB:  256.0,
				InitDurationMs:   100.0,
			},
		},
	}
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}

	go generateEnhancedMetricsFromReportLog(reportLog, tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.max_memory_used",
		Value:      256.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.memorysize",
		Value:      1024.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.billed_duration",
		Value:      0.80,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.duration",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.estimated_cost",
		Value:      calculateEstimatedCost(800.0, 1024.0),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.init_duration",
		Value:      0.1,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}})
}

func TestGenerateEnhancedMetricsFromReportLogNoColdStart(t *testing.T) {
	reportLog := aws.LogMessage{
		Type: aws.LogTypePlatformReport,
		Time: time.Now(),
		ObjectRecord: aws.PlatformObjectRecord{
			Metrics: aws.ReportLogMetrics{
				DurationMs:       1000.0,
				BilledDurationMs: 800.0,
				MemorySizeMB:     1024.0,
				MaxMemoryUsedMB:  256.0,
				InitDurationMs:   0,
			},
		},
	}
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}

	go generateEnhancedMetricsFromReportLog(reportLog, tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.max_memory_used",
		Value:      256.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.memorysize",
		Value:      1024.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.billed_duration",
		Value:      0.80,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.duration",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.estimated_cost",
		Value:      calculateEstimatedCost(800.0, 1024.0),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLog.Time.UnixNano()),
	}})
}

func TestSendTimeoutEnhancedMetric(t *testing.T) {
	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}

	go sendTimeoutEnhancedMetric(tags, metricsChan)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.timeouts",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		// compare the generated timestamp to itself because we can't know its value
		Timestamp: generatedMetrics[0].Timestamp,
	}})
}

func TestGetTagsForEnhancedMetrics(t *testing.T) {
	aws.SetARN("arn:aws:lambda:us-east-1:123456789012:function:my-function:7")
	defer aws.SetARN("")

	generatedTags := getTagsForEnhancedMetrics()

	assert.Equal(t, generatedTags, []string{
		"region:us-east-1",
		"aws_account:123456789012",
		"account_id:123456789012",
		"functionname:my-function",
		"function_arn:arn:aws:lambda:us-east-1:123456789012:function:my-function",
		"executedversion:7",
		"resource:my-function:7",
		"cold_start:false",
	})
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
