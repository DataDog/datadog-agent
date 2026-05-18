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
	"strings"
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

// errorLogToLog converts an ErrorLog (carried across the pkg/util/log ->
// comp/core boundary) into the wire-shape Log struct expected by dd-go's
// tracer-telemetry-intake/telemetry-payload/logs.go.
//
// This pipeline ships PC-only telemetry. Every formatted slog message and
// every slog.Attr value is potentially user-controlled — paths, hostnames,
// request bodies, error strings carrying user data. Until template-aware
// static-message capture lands (follow-up), the only fields safe by
// construction are PC, Level, Time, and Count. The schema fields stay (wire-shape
// parity with dd-go) but are not populated and should not be populated by
// this path.
//
//   - Time   -> tracer_time (unix seconds)
//   - Level  -> uppercase LogLevel (always "ERROR" today)
//   - PC     -> single-frame stack_trace ("file:line")
//   - Count  -> 1 today; the Bouncer populates the suppressed-duplicate
//     count here.
//   - Message, Tags, TraceID, SpanID -> "" (not populated)
//   - IsCrash -> false (this path does not emit crash logs)
func errorLogToLog(e errortracking.ErrorLog) Log {
	count := int(e.Count)
	if count < 1 {
		// Defensive: producers that didn't go through the Bouncer
		// (synthetic tests, direct SubmitErrorRecord callers) won't
		// populate Count. Default to 1 so the wire field stays valid.
		count = 1
	}
	out := Log{
		Level:      slogLevelToLogLevel(e.Level),
		TracerTime: e.Time.Unix(),
		Count:      count,
		IsCrash:    false,
	}

	out.StackTrace = symbolizeStack(e)

	return out
}

// symbolizeStack walks the PCs captured at log time and produces the
// multi-line "file:line\tfunc" string the dd-go intake schema expects.
// Falls back to the single-PC path when the handler did not populate
// PCs (older callers, synthetic tests).
//
// Symbolization is deferred from handler-side to flush-time on purpose:
// runtime.CallersFrames performs symbol table lookups and is relatively
// expensive; doing it here keeps the producer's hot path zero-alloc
// beyond the runtime.Callers fixed-array fill.
func symbolizeStack(e errortracking.ErrorLog) string {
	if e.PCsLen > 0 {
		var b strings.Builder
		frames := runtime.CallersFrames(e.PCs[:e.PCsLen])
		for {
			frame, more := frames.Next()
			if frame.File != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				fmt.Fprintf(&b, "%s:%d\t%s", frame.File, frame.Line, frame.Function)
			}
			if !more {
				break
			}
		}
		return b.String()
	}
	if e.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{e.PC}).Next()
		if frame.File != "" {
			return fmt.Sprintf("%s:%d\t%s", frame.File, frame.Line, frame.Function)
		}
	}
	return ""
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
