package serverless

import (
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
)

// Latest Lambda pricing per https://aws.amazon.com/lambda/pricing/
const (
	baseLambdaInvocationPrice = 0.0000002
	lambdaPricePerGbSecond    = 0.0000166667
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

// generateEnhancedMetricsFromRegularLog generates enhanced metrics from a LogTypeFunction message
func generateEnhancedMetricsFromFunctionLog(message aws.LogMessage, tags []string, metricsChan chan []metrics.MetricSample) {
	logString := message.StringRecord
	for _, substring := range getOutOfMemorySubstrings() {
		if strings.Contains(logString, substring) {
			metricsChan <- []metrics.MetricSample{{
				Name:       "aws.lambda.enhanced.out_of_memory",
				Value:      1.0,
				Mtype:      metrics.DistributionType,
				Tags:       tags,
				SampleRate: 1,
				Timestamp:  float64(message.Time.UnixNano()),
			}}
			return
		}
	}
}

// generateEnhancedMetricsFromReportLog generates enhanced metrics from a LogTypePlatformReport log message
func generateEnhancedMetricsFromReportLog(message aws.LogMessage, tags []string, metricsChan chan []metrics.MetricSample) {
	memorySizeMb := float64(message.ObjectRecord.Metrics.MemorySizeMB)
	billedDurationMs := float64(message.ObjectRecord.Metrics.BilledDurationMs)

	metricsChan <- []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.max_memory_used",
		Value:      float64(message.ObjectRecord.Metrics.MaxMemoryUsedMB),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(message.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.memorysize",
		Value:      memorySizeMb,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(message.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.billed_duration",
		Value:      billedDurationMs,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(message.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.duration",
		Value:      message.ObjectRecord.Metrics.DurationMs,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(message.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.init_duration",
		Value:      message.ObjectRecord.Metrics.InitDurationMs,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(message.Time.UnixNano()),
	}, {
		Name:       "aws.lambda.enhanced.estimated_cost",
		Value:      calculateEstimatedCost(billedDurationMs, memorySizeMb),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(message.Time.UnixNano()),
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
