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
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// sendLogsTypedBatch is the v3 entry that takes already-converted wire
// Log structs (produced by errorLogToLog) and POSTs them as a single
// LogsPayload-envelope to every configured endpoint via the shared
// sendSerializedPayload helper.
//
// The marshal -> scrub -> compress -> endpoint-fanout pipeline is owned by
// sendSerializedPayload (see sender.go); this method only constructs the
// logs payload envelope. Error semantics are uniform across reqType: any
// non-2xx status or transport failure joins into the returned error.
//
// This function does NOT log at Error. Doing so would re-enter the
// errortracking slog handler and feed records back into the same flush
// path (review comment F6 on PR #50607); callers observe failures via
// the returned error and log at Debug.
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
// expected by dd-go's tracer-telemetry-intake/telemetry-payload/logs.go:
//   - Time -> tracer_time (unix seconds)
//   - Level -> uppercase LogLevel
//   - Message -> message
//   - PC -> single-frame stack_trace ("file:line")
//   - Attrs.trace_id / .span_id -> reserved typed fields (extracted)
//   - remaining Attrs -> sorted CSV "key:value" tags
//   - count: 1 (no client-side dedup in v3)
//   - is_crash: false (this path does not emit crash logs)
func errorLogToLog(e errortracking.ErrorLog) Log {
	out := Log{
		Message:    e.Message,
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

	var pairs []string
	for _, a := range e.Attrs {
		switch a.Key {
		case "trace_id":
			out.TraceID = a.Value.String()
		case "span_id":
			out.SpanID = a.Value.String()
		default:
			pairs = append(pairs, a.Key+":"+a.Value.String())
		}
	}
	sort.Strings(pairs)
	out.Tags = strings.Join(pairs, ",")

	return out
}

// slogLevelToLogLevel maps the slog level on an ErrorLog to the
// UPPERCASE wire LogLevel constant accepted by dd-go's logs intake.
// The pkg/util/log/errortracking handler filters Level < Error before
// dispatch, so only LevelError ever reaches this function in practice.
// Lower levels are a contract violation and panic loudly so a future
// regression is caught in tests instead of silently producing an
// invalid wire payload.
func slogLevelToLogLevel(l slog.Level) LogLevel {
	if l < slog.LevelError {
		panic(fmt.Sprintf("slogLevelToLogLevel: handler must filter Level < Error; got %v", l))
	}
	return LogLevelError
}
