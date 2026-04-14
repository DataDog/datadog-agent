// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides telemetry metrics for the logs agent
//
// gRPC sender parity tracker
// ===========================
// This table tracks which metrics are emitted on the gRPC sending path.
// "Shared" means the metric is set in a component common to all pipelines (e.g. processor, tailer).
//
// Metric                               | gRPC status    | Notes
// -------------------------------------|----------------|----------------------------------------------
// LogsDecoded / TlmLogsDecoded         | SHARED         | Set in processor.go for all pipelines
// LogsProcessed / TlmLogsProcessed     | SHARED         | Set in processor.go for all pipelines
// LogsSent / TlmLogsSent               | DONE           | gRPC: stream_worker.go handleBatchAck; HTTP: destination.go, sync_destination.go; TCP: destination.go
// DestinationErrors / TlmDestErrors    | DONE           | gRPC: stream_worker.go signalStreamFailure; HTTP: destination.go, sync_destination.go; TCP: destination.go
// DestinationLogsDropped / TlmDropped  | DONE           | gRPC: stream_worker.go handleBatchAck (auditor full); HTTP: destination.go; TCP: destination.go
// BytesSent / TlmBytesSent             | DONE           | gRPC: stream_worker.go handleBatchAck; HTTP: destination.go; TCP: destination.go
// RetryCount / TlmRetryCount           | TODO           | HTTP only: destination.go
// RetryTimeSpent                       | TODO           | HTTP only: destination.go
// EncodedBytesSent / TlmEncBytesSent   | DONE           | gRPC: stream_worker.go handleBatchAck; HTTP: destination.go; TCP: destination.go
// BytesMissed / TlmBytesMissed         | SHARED         | Set in file tailer, not sender-specific
// SenderLatency / TlmSenderLatency     | TODO           | HTTP only: destination.go
// LogsTruncated / TlmTruncatedCount    | SHARED         | Set in processor.go and decoder handlers
// DestHTTPResp / TlmDestHTTPResp       | N/A            | HTTP-specific, no gRPC equivalent needed
// TlmUtilizationRatio/Items/Bytes      | SHARED         | Set via CapacityMonitor / UtilizationMonitor
// TlmDestNumWorkers                    | N/A            | HTTP worker pool specific
// TlmDestVirtualLatency                | N/A            | HTTP worker pool specific
// TlmDestWorkerResets                  | N/A            | HTTP worker pool specific
// TlmHTTPConnectivityCheck             | N/A            | HTTP restart/connectivity logic in agentimpl
// TlmHTTPConnectivityRetryAttempt      | N/A            | HTTP restart/connectivity logic in agentimpl
// TlmRestartAttempt                    | N/A            | HTTP restart/connectivity logic in agentimpl
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
	// The remote_agent tag identifies which agent sent the logs. Use GetAgentIdentityTag()
	// to get the correct value for the current agent. This tag is used by COAT to partition
	// log bytes by agent type.
	TlmBytesSent = telemetry.NewCounter("logs", "bytes_sent",
		[]string{"remote_agent", "source"}, "Total number of bytes sent before encoding if any")
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
	// The remote_agent tag identifies which agent sent the logs. Use GetAgentIdentityTag()
	// to get the correct value for the current agent. This tag is used by COAT to partition
	// encoded log bytes by agent type.
	TlmEncodedBytesSent = telemetry.NewCounter("logs", "encoded_bytes_sent",
		[]string{"remote_agent", "source", "compression_kind"}, "Total number of sent bytes after encoding if any")
	// PreCompressionBytesSent is the total number of gRPC bytes before compression and after protobuf serialization
	PreCompressionBytesSent = expvar.Int{}
	// TlmPreCompressionBytesSent is the total number of gRPC bytes before compression and after protobuf serialization
	TlmPreCompressionBytesSent = telemetry.NewCounter("logs", "pre_compression_bytes_sent",
		[]string{"source"}, "Total number of gRPC bytes before compression and after protobuf serialization")
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

	// TlmLogLineSizes is a distribution of post-framer log line sizes
	TlmLogLineSizes = telemetry.NewHistogram("logs", "log_line_sizes",
		nil, "Distribution of post-framer log line sizes before line parsers/handlers are applied", []float64{32, 128, 512, 2048, 8192, 32768, 131072, 524288, 2097152})

	// TlmRotationsNix tracks file rotations detected on *nix platforms by rotation type (new_file vs truncated)
	TlmRotationsNix = telemetry.NewCounter("logs", "rotations_nix",
		[]string{"rotation_type"}, "Count of file rotations detected on *nix platforms, tagged by rotation_type (new_file or truncated)")

	// TlmRotationSizeMismatch counts disagreements between cache-growth and offset-unread rotation detectors.
	// The `detector` tag indicates which heuristic detected a potential rotation (not which claimed all was fine):
	// - detector:cache = cache observed growth but offset indicates all data was read (likely missed rotation)
	// - detector:offset = offset indicates unread data but cache saw no growth (likely false-positive rotation)
	TlmRotationSizeMismatch = telemetry.NewCounter("logs", "rotation_size_mismatch",
		[]string{"detector"}, "Count of disagreements between cache-growth and offset-unread rotation detectors")

	// TlmRotationSizeDifferences records the absolute file size difference whenever the file size changes between checks
	TlmRotationSizeDifferences = telemetry.NewHistogram("logs", "rotation_size_differences",
		nil, "Distribution of absolute file size differences observed between consecutive file rotation checks", []float64{256, 1024, 4096, 16384, 65536, 262144, 1048576, 10485760, 104857600})

	// TlmHTTPConnectivityCheck tracks HTTP connectivity check results
	// Tags: status (success/failure)
	TlmHTTPConnectivityCheck = telemetry.NewCounter("logs", "http_connectivity_check",
		[]string{"status"}, "Count of HTTP connectivity checks with status")

	// TlmHTTPConnectivityRetryAttempt tracks HTTP connectivity retry attempts
	// Tags: status (success/failure)
	TlmHTTPConnectivityRetryAttempt = telemetry.NewCounter("logs", "http_connectivity_retry_attempt",
		[]string{"status"}, "Count of HTTP connectivity retry attempts with success/failure status")

	// TlmRestartAttempt tracks logs agent restart attempts
	// Tags: status (success/failure/timeout), transport (tcp/http)
	TlmRestartAttempt = telemetry.NewCounter("logs", "restart_attempt",
		[]string{"status", "transport"}, "Count of logs agent restart attempts with status and target transport")

	// COAT telemetry for auto multiline default-on impact analysis.
	TlmAutoMultilineTotalLines = telemetry.NewCounter("logs", "auto_multi_line_default_total_lines",
		nil, "Total lines processed by the detecting aggregator for default-path sources")
	TlmAutoMultilineWouldCombine = telemetry.NewCounter("logs", "auto_multi_line_default_would_combine",
		nil, "Lines that would be combined if auto multiline were the default")
	TlmAutoMultilineWouldTruncate = telemetry.NewCounter("logs", "auto_multi_line_default_would_truncate",
		nil, "Lines belonging to groups that would be truncated if auto multiline were the default")

	// TlmDatumCount tracks per-datum-type counts for stateful (gRPC) encoding.
	TlmDatumCount = telemetry.NewCounter("logs", "datum_count",
		[]string{"datum_type"}, "Per-datum-type count for stateful encoding")

	// TlmDatumBytes tracks per-datum-type proto size for stateful (gRPC) encoding.
	TlmDatumBytes = telemetry.NewCounter("logs", "datum_bytes",
		[]string{"datum_type"}, "Per-datum-type proto.Size bytes for stateful encoding")
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
	LogsExpvars.Set("PreCompressionBytesSent", &PreCompressionBytesSent)
	LogsExpvars.Set("BytesMissed", &BytesMissed)
	LogsExpvars.Set("SenderLatency", &SenderLatency)
	LogsExpvars.Set("HttpDestinationStats", &DestinationExpVars)
	LogsExpvars.Set("LogsTruncated", &LogsTruncated)
}

// agentIdentityTag holds the remote_agent tag value for this agent process.
// It must be set once at startup via SetAgentIdentity before any log sending occurs.
//
// This mirrors the pattern used by pkg/util/flavor (SetFlavor/GetFlavor) rather than
// importing it directly, because importing flavor would pull in pkg/config/model and
// pkg/config/setup, significantly widening the dependency graph for the 40+ files that
// import pkg/logs/metrics.
var agentIdentityTag = "agent"

// SetAgentIdentity sets the remote_agent tag value for the current agent process.
// This must be called once during agent startup, before any logs are sent.
// Example values: "agent", "system-probe", "trace-agent", etc.
func SetAgentIdentity(tag string) {
	agentIdentityTag = tag
}

// GetAgentIdentityTag returns the remote_agent tag value for the current agent process.
// The value is set at startup via SetAgentIdentity and defaults to "agent".
func GetAgentIdentityTag() string {
	return agentIdentityTag
}
