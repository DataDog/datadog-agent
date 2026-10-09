// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

// LogLevel is the wire-level severity of a single log record.
type LogLevel string

// LogLevelError is the error severity accepted by the telemetry intake.
const LogLevelError LogLevel = "ERROR"

// Log is a single record in a LogsPayload. It mirrors dd-go's agent logs
// telemetry schema. All fields intentionally remain present on the wire.
type Log struct {
	Message    string   `json:"message"`
	Tags       string   `json:"tags"`
	Level      LogLevel `json:"level"`
	StackTrace string   `json:"stack_trace"`
	TracerTime int64    `json:"tracer_time"`
	Count      int      `json:"count"`
	TraceID    string   `json:"trace_id"`
	SpanID     string   `json:"span_id"`
	IsCrash    bool     `json:"is_crash"`
	ErrorKind  string   `json:"error_kind"`
}

// LogsPayload is the inner payload of an agent-logs telemetry envelope.
type LogsPayload struct {
	Logs []Log `json:"logs"`
}
