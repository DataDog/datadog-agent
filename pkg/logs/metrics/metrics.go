// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides telemetry metrics for the logs agent
package metrics

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// LogsExpvars contains metrics for the logs agent.
	LogsExpvars *expvar.Map
	// LogsDecoded is the total number of decoded logs
	LogsDecoded = expvar.Int{}
	// TlmLogsDecoded is the total number of decoded logs
	TlmLogsDecoded = telemetry.NewCounter("logs", "decoded",
		nil, "Total number of decoded logs")
	// LogsProcessed is the total number of processed logs.
	LogsProcessed = expvar.Int{}
	// TlmLogsProcessed is the total number of processed logs.
	TlmLogsProcessed = telemetry.NewCounter("logs", "processed",
		nil, "Total number of processed logs")

	// LogsSent is the total number of sent logs.
	LogsSent = expvar.Int{}
	// TlmLogsSent is the total number of sent logs.
	TlmLogsSent = telemetry.NewCounter("logs", "sent",
		nil, "Total number of sent logs")
	// DestinationErrors is the total number of network errors.
	DestinationErrors = expvar.Int{}
	// TlmDestinationErrors is the total number of network errors.
	TlmDestinationErrors = telemetry.NewCounter("logs", "network_errors",
		nil, "Total number of network errors")
	// DestinationLogsDropped is the total number of logs dropped per Destination
	DestinationLogsDropped = expvar.Map{}
	// TlmLogsDropped is the total number of logs dropped per Destination
	TlmLogsDropped = telemetry.NewCounter("logs", "dropped",
		[]string{"destination"}, "Total number of logs dropped per Destination")
	// BytesSent is the total number of sent bytes before encoding if any
	BytesSent = expvar.Int{}
	// TlmBytesSent is the total number of sent bytes before encoding if any
	TlmBytesSent = telemetry.NewCounter("logs", "bytes_sent",
		[]string{"source"}, "Total number of bytes sent before encoding if any")
	// RetryCount is the total number of times we have retried payloads that failed to send
	RetryCount = expvar.Int{}
	// TlmRetryCount is the total number of times we have retried payloads that failed to send
	TlmRetryCount = telemetry.NewCounter("logs", "retry_count",
		nil, "Total number of retried payloads")
	// RetryTimeSpent is the total time spent retrying payloads that failed to send
	RetryTimeSpent = expvar.Int{}
	// EncodedBytesSent is the total number of sent bytes after encoding if any
	EncodedBytesSent = expvar.Int{}
	// TlmEncodedBytesSent is the total number of sent bytes after encoding if any
	TlmEncodedBytesSent = telemetry.NewCounter("logs", "encoded_bytes_sent",
		[]string{"source", "compression_kind"}, "Total number of sent bytes after encoding if any")
	// BytesMissed is the number of bytes lost before they could be consumed by the agent, such as after a log rotation
	BytesMissed = expvar.Int{}
	// TlmBytesMissed is the number of bytes lost before they could be consumed by the agent, such as after log rotation
	TlmBytesMissed = telemetry.NewCounter("logs", "bytes_missed",
		nil, "Total number of bytes lost before they could be consumed by the agent, such as after log rotation")
	// SenderLatency the last reported latency value from the http sender (ms)
	SenderLatency = expvar.Int{}
	// TlmSenderLatency a histogram of http sender latency (ms)
	TlmSenderLatency = telemetry.NewHistogram("logs", "sender_latency",
		nil, "Histogram of http sender latency in ms", []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000})
	// DestinationExpVars a map of sender utilization metrics for each http destination
	DestinationExpVars = expvar.Map{}
	// DestinationHTTPRespByStatusAndURL tracks HTTP responses by status code and destination URL
	DestinationHTTPRespByStatusAndURL = expvar.Map{}
	// TlmDestinationHTTPRespByStatusAndURL tracks HTTP responses by status code and destination URL
	TlmDestinationHTTPRespByStatusAndURL = telemetry.NewCounter("logs", "destination_http_resp", []string{"status_code", "url"}, "Count of http responses by status code and destination url")

	// TlmAutoMultilineAggregatorFlush Count of each line flushed from the auto multiline aggregator.
	TlmAutoMultilineAggregatorFlush = telemetry.NewCounter("logs", "auto_multi_line_aggregator_flush", []string{"truncated", "line_type"}, "Count of each line flushed from the auto multiline aggregator")

	// TlmAutoMultilineJSONAggregatorFlush Count of each line flushed from the auto multiline JSON aggregator.
	TlmAutoMultilineJSONAggregatorFlush = telemetry.NewCounter("logs", "auto_multi_line_json_aggregator_flush", []string{"is_valid"}, "Count of each line flushed from the auto multiline JSON aggregator")

	// TlmUtilizationRatio is the utilization ratio of a component.
	// Utilization ratio is calculated as the ratio of time spent in use to the total time.
	// This metric is internally sampled and exposed as an ewma in order to produce a useable value.
	TlmUtilizationRatio = telemetry.NewGauge("logs_component_utilization", "ratio", []string{"name", "instance"}, "Gauge of the utilization ratio of a component")
	// TlmUtilizationItems is the capacity of a component by number of elements
	// Both the number of items and the number of bytes are aggregated and exposed as a ewma.
	TlmUtilizationItems = telemetry.NewGauge("logs_component_utilization", "items", []string{"name", "instance"}, "Gauge of the number of items currently held in a component and its buffers")
	// TlmUtilizationBytes is the capacity of a component by number of bytes
	TlmUtilizationBytes = telemetry.NewGauge("logs_component_utilization", "bytes", []string{"name", "instance"}, "Gauge of the number of bytes currently held in a component and its buffers")
	// TlmDestNumWorkers is the number of destination workers in use.
	TlmDestNumWorkers = telemetry.NewGauge("logs_destination", "destination_workers", []string{"instance"}, "Gauge of the number of destination workers in use")
	// TlmDestVirtualLatency is a moving average of the destination's latency.
	TlmDestVirtualLatency = telemetry.NewGauge("logs_destination", "virtual_latency", []string{"instance"}, "Gauge of the destination's average latency")
	// TlmDestWorkerResets tracks the count of times the destination worker pool resets the worker count after encountering a retryable error.
	TlmDestWorkerResets = telemetry.NewCounter("logs_destination", "destination_worker_resets", []string{"instance"}, "Count of times the destination worker pool resets the worker count")
	// LogsTruncated is the number of logs truncated by the Agent
	LogsTruncated = expvar.Int{}
	// TlmTruncatedCount tracks the count of times a log is truncated
	TlmTruncatedCount = telemetry.NewCounter("logs", "truncated", []string{"service", "source"}, "Count the number of times a log is truncated")
)

func init() {
	LogsExpvars = expvar.NewMap("logs-agent")
	LogsExpvars.Set("LogsDecoded", &LogsDecoded)
	LogsExpvars.Set("LogsProcessed", &LogsProcessed)
	LogsExpvars.Set("LogsSent", &LogsSent)
	LogsExpvars.Set("DestinationErrors", &DestinationErrors)
	LogsExpvars.Set("DestinationLogsDropped", &DestinationLogsDropped)
	LogsExpvars.Set("BytesSent", &BytesSent)
	LogsExpvars.Set("RetryCount", &RetryCount)
	LogsExpvars.Set("RetryTimeSpent", &RetryTimeSpent)
	LogsExpvars.Set("EncodedBytesSent", &EncodedBytesSent)
	LogsExpvars.Set("BytesMissed", &BytesMissed)
	LogsExpvars.Set("SenderLatency", &SenderLatency)
	LogsExpvars.Set("HttpDestinationStats", &DestinationExpVars)
	LogsExpvars.Set("LogsTruncated", &LogsTruncated)
}
