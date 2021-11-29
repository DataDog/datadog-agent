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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Latest Lambda pricing per https://aws.amazon.com/lambda/pricing/
	baseLambdaInvocationPrice = 0.0000002
	lambdaPricePerGbSecond    = 0.0000166667
	msToSec                   = 0.001

	// Enhanced metrics
	maxMemoryUsedMetric   = "aws.lambda.enhanced.max_memory_used"
	memorySizeMetric      = "aws.lambda.enhanced.memorysize"
	runtimeDurationMetric = "aws.lambda.enhanced.runtime_duration"
	billedDurationMetric  = "aws.lambda.enhanced.billed_duration"
	durationMetric        = "aws.lambda.enhanced.duration"
	estimatedCostMetric   = "aws.lambda.enhanced.estimated_cost"
	initDurationMetric    = "aws.lambda.enhanced.init_duration"
	// OutOfMemoryMetric is the name of the out of memory enhanced Lambda metric
	OutOfMemoryMetric = "aws.lambda.enhanced.out_of_memory"
	timeoutsMetric    = "aws.lambda.enhanced.timeouts"
	errorsMetric      = "aws.lambda.enhanced.errors"
	invocationsMetric = "aws.lambda.enhanced.invocations"
)

func getOutOfMemorySubstrings() []string {
	return []string{
		"fatal error: runtime: out of memory",       // Go
		"java.lang.OutOfMemoryError",                // Java
		"JavaScript heap out of memory",             // Node
		"Runtime exited with error: signal: killed", // Node
		"MemoryError", // Python
		"failed to allocate memory (NoMemoryError)", // Ruby
		"OutOfMemoryException",                      // .NET
	}
}

// GenerateRuntimeDurationMetric generates the runtime duration metric
func GenerateRuntimeDurationMetric(start time.Time, end time.Time, status string, tags []string, metricsChan chan []metrics.MetricSample) {
	// first check if both date are set
	if start.IsZero() || end.IsZero() {
		log.Debug("Impossible to compute aws.lambda.enhanced.runtime_duration due to an invalid interval")
	} else {
		duration := end.Sub(start).Milliseconds()
		metricsChan <- []metrics.MetricSample{{
			Name:       runtimeDurationMetric,
			Value:      float64(duration),
			Mtype:      metrics.DistributionType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  float64(end.UnixNano()),
		}}
	}
}

// GenerateEnhancedMetricsFromFunctionLog generates enhanced metrics from a LogTypeFunction message
func GenerateEnhancedMetricsFromFunctionLog(logString string, time time.Time, tags []string, metricsChan chan []metrics.MetricSample) {
	for _, substring := range getOutOfMemorySubstrings() {
		if strings.Contains(logString, substring) {
			SendOutOfMemoryEnhancedMetric(tags, time, metricsChan)
			SendErrorsEnhancedMetric(tags, time, metricsChan)
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
		Name:       maxMemoryUsedMetric,
		Value:      float64(maxMemoryUsedMb),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       memorySizeMetric,
		Value:      memorySize,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       billedDurationMetric,
		Value:      billedDuration * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       durationMetric,
		Value:      durationMs * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}, {
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(billedDuration, memorySize),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	}}
	if initDurationMs > 0 {
		initDurationMetric := metrics.MetricSample{
			Name:       initDurationMetric,
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

// SendOutOfMemoryEnhancedMetric sends an enhanced metric representing a function running out of memory at a given time
func SendOutOfMemoryEnhancedMetric(tags []string, time time.Time, metricsChan chan []metrics.MetricSample) {
	incrementEnhancedMetric(OutOfMemoryMetric, tags, float64(time.UnixNano()), metricsChan)
}

// SendErrorsEnhancedMetric sends an enhanced metric representing an error at a given time
func SendErrorsEnhancedMetric(tags []string, time time.Time, metricsChan chan []metrics.MetricSample) {
	incrementEnhancedMetric(errorsMetric, tags, float64(time.UnixNano()), metricsChan)
}

// SendTimeoutEnhancedMetric sends an enhanced metric representing a timeout at the current time
func SendTimeoutEnhancedMetric(tags []string, metricsChan chan []metrics.MetricSample) {
	incrementEnhancedMetric(timeoutsMetric, tags, float64(time.Now().UnixNano()), metricsChan)
}

// SendInvocationEnhancedMetric sends an enhanced metric representing an invocation at the current time
func SendInvocationEnhancedMetric(tags []string, metricsChan chan []metrics.MetricSample) {
	incrementEnhancedMetric(invocationsMetric, tags, float64(time.Now().UnixNano()), metricsChan)
}

// incrementEnhancedMetric sends an enhanced metric with a value of 1 to the metrics channel
func incrementEnhancedMetric(name string, tags []string, timestamp float64, metricsChan chan []metrics.MetricSample) {
	metricsChan <- []metrics.MetricSample{{
		Name:       name,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
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
