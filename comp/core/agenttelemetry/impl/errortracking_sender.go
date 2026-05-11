// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// sendLogsTypedBatch is the v3 entry that takes already-converted wire
// Log structs (produced by errorLogToLog) and POSTs them as a single
// LogsPayload-envelope to every configured endpoint via the shared
// sendPayloadBody helper.
//
// Error semantics:
//   - empty batch is a no-op
//   - 5xx or request-timeout from any endpoint: non-nil joined error
//     (callers may retry once or drop, per their policy)
//   - 4xx from any endpoint: logged at error, treated as terminal for
//     that endpoint (no error returned for that endpoint specifically)
//   - transport failure: non-nil joined error
func (s *senderImpl) sendLogsTypedBatch(ctx context.Context, logs []Log) error {
	if len(logs) == 0 {
		return nil
	}

	payload := s.payloadTemplate
	payload.RequestType = logsPayloadType
	payload.EventTime = time.Now().Unix()
	payload.Payload = LogsPayload{Logs: logs}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal logs payload: %w", err)
	}
	body, err = scrubber.ScrubJSON(body)
	if err != nil {
		return fmt.Errorf("scrub logs payload: %w", err)
	}
	compressed := false
	if s.compress {
		if cBody, cErr := zstd.CompressLevel(nil, body, s.compressionLevel); cErr == nil {
			body = cBody
			compressed = true
		}
		// On compression failure, fall back to uncompressed (matches
		// flushSession's behavior).
	}

	var errs error
	for _, ep := range s.endpoints.Endpoints {
		url := buildURL(ep)
		status, sendErr := s.sendPayloadBody(ctx, body, logsPayloadType, ep.GetAPIKey(), url, compressed)
		if sendErr != nil {
			errs = errors.Join(errs, sendErr)
			continue
		}
		switch {
		case status >= 200 && status < 300:
			s.logComp.Debugf("Logs intake response status code:%d, request type:%s", status, logsPayloadType)
		case status >= 500 || status == http.StatusRequestTimeout:
			errs = errors.Join(errs,
				fmt.Errorf("logs intake returned %d at %s", status, url))
		default:
			s.logComp.Errorf("logs intake returned terminal %d at %s; dropping batch (%d records)",
				status, url, len(logs))
		}
	}
	return errs
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

// slogLevelToLogLevel maps slog.Level to the UPPERCASE wire LogLevel
// constants accepted by dd-go's logs intake. The handler at
// pkg/util/log/errortracking only forwards Level >= Error, so in
// practice only LogLevelError is emitted; other levels are mapped here
// for completeness and test coverage.
func slogLevelToLogLevel(l slog.Level) LogLevel {
	switch {
	case l >= slog.LevelError:
		return LogLevelError
	case l >= slog.LevelWarn:
		return LogLevelWarn
	case l >= slog.LevelInfo:
		return LogLevelInfo
	default:
		return LogLevelDebug
	}
}
