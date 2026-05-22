// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// sendLogsBatch takes already-converted wire Log structs (produced by
// enrichErrorLog) and POSTs them as a single LogsPayload-envelope to
// every configured endpoint via the shared sendPayload helper.
//
// The marshal -> scrub -> compress -> endpoint-fanout pipeline is owned
// by sendPayload (see sender.go); this method only constructs the logs
// payload envelope.
//
// Error semantics: only transport errors (network failures,
// request-build errors) are joined into the returned error. Non-2xx
// HTTP statuses (4xx/5xx including 429) are logged at Debug by
// sendPayload and NOT surfaced to the caller — this matches the
// existing flushSession contract and keeps the shutdown drain treating
// "enqueue-to-HTTP succeeded" as success regardless of server response.
//
// This function does NOT log at Error. Doing so would re-enter the
// errortracking slog handler and feed logs back into the same flush
// path; callers observe failures via the returned error and log at
// Debug. The invariant is enforced by convention — see
// comp/core/agenttelemetry/def/component.go.
func (s *senderImpl) sendLogsBatch(ctx context.Context, logs []Log) error {
	if len(logs) == 0 {
		return nil
	}

	payload := s.payloadTemplate
	payload.RequestType = logsPayloadType
	payload.EventTime = time.Now().Unix()
	payload.Payload = LogsPayload{Logs: logs}

	return s.sendPayload(ctx, payload, logsPayloadType)
}

// enrichErrorLog converts an ErrorLog (carried across the pkg/util/log ->
// comp/core boundary) into the wire-shape Log struct expected by dd-go's
// tracer-telemetry-intake/telemetry-payload/logs.go.
//
// This pipeline ships PC-only telemetry. Every formatted slog message and
// every slog.Attr value is potentially user-controlled — paths, hostnames,
// request bodies, error strings carrying user data. The handler does not
// capture them, and the wire-shape schema fields that the dd-go intake
// expects stay empty here.
//
//   - Time   -> tracer_time (unix seconds)
//   - Level  -> LogLevelError (the only level this pipeline emits)
//   - PCs    -> multi-frame stack_trace ("file:line\tfunc" per line)
//   - Count  -> 1 today; the Bouncer populates the suppressed-duplicate
//     count here.
//   - Message, Tags, TraceID, SpanID -> "" (not populated)
//   - IsCrash -> false (this path does not emit crash logs)
func enrichErrorLog(e errortracking.ErrorLog) Log {
	count := int(e.Count)
	if count < 1 {
		// Defensive: producers that didn't go through the Bouncer
		// (synthetic tests, direct SubmitErrorLog callers) won't
		// populate Count. Default to 1 so the wire field stays valid.
		count = 1
	}
	out := Log{
		Level:      LogLevelError,
		TracerTime: e.Time.Unix(),
		Count:      count,
		IsCrash:    false,
	}
	out.StackTrace = symbolizeStackFrames(e)
	return out
}

// symbolizeStackFrames walks the PCs captured at log time and produces
// the multi-line "file:line\tfunc" string the dd-go intake schema expects.
//
// Symbolization is deferred from handler-side to flush-time on purpose:
// runtime.CallersFrames performs symbol table lookups and is relatively
// expensive; doing it here keeps the producer's hot path zero-alloc
// beyond the runtime.Callers fixed-array fill.
func symbolizeStackFrames(e errortracking.ErrorLog) string {
	if e.PCsLen == 0 {
		return ""
	}
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
