// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
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
		nil, "Total number of bytes send before encoding if any")
	// RetryCount is the total number of times we have retried payloads that failed to send
	RetryCount = expvar.Int{}
	// TlmRetryCountis the total number of times we have retried payloads that failed to send
	TlmRetryCount = telemetry.NewCounter("logs", "retry_count",
		nil, "Total number of retried paylaods")
	// RetryTimeSpent is the total time spent retrying payloads that failed to send
	RetryTimeSpent = expvar.Int{}
	// EncodedBytesSent is the total number of sent bytes after encoding if any
	EncodedBytesSent = expvar.Int{}
	// TlmEncodedBytesSent is the total number of sent bytes after encoding if any
	TlmEncodedBytesSent = telemetry.NewCounter("logs", "encoded_bytes_sent",
		nil, "Total number of sent bytes after encoding if any")
	// SenderLatency the last reported latency value from the http sender (ms)
	SenderLatency = expvar.Int{}
	// TlmSenderLatency a histogram of http sender latency (ms)
	TlmSenderLatency = telemetry.NewHistogram("logs", "sender_latency",
		nil, "Histogram of http sender latency in ms", []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000})
	// DestinationExpVars a map of sender utilization metrics for each http destination
	DestinationExpVars = expvar.Map{}
	// TODO: Add LogsCollected for the total number of collected logs.
	//nolint:revive // TODO(AML) Fix revive linter
	DestinationHttpRespByStatusAndUrl = expvar.Map{}
	//nolint:revive // TODO(AML) Fix revive linter
	TlmDestinationHttpRespByStatusAndUrl = telemetry.NewCounter("logs", "destination_http_resp", []string{"status_code", "url"}, "Count of http responses by status code and destination url")

	TlmLogsInput = telemetry.NewCounter("logs", "input", nil, "Total number of log messages read.")
	// For output see `TlmLogsSent`.
	TlmFileRead = telemetry.NewGauge("processing", "file_read", []string{"file_name"}, "File read progress.")
	TlmChanTime = telemetry.NewHistogram("processing",
		"channel_time",
		[]string{"channel"},
		"Time to send on the processing channel",
		[]float64{1000000, 2000000, 3000000, 4000000, 5000000, 6000000, 7000000, 8000000, 9000000, 10000000})

	TlmChanTimeSkew = telemetry.NewGauge("processing", "channel_time_skew", []string{"channel"}, "Skew of the processing channel")

	TlmChanLength = telemetry.NewGauge("processing", "channel_length", []string{"channel"}, "Length of the processing channel")
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
	LogsExpvars.Set("SenderLatency", &SenderLatency)
	LogsExpvars.Set("HttpDestinationStats", &DestinationExpVars)
}
