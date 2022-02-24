// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"math"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
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
	// ErrorsMetric is the name of the errors enhanced Lambda metric
	ErrorsMetric      = "aws.lambda.enhanced.errors"
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
func GenerateRuntimeDurationMetric(start time.Time, end time.Time, status string, tags []string, demux aggregator.Demultiplexer) {
	// first check if both date are set
	if start.IsZero() || end.IsZero() {
		log.Debug("Impossible to compute aws.lambda.enhanced.runtime_duration due to an invalid interval")
	} else {
		duration := end.Sub(start).Milliseconds()
		demux.AddTimeSample(metrics.MetricSample{
			Name:       runtimeDurationMetric,
			Value:      float64(duration),
			Mtype:      metrics.DistributionType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  float64(end.UnixNano()) / float64(time.Second),
		})
	}
}

// GenerateEnhancedMetricsFromFunctionLog generates enhanced metrics from a LogTypeFunction message
func GenerateEnhancedMetricsFromFunctionLog(logString string, time time.Time, tags []string, demux aggregator.Demultiplexer) {
	for _, substring := range getOutOfMemorySubstrings() {
		if strings.Contains(logString, substring) {
			SendOutOfMemoryEnhancedMetric(tags, time, demux)
			SendErrorsEnhancedMetric(tags, time, demux)
			return
		}
	}
}

// GenerateEnhancedMetricsFromReportLog generates enhanced metrics from a LogTypePlatformReport log message
func GenerateEnhancedMetricsFromReportLog(initDurationMs float64, durationMs float64, billedDurationMs int, memorySizeMb int, maxMemoryUsedMb int, t time.Time, tags []string, demux aggregator.Demultiplexer) {
	timestamp := float64(t.UnixNano()) / float64(time.Second)
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

	for _, metric := range enhancedMetrics {
		demux.AddTimeSample(metric)
	}
}

// SendOutOfMemoryEnhancedMetric sends an enhanced metric representing a function running out of memory at a given time
func SendOutOfMemoryEnhancedMetric(tags []string, t time.Time, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(OutOfMemoryMetric, tags, float64(t.UnixNano())/float64(time.Second), demux)
}

// SendErrorsEnhancedMetric sends an enhanced metric representing an error at a given time
func SendErrorsEnhancedMetric(tags []string, t time.Time, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(ErrorsMetric, tags, float64(t.UnixNano())/float64(time.Second), demux)
}

// SendTimeoutEnhancedMetric sends an enhanced metric representing a timeout at the current time
func SendTimeoutEnhancedMetric(tags []string, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(timeoutsMetric, tags, float64(time.Now().UnixNano())/float64(time.Second), demux)
}

// SendInvocationEnhancedMetric sends an enhanced metric representing an invocation at the current time
func SendInvocationEnhancedMetric(tags []string, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(invocationsMetric, tags, float64(time.Now().UnixNano())/float64(time.Second), demux)
}

// incrementEnhancedMetric sends an enhanced metric with a value of 1 to the metrics channel
func incrementEnhancedMetric(name string, tags []string, timestamp float64, demux aggregator.Demultiplexer) {
	demux.AddTimeSample(metrics.MetricSample{
		Name:       name,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
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
