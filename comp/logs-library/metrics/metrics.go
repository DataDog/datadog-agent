// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides telemetry metrics for the logs agent
package metrics

import (
	"expvar"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

var (
	// LogsExpvars contains metrics for the logs agent.
	LogsExpvars *expvar.Map
	// LogsDecoded is the total number of decoded logs
	LogsDecoded = expvar.Int{}
	// TlmLogsDecoded is the total number of decoded logs
	TlmLogsDecoded = telemetryimpl.GetCompatComponent().NewCounter("logs", "decoded",
		nil, "Total number of decoded logs")
	// LogsProcessed is the total number of processed logs.
	LogsProcessed = expvar.Int{}
	// TlmLogsProcessed is the total number of processed logs.
	TlmLogsProcessed = telemetryimpl.GetCompatComponent().NewCounter("logs", "processed",
		nil, "Total number of processed logs")

	// LogsSent is the total number of sent logs.
	LogsSent = expvar.Int{}
	// TlmLogsSent is the total number of sent logs.
	TlmLogsSent = telemetryimpl.GetCompatComponent().NewCounter("logs", "sent",
		nil, "Total number of sent logs")
	// DestinationErrors is the total number of network errors.
	DestinationErrors = expvar.Int{}
	// TlmDestinationErrors is the total number of network errors.
	TlmDestinationErrors = telemetryimpl.GetCompatComponent().NewCounter("logs", "network_errors",
		nil, "Total number of network errors")
	// DestinationLogsDropped is the total number of logs dropped per Destination
	DestinationLogsDropped = expvar.Map{}
	// TlmLogsDropped is the total number of logs dropped per Destination
	TlmLogsDropped = telemetryimpl.GetCompatComponent().NewCounter("logs", "dropped",
		[]string{"destination"}, "Total number of logs dropped per Destination")
	// BytesSent is the total number of sent bytes before encoding if any
	BytesSent = expvar.Int{}
	// TlmBytesSent is the total number of sent bytes before encoding if any
	// The emitter tag identifies which agent sent the logs (e.g. "agent", "system-probe").
	// Use GetAgentIdentityTag() to get the correct value for the current agent.
	TlmBytesSent = telemetryimpl.GetCompatComponent().NewCounter("logs", "bytes_sent",
		[]string{"emitter", "source"}, "Total number of bytes sent before encoding if any")
	// RetryCount is the total number of times we have retried payloads that failed to send
	RetryCount = expvar.Int{}
	// TlmRetryCount is the total number of times we have retried payloads that failed to send
	TlmRetryCount = telemetryimpl.GetCompatComponent().NewCounter("logs", "retry_count",
		nil, "Total number of retried payloads")
	// RetryTimeSpent is the total time spent retrying payloads that failed to send
	RetryTimeSpent = expvar.Int{}
	// EncodedBytesSent is the total number of sent bytes after encoding if any
	EncodedBytesSent = expvar.Int{}
	// TlmEncodedBytesSent is the total number of sent bytes after encoding if any
	// The emitter tag identifies which agent sent the logs (e.g. "agent", "system-probe").
	// Use GetAgentIdentityTag() to get the correct value for the current agent.
	TlmEncodedBytesSent = telemetryimpl.GetCompatComponent().NewCounter("logs", "encoded_bytes_sent",
		[]string{"emitter", "source", "compression_kind"}, "Total number of sent bytes after encoding if any")
	// BytesMissed is the number of bytes lost before they could be consumed by the agent, such as after a log rotation
	BytesMissed = expvar.Int{}
	// TlmBytesMissed is the number of bytes lost before they could be consumed by the agent, such as after log rotation
	TlmBytesMissed = telemetryimpl.GetCompatComponent().NewCounter("logs", "bytes_missed",
		nil, "Total number of bytes lost before they could be consumed by the agent, such as after log rotation")
	// SenderLatency the last reported latency value from the http sender (ms)
	SenderLatency = expvar.Int{}
	// TlmSenderLatency a histogram of http sender latency (ms)
	TlmSenderLatency = telemetryimpl.GetCompatComponent().NewHistogram("logs", "sender_latency",
		nil, "Histogram of http sender latency in ms", []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000})
	// DestinationExpVars a map of sender utilization metrics for each http destination
	DestinationExpVars = expvar.Map{}
	// DestinationHTTPRespByStatusAndURL tracks HTTP responses by status code and destination URL
	DestinationHTTPRespByStatusAndURL = expvar.Map{}
	// TlmDestinationHTTPRespByStatusAndURL tracks HTTP responses by status code and destination URL
	TlmDestinationHTTPRespByStatusAndURL = telemetryimpl.GetCompatComponent().NewCounter("logs", "destination_http_resp", []string{"status_code", "url"}, "Count of http responses by status code and destination url")

	// TlmAutoMultilineAggregatorFlush Count of each line flushed from the auto multiline aggregator.
	TlmAutoMultilineAggregatorFlush = telemetryimpl.GetCompatComponent().NewCounter("logs", "auto_multi_line_aggregator_flush", []string{"truncated", "line_type"}, "Count of each line flushed from the auto multiline aggregator")

	// TlmAutoMultilineJSONAggregatorFlush Count of each line flushed from the auto multiline JSON aggregator.
	TlmAutoMultilineJSONAggregatorFlush = telemetryimpl.GetCompatComponent().NewCounter("logs", "auto_multi_line_json_aggregator_flush", []string{"is_valid"}, "Count of each line flushed from the auto multiline JSON aggregator")

	// TlmAutoMultilineStackTraceAggregatorFlush counts Go stack trace aggregation outcomes.
	TlmAutoMultilineStackTraceAggregatorFlush = telemetryimpl.GetCompatComponent().NewCounter("logs", "auto_multi_line_go_stack_trace_aggregator_flush", []string{"result"}, "Count of Go stack traces flushed from the stack trace aggregator")

	// TlmUtilizationRatio is the N=15 EWMA utilization ratio of a component (~15s window).
	TlmUtilizationRatio = telemetryimpl.GetCompatComponent().NewGauge("logs_component_utilization", "ratio", []string{"name", "instance"}, "Gauge of the utilization ratio of a component (N=15 EWMA, ~15s window)")
	// TlmUtilizationItems is the EWMA item count held in a component and its buffers.
	TlmUtilizationItems = telemetryimpl.GetCompatComponent().NewGauge("logs_component_utilization", "items", []string{"name", "instance"}, "Gauge of the number of items currently held in a component and its buffers (N=15 EWMA, ~15s window)")
	// TlmUtilizationBytes is the EWMA byte count held in a component and its buffers.
	TlmUtilizationBytes = telemetryimpl.GetCompatComponent().NewGauge("logs_component_utilization", "bytes", []string{"name", "instance"}, "Gauge of the number of bytes currently held in a component and its buffers (N=15 EWMA, ~15s window)")
	// TlmDestNumWorkers is the number of destination workers in use.
	TlmDestNumWorkers = telemetryimpl.GetCompatComponent().NewGauge("logs_destination", "destination_workers", []string{"instance"}, "Gauge of the number of destination workers in use")
	// TlmDestVirtualLatency is a moving average of the destination's latency.
	TlmDestVirtualLatency = telemetryimpl.GetCompatComponent().NewGauge("logs_destination", "virtual_latency", []string{"instance"}, "Gauge of the destination's average latency")
	// TlmDestWorkerResets tracks the count of times the destination worker pool resets the worker count after encountering a retryable error.
	TlmDestWorkerResets = telemetryimpl.GetCompatComponent().NewCounter("logs_destination", "destination_worker_resets", []string{"instance"}, "Count of times the destination worker pool resets the worker count")
	// LogsTruncated is the number of logs truncated by the Agent
	LogsTruncated = expvar.Int{}
	// TlmTruncatedCount tracks the count of times a log is truncated
	TlmTruncatedCount = telemetryimpl.GetCompatComponent().NewCounter("logs", "truncated", []string{"service", "source"}, "Count the number of times a log is truncated")

	// TlmLogLineSizes is a distribution of post-framer log line sizes
	TlmLogLineSizes = telemetryimpl.GetCompatComponent().NewHistogram("logs", "log_line_sizes",
		nil, "Distribution of post-framer log line sizes before line parsers/handlers are applied", []float64{32, 128, 512, 2048, 8192, 32768, 131072, 524288, 2097152})

	// TlmCharacterizationIngressEvents counts messages observed at pipeline ingress.
	TlmCharacterizationIngressEvents = telemetryimpl.GetCompatComponent().NewCounter("logs_characterization", "ingress_events",
		[]string{"source_type", "pipeline", "has_service", "has_source"}, "Messages observed at Logs pipeline ingress")
	// TlmCharacterizationIngressBytes counts content bytes observed at pipeline ingress.
	TlmCharacterizationIngressBytes = telemetryimpl.GetCompatComponent().NewCounter("logs_characterization", "ingress_bytes",
		[]string{"source_type", "pipeline"}, "Content bytes observed at Logs pipeline ingress")
	// TlmCharacterizationPayloadFamilyEvents counts allowlisted structural payload families.
	TlmCharacterizationPayloadFamilyEvents = telemetryimpl.GetCompatComponent().NewCounter("logs_characterization", "payload_family_events",
		[]string{"payload_family", "pipeline"}, "Messages by bounded structural payload-family classification")
	// TlmCharacterizationPayloadFamilyBytes counts content bytes by allowlisted structural payload family.
	TlmCharacterizationPayloadFamilyBytes = telemetryimpl.GetCompatComponent().NewCounter("logs_characterization", "payload_family_bytes",
		[]string{"payload_family", "pipeline"}, "Content bytes by bounded structural payload-family classification")
	// TlmCharacterizationMessageSizes records content size at pipeline ingress.
	TlmCharacterizationMessageSizes = telemetryimpl.GetCompatComponent().NewHistogram("logs_characterization", "message_sizes",
		[]string{"source_type", "pipeline"}, "Distribution of message content sizes at Logs pipeline ingress", []float64{32, 128, 512, 2048, 8192, 32768, 131072, 524288, 2097152})
	// TlmCharacterizationRawSizes records pre-transformation source byte size at pipeline ingress.
	TlmCharacterizationRawSizes = telemetryimpl.GetCompatComponent().NewHistogram("logs_characterization", "raw_sizes",
		[]string{"source_type", "pipeline"}, "Distribution of raw source byte sizes at Logs pipeline ingress", []float64{32, 128, 512, 2048, 8192, 32768, 131072, 524288, 2097152})
	// TlmCharacterizationInterarrivalSeconds records bounded aggregate gaps between ingress messages.
	TlmCharacterizationInterarrivalSeconds = telemetryimpl.GetCompatComponent().NewHistogram("logs_characterization", "interarrival_seconds",
		[]string{"source_type", "pipeline"}, "Distribution of elapsed seconds between Logs pipeline ingress messages", []float64{0.000001, 0.00001, 0.0001, 0.001, 0.01, 0.1, 1, 10})
	// TlmCharacterizationTagCounts records aggregate tag counts without retaining tag values.
	TlmCharacterizationTagCounts = telemetryimpl.GetCompatComponent().NewHistogram("logs_characterization", "tag_counts",
		[]string{"source_type", "pipeline"}, "Distribution of tag counts at Logs pipeline ingress", []float64{0, 1, 2, 4, 8, 16, 32, 64, 128})
	// TlmCharacterizationTagBytes records aggregate tag bytes without retaining tag values.
	TlmCharacterizationTagBytes = telemetryimpl.GetCompatComponent().NewHistogram("logs_characterization", "tag_bytes",
		[]string{"source_type", "pipeline"}, "Distribution of tag metadata byte sizes at Logs pipeline ingress", []float64{0, 32, 128, 512, 2048, 8192, 32768})
	// TlmCharacterizationObserverDrops counts observations discarded when the bounded queue is full.
	TlmCharacterizationObserverDrops = telemetryimpl.GetCompatComponent().NewCounter("logs_characterization", "observer_drops",
		[]string{"pipeline"}, "Characterization observations dropped because the bounded queue was full")
	// TlmCharacterizationObserverStartTime records when each pipeline-local observer began collecting.
	TlmCharacterizationObserverStartTime = telemetryimpl.GetCompatComponent().NewGauge("logs_characterization", "observer_start_time_seconds",
		[]string{"pipeline"}, "Unix time when characterization began for a Logs pipeline")
	// TlmCharacterizationSourceCardinality records bounded source cardinality without exporting source identifiers.
	TlmCharacterizationSourceCardinality = telemetryimpl.GetCompatComponent().NewGauge("logs_characterization", "source_cardinality",
		[]string{"source_type", "pipeline"}, "Distinct source identities observed at Logs pipeline ingress")
	// TlmCharacterizationSourceIdentityMissing counts messages for which no source identity was available.
	TlmCharacterizationSourceIdentityMissing = telemetryimpl.GetCompatComponent().NewCounter("logs_characterization", "source_identity_missing",
		[]string{"source_type", "pipeline"}, "Messages without a source identity at Logs pipeline ingress")
	// TlmCharacterizationSourceCardinalitySaturated reports that the bounded identity set reached capacity.
	TlmCharacterizationSourceCardinalitySaturated = telemetryimpl.GetCompatComponent().NewGauge("logs_characterization", "source_cardinality_saturated",
		[]string{"source_type", "pipeline"}, "Whether the bounded source-cardinality set reached capacity")

	// TlmSyslogMalformedBytes is the total number of malformed bytes seen by the syslog
	// framer (bytes that do not form a valid RFC 6587 frame).
	TlmSyslogMalformedBytes = telemetryimpl.GetCompatComponent().NewCounter("logs", "syslog_malformed_bytes",
		nil, "Total number of malformed bytes seen by the syslog framer")

	// TlmSyslogOversizedFrames counts syslog frames split because they exceeded the
	// content length limit. Syslog lines are conventionally far below the limit, so a
	// non-zero value signals a severe upstream framing error.
	TlmSyslogOversizedFrames = telemetryimpl.GetCompatComponent().NewCounter("logs", "syslog_oversized_frames",
		nil, "Count of syslog frames split because they exceeded the content length limit")

	// TlmRotationsNix tracks file rotations detected on *nix platforms by rotation type (new_file vs truncated)
	TlmRotationsNix = telemetryimpl.GetCompatComponent().NewCounter("logs", "rotations_nix",
		[]string{"rotation_type"}, "Count of file rotations detected on *nix platforms, tagged by rotation_type (new_file or truncated)")

	// TlmRotationSizeMismatch counts disagreements between cache-growth and offset-unread rotation detectors.
	// The `detector` tag indicates which heuristic detected a potential rotation (not which claimed all was fine):
	// - detector:cache = cache observed growth but offset indicates all data was read (likely missed rotation)
	// - detector:offset = offset indicates unread data but cache saw no growth (likely false-positive rotation)
	TlmRotationSizeMismatch = telemetryimpl.GetCompatComponent().NewCounter("logs", "rotation_size_mismatch",
		[]string{"detector"}, "Count of disagreements between cache-growth and offset-unread rotation detectors")

	// TlmRotationSizeDifferences records the absolute file size difference whenever the file size changes between checks
	TlmRotationSizeDifferences = telemetryimpl.GetCompatComponent().NewHistogram("logs", "rotation_size_differences",
		nil, "Distribution of absolute file size differences observed between consecutive file rotation checks", []float64{256, 1024, 4096, 16384, 65536, 262144, 1048576, 10485760, 104857600})

	// TlmPayloadFlushed is the total number of payloads flushed by the batch strategy.
	// Tags: pipeline, flush_reason (max_count, max_bytes, timer, flush, shutdown)
	TlmPayloadFlushed = telemetryimpl.GetCompatComponent().NewCounter("logs", "batch_payload_flushed",
		[]string{"pipeline", "flush_reason"}, "Total number of payloads flushed, tagged by the reason the flush was triggered")

	// TlmHTTPConnectivityCheck tracks HTTP connectivity check results
	// Tags: status (success/failure)
	TlmHTTPConnectivityCheck = telemetryimpl.GetCompatComponent().NewCounter("logs", "http_connectivity_check",
		[]string{"status"}, "Count of HTTP connectivity checks with status")

	// TlmHTTPConnectivityFailure tracks HTTP connectivity check failures broken down by root cause.
	// Tags: failure_cause — use the FailureCause* constants defined below.
	TlmHTTPConnectivityFailure = telemetryimpl.GetCompatComponent().NewCounter("logs", "http_connectivity_failure",
		[]string{"failure_cause"}, "Count of HTTP connectivity check failures broken down by root cause")

	// TlmHTTPConnectivityRetryAttempt tracks HTTP connectivity retry attempts
	// Tags: status (success/failure)
	TlmHTTPConnectivityRetryAttempt = telemetryimpl.GetCompatComponent().NewCounter("logs", "http_connectivity_retry_attempt",
		[]string{"status"}, "Count of HTTP connectivity retry attempts with success/failure status")

	// TlmRestartAttempt tracks logs agent restart attempts
	// Tags: status (success/failure/timeout), transport (tcp/http)
	TlmRestartAttempt = telemetryimpl.GetCompatComponent().NewCounter("logs", "restart_attempt",
		[]string{"status", "transport"}, "Count of logs agent restart attempts with status and target transport")

	// COAT telemetry for auto multiline default-on impact analysis.
	// These counters only increment for sources that rely on the default value of
	// auto_multi_line_detection (i.e. sources where changing the default would alter behavior).

	// TlmAutoMultilineTotalLines counts all lines processed by the detecting aggregator
	// for sources on the default path. Used as the denominator for both X% and Y% metrics.
	TlmAutoMultilineTotalLines = telemetryimpl.GetCompatComponent().NewCounter("logs", "auto_multi_line_default_total_lines",
		nil, "Total lines processed by the detecting aggregator for default-path sources")

	// TlmAutoMultilineWouldCombine counts lines that would be merged into a preceding
	// startGroup message if auto multiline were enabled by default.
	TlmAutoMultilineWouldCombine = telemetryimpl.GetCompatComponent().NewCounter("logs", "auto_multi_line_default_would_combine",
		nil, "Lines that would be combined if auto multiline were the default")

	// TlmAutoMultilineWouldTruncate counts raw input lines belonging to multiline
	// groups that would exceed maxContentSize due to combining. Single lines that are
	// individually oversized are excluded (they'd be truncated regardless).
	TlmAutoMultilineWouldTruncate = telemetryimpl.GetCompatComponent().NewCounter("logs", "auto_multi_line_default_would_truncate",
		nil, "Lines belonging to groups that would be truncated if auto multiline were the default")

	// TlmListenerIPDenied counts connections or datagrams rejected by IP allow/deny filters.
	// Tags: listener_type (tcp, udp)
	TlmListenerIPDenied = telemetryimpl.GetCompatComponent().NewCounter("logs", "listener_ip_denied",
		[]string{"listener_type"}, "Count of connections or datagrams rejected by IP allow/deny filters")
)

