// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"math"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Latest Lambda pricing per https://aws.amazon.com/lambda/pricing/
const (
	baseLambdaInvocationPrice = 0.0000002
	lambdaPricePerGbSecond    = 0.0000166667
	msToSec                   = 0.001
)

func getOutOfMemorySubstrings() []string {
	return []string{
		"fatal error: runtime: out of memory",       // Go
		"java.lang.OutOfMemoryError",                // Java
		"JavaScript heap out of memory",             // Node
		"Runtime exited with error: signal: killed", // Node
		"MemoryError", // Python
		"failed to allocate memory (NoMemoryError)", // Ruby
	}
}

// GenerateEnhancedMetricsFromFunctionLog generates enhanced metrics from a LogTypeFunction message
func GenerateEnhancedMetricsFromFunctionLog(logString string, time time.Time, tags []string, metricsChan chan []metrics.MetricSample) {
	for _, substring := range getOutOfMemorySubstrings() {
		if strings.Contains(logString, substring) {
			metricsChan <- []metrics.MetricSample{{
				Name:       "aws.lambda.enhanced.out_of_memory",
				Value:      1.0,
				Mtype:      metrics.DistributionType,
				Tags:       tags,
				SampleRate: 1,
				Timestamp:  float64(time.UnixNano()),
			}}
			return
		}
	}
}

// GenerateEnhancedMetricsFromReportLog generates enhanced metrics from a LogTypePlatformReport log message
func GenerateEnhancedMetricsFromReportLog(initDurationMs float64, durationMs float64, billedDurationMs int, memorySizeMb int, maxMemoryUsedMb int, time time.Time, tags []string, metricsChan chan []metrics.MetricSample) {
	timestamp := float64(time.UnixNano())
	billedDuration := float64(billedDurationMs)
	memorySize := float64(memorySizeMb)
	enhancedMetrics := []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.max_memory_used",
		Value:      float64(maxMemoryUsedMb),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       "aws.lambda.enhanced.memorysize",
		Value:      memorySize,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       "aws.lambda.enhanced.billed_duration",
		Value:      billedDuration * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       "aws.lambda.enhanced.duration",
		Value:      durationMs * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       "aws.lambda.enhanced.estimated_cost",
		Value:      calculateEstimatedCost(billedDuration, memorySize),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}}
	if initDurationMs > 0 {
		initDurationMetric := metrics.MetricSample{
			Name:       "aws.lambda.enhanced.init_duration",
			Value:      initDurationMs * msToSec,
			Mtype:      metrics.DistributionType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		}
		enhancedMetrics = append(enhancedMetrics, initDurationMetric)
	}
	metricsChan <- enhancedMetrics
}

// SendTimeoutEnhancedMetric sends an enhanced metric representing a timeout
func SendTimeoutEnhancedMetric(tags []string, metricsChan chan []metrics.MetricSample) {
	metricsChan <- []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.timeouts",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(time.Now().UnixNano()),
	}}
}

// calculateEstimatedCost returns the estimated cost in USD of a Lambda invocation
func calculateEstimatedCost(billedDurationMs float64, memorySizeMb float64) float64 {
	billedDurationSeconds := billedDurationMs / 1000.0
	memorySizeGb := memorySizeMb / 1024.0
	gbSeconds := billedDurationSeconds * memorySizeGb
	// round the final float result because float math could have float point imprecision
	// on some arch. (i.e. 1.00000000000002 values)
	return math.Round((baseLambdaInvocationPrice+(gbSeconds*lambdaPricePerGbSecond))*10e12) / 10e12
}
