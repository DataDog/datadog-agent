// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// sendLogsTypedBatch is the entry that takes already-converted wire
// Log structs (produced by errorLogToLog) and POSTs them as a single
// LogsPayload-envelope to every configured endpoint via the shared
// sendSerializedPayload helper.
//
// The marshal -> scrub -> compress -> endpoint-fanout pipeline is owned
// by sendSerializedPayload (see sender.go); this method only constructs
// the logs payload envelope.
//
// Error semantics: only transport errors (network failures,
// request-build errors) are joined into the returned error. Non-2xx
// HTTP statuses (4xx/5xx including 429) are logged at Debug by
// sendSerializedPayload and NOT surfaced to the caller — this matches
// the existing flushSession contract and keeps the shutdown drain
// treating "enqueue-to-HTTP succeeded" as success regardless of server
// response. Addresses louis-cqrl's 🟡 "doc comment misrepresents the
// error contract" thread on PR #50607.
//
// This function does NOT log at Error. Doing so would re-enter the
// errortracking slog handler and feed records back into the same flush
// path; callers observe failures via the returned error and log at
// Debug. The invariant is enforced by convention — see
// comp/core/agenttelemetry/def/component.go.
func (s *senderImpl) sendLogsTypedBatch(ctx context.Context, logs []Log) error {
	if len(logs) == 0 {
		return nil
	}

	payload := s.payloadTemplate
	payload.RequestType = logsPayloadType
	payload.EventTime = time.Now().Unix()
	payload.Payload = LogsPayload{Logs: logs}

	return s.sendSerializedPayload(ctx, payload, logsPayloadType)
}

// errorLogToLog converts the foundational ErrorLog DTO (carried across
// the pkg/util/log -> comp/core boundary) into the wire-shape Log struct
// expected by dd-go's tracer-telemetry-intake/telemetry-payload/logs.go.
//
// PII pivot (PR #50607): Message and Tags are intentionally emitted as
// empty strings. Every formatted slog message and every slog.Attr value
// is potentially user-controlled — paths, hostnames, request bodies,
// or error strings carrying user data. Until template-aware static-
// message capture lands (follow-up PR), the only fields safe by
// construction are PC and Level. The schema fields stay (wire-shape
// parity with dd-go) but always emit empty.
//
//   - Time   -> tracer_time (unix seconds)
//   - Level  -> uppercase LogLevel (always "ERROR" today)
//   - PC     -> single-frame stack_trace ("file:line")
//   - Count  -> 1 today; the Bouncer (see follow-up commit) populates
//     the suppressed-duplicate count here when it lands.
//   - Message, Tags, TraceID, SpanID -> "" (PII or unpopulated)
//   - IsCrash -> false (this path does not emit crash logs)
func errorLogToLog(e errortracking.ErrorLog) Log {
	out := Log{
		Level:      slogLevelToLogLevel(e.Level),
		TracerTime: e.Time.Unix(),
		Count:      1,
		IsCrash:    false,
	}

	if e.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{e.PC}).Next()
		if frame.File != "" {
			out.StackTrace = fmt.Sprintf("%s:%d", frame.File, frame.Line)
		}
	}

	return out
}

// slogLevelToLogLevel maps the slog level on an ErrorLog to the
// UPPERCASE wire LogLevel constant accepted by dd-go's logs intake.
//
// The wire schema in this PR only emits LogLevelError — non-error
// levels are filtered at handler.Enabled (pkg/util/log/errortracking)
// and not part of the flush-path contract. The function is therefore
// total: any input maps to LogLevelError. This intentionally reverses
// the prior-round F5 design (which panicked on sub-Error inputs):
// the panic ran on a background flush goroutine and would crash the
// agent on any direct SubmitErrorRecord caller that bypassed the
// handler filter. Addresses louis-cqrl's 🟠 thread on PR #50607 and
// pducolin's overlapping suggestion.
//
// When we later widen the wire schema to non-error levels, this
// function gains a real mapping (and a real error contract) — for
// now, totality is the right contract.
func slogLevelToLogLevel(_ slog.Level) LogLevel {
	return LogLevelError
}