// FailureCause* are the stable tag values for the logs.http_connectivity_failure
// counter's failure_cause tag. Dashboards and monitors filter on these literal strings,
// so they must not change across agent versions.
const (
	FailureCauseDNS        = "dns"
	FailureCauseTLS        = "tls"
	FailureCauseTimeout    = "timeout"
	FailureCauseConnection = "connection"
	FailureCauseHTTPStatus = "http_status"
	FailureCauseOther      = "other"
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

// agentIdentityTag holds the emitter tag value for this agent process.
// It must be set once at startup via SetAgentIdentity before any log sending occurs.
//
// This mirrors the pattern used by pkg/util/flavor (SetFlavor/GetFlavor) rather than
// importing it directly, because importing flavor would pull in pkg/config/model and
// pkg/config/setup, significantly widening the dependency graph for the 40+ files that
// import pkg/logs/metrics.
var agentIdentityTag = "agent"

// SetAgentIdentity sets the emitter tag value for the current agent process.
// This must be called once during agent startup, before any logs are sent.
// Example values: "agent", "system-probe", "trace-agent", etc.
func SetAgentIdentity(tag string) {
	agentIdentityTag = tag
}

// GetAgentIdentityTag returns the emitter tag value for the current agent process.
// The value is set at startup via SetAgentIdentity and defaults to "agent".
func GetAgentIdentityTag() string {
	return agentIdentityTag
}
