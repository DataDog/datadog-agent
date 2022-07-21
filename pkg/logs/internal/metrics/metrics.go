// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

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

)

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 65`)
	LogsExpvars = expvar.NewMap("logs-agent")
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 66`)
	LogsExpvars.Set("LogsDecoded", &LogsDecoded)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 67`)
	LogsExpvars.Set("LogsProcessed", &LogsProcessed)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 68`)
	LogsExpvars.Set("LogsSent", &LogsSent)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 69`)
	LogsExpvars.Set("DestinationErrors", &DestinationErrors)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 70`)
	LogsExpvars.Set("DestinationLogsDropped", &DestinationLogsDropped)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 71`)
	LogsExpvars.Set("BytesSent", &BytesSent)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 72`)
	LogsExpvars.Set("EncodedBytesSent", &EncodedBytesSent)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 73`)
	LogsExpvars.Set("SenderLatency", &SenderLatency)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 74`)
	LogsExpvars.Set("HttpDestinationStats", &DestinationExpVars)
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\logs\internal\metrics\metrics.go 75`)
}
