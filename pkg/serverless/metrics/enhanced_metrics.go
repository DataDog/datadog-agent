// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Latest Lambda pricing per https://aws.amazon.com/lambda/pricing/
	baseLambdaInvocationPrice = 0.0000002
	x86LambdaPricePerGbSecond = 0.0000166667
	armLambdaPricePerGbSecond = 0.0000133334
	msToSec                   = 0.001

	// Enhanced metrics
	maxMemoryUsedMetric       = "aws.lambda.enhanced.max_memory_used"
	memorySizeMetric          = "aws.lambda.enhanced.memorysize"
	runtimeDurationMetric     = "aws.lambda.enhanced.runtime_duration"
	billedDurationMetric      = "aws.lambda.enhanced.billed_duration"
	durationMetric            = "aws.lambda.enhanced.duration"
	postRuntimeDurationMetric = "aws.lambda.enhanced.post_runtime_duration"
	estimatedCostMetric       = "aws.lambda.enhanced.estimated_cost"
	initDurationMetric        = "aws.lambda.enhanced.init_duration"
	responseLatencyMetric     = "aws.lambda.enhanced.response_latency"
	responseDurationMetric    = "aws.lambda.enhanced.response_duration"
	producedBytesMetric       = "aws.lambda.enhanced.produced_bytes"
	// OutOfMemoryMetric is the name of the out of memory enhanced Lambda metric
	OutOfMemoryMetric = "aws.lambda.enhanced.out_of_memory"
	timeoutsMetric    = "aws.lambda.enhanced.timeouts"
	// ErrorsMetric is the name of the errors enhanced Lambda metric
	ErrorsMetric          = "aws.lambda.enhanced.errors"
	invocationsMetric     = "aws.lambda.enhanced.invocations"
	enhancedMetricsEnvVar = "DD_ENHANCED_METRICS"
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

// GenerateEnhancedMetricsFromRuntimeDoneLogArgs are the arguments required for
// the GenerateEnhancedMetricsFromRuntimeDoneLog func
type GenerateEnhancedMetricsFromRuntimeDoneLogArgs struct {
	Start            time.Time
	End              time.Time
	ResponseLatency  float64
	ResponseDuration float64
	ProducedBytes    float64
	Tags             []string
	Demux            aggregator.Demultiplexer
}

// GenerateEnhancedMetricsFromRuntimeDoneLog generates the runtime duration metric
func GenerateEnhancedMetricsFromRuntimeDoneLog(args GenerateEnhancedMetricsFromRuntimeDoneLogArgs) {
	// first check if both date are set
	if args.Start.IsZero() || args.End.IsZero() {
		log.Debug("Impossible to compute aws.lambda.enhanced.runtime_duration due to an invalid interval")
	} else {
		duration := args.End.Sub(args.Start).Milliseconds()
		args.Demux.AggregateSample(metrics.MetricSample{
			Name:       runtimeDurationMetric,
			Value:      float64(duration),
			Mtype:      metrics.DistributionType,
			Tags:       args.Tags,
			SampleRate: 1,
			Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
		})
	}
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       responseLatencyMetric,
		Value:      args.ResponseLatency,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       responseDurationMetric,
		Value:      args.ResponseDuration,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       producedBytesMetric,
		Value:      args.ProducedBytes,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
	})
}

// ContainsOutOfMemoryLog determines whether a runtime specific out of memory string is found in the log line
func ContainsOutOfMemoryLog(logString string) bool {
	for _, substring := range getOutOfMemorySubstrings() {
		if strings.Contains(logString, substring) {
			return true
		}
	}
	return false
}

// GenerateEnhancedMetricsFromFunctionLog generates enhanced metrics specific to an out of memory from a LogTypeFunction message
func GenerateEnhancedMetricsFromFunctionLog(time time.Time, tags []string, demux aggregator.Demultiplexer) {
	SendOutOfMemoryEnhancedMetric(tags, time, demux)
	SendErrorsEnhancedMetric(tags, time, demux)
}

// GenerateEnhancedMetricsFromReportLogArgs provides the arguments required for
// the GenerateEnhancedMetricsFromReportLog func
type GenerateEnhancedMetricsFromReportLogArgs struct {
	InitDurationMs   float64
	DurationMs       float64
	BilledDurationMs int
	MemorySizeMb     int
	MaxMemoryUsedMb  int
	RuntimeStart     time.Time
	RuntimeEnd       time.Time
	T                time.Time
	Tags             []string
	Demux            aggregator.Demultiplexer
}

// GenerateEnhancedMetricsFromReportLog generates enhanced metrics from a LogTypePlatformReport log message
func GenerateEnhancedMetricsFromReportLog(args GenerateEnhancedMetricsFromReportLogArgs) {
	timestamp := float64(args.T.UnixNano()) / float64(time.Second)
	billedDuration := float64(args.BilledDurationMs)
	memorySize := float64(args.MemorySizeMb)
	postRuntimeDuration := args.DurationMs - float64(args.RuntimeEnd.Sub(args.RuntimeStart).Milliseconds())
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       maxMemoryUsedMetric,
		Value:      float64(args.MaxMemoryUsedMb),
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       memorySizeMetric,
		Value:      memorySize,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       billedDurationMetric,
		Value:      billedDuration * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       durationMetric,
		Value:      args.DurationMs * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(billedDuration, memorySize, serverlessTags.ResolveRuntimeArch()),
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       postRuntimeDurationMetric,
		Value:      postRuntimeDuration,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	if args.InitDurationMs > 0 {
		args.Demux.AggregateSample(metrics.MetricSample{
			Name:       initDurationMetric,
			Value:      args.InitDurationMs * msToSec,
			Mtype:      metrics.DistributionType,
			Tags:       args.Tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		})
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
	// TODO - pass config here, instead of directly looking up var
	if strings.ToLower(os.Getenv(enhancedMetricsEnvVar)) == "false" {
		return
	}
	demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
}

// calculateEstimatedCost returns the estimated cost in USD of a Lambda invocation
func calculateEstimatedCost(billedDurationMs float64, memorySizeMb float64, architecture string) float64 {
	billedDurationSeconds := billedDurationMs / 1000.0
	memorySizeGb := memorySizeMb / 1024.0
	gbSeconds := billedDurationSeconds * memorySizeGb
	// round the final float result because float math could have float point imprecision
	// on some arch. (i.e. 1.00000000000002 values)
	return math.Round((baseLambdaInvocationPrice+(gbSeconds*getLambdaPricePerGbSecond(architecture)))*10e12) / 10e12
}

// get the lambda price per Gb second based on the runtime platform
func getLambdaPricePerGbSecond(architecture string) float64 {
	switch architecture {
	case serverlessTags.ArmLambdaPlatform:
		// for arm64
		return armLambdaPricePerGbSecond
	default:
		// for x86 and amd64
		return x86LambdaPricePerGbSecond
	}
}
