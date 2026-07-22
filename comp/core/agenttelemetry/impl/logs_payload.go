// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

// This file mirrors dd-go's logs telemetry payload at
// trace/apps/tracer-telemetry-intake/telemetry-payload/logs.go (pinned
// reference: ref=7957be33). The receiver-side processor expects these
// exact JSON keys and an UPPERCASE LogLevel; deviation will silently fail
// validation in dd-go's Validate() chain.

// LogLevel is the wire-level severity of a single log record. The
// dd-go intake accepts "ERROR", "WARN", "DEBUG" (and "INFO" when the
// envelope request_type is "debug-logs"), but the errortracking path
// only ever emits LogLevelError; the other constants were dropped as
// dead code per iglendd's "why bother with anything but ERRORS"
// thread on PR #50607.
type LogLevel string

// LogLevel value emitted on the wire by the errortracking path.
const (
	LogLevelError LogLevel = "ERROR"
)

// Log mirrors dd-go's tracer-telemetry-intake/telemetry-payload/logs.go::Log
// verbatim. All fields are emitted on the wire (omitempty would change the
// wire format and is intentionally avoided so dd-go's Validate() sees the
// canonical shape).
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

// LogsPayload is the inner "payload" field of the apmtelemetry envelope
// when the request_type is "logs". The outer key is "logs" (NOT "records")
// per dd-go's LogsPayload definition.
type LogsPayload struct {
	Logs []Log `json:"logs"`
}
