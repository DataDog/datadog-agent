// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	agentversion "github.com/DataDog/datadog-agent/pkg/version"
)

// commitSHAPlaceholder is embedded in the tags field by enrichErrorLog instead
// of the real 40-char hex SHA. The scrubber's appKeyReplacer masks 40-char hex
// strings (Datadog app-key pattern); this placeholder is not hex so it passes
// through unmodified. sendLogsBatch replaces it with version.FullCommit after
// scrubbing is done.
const commitSHAPlaceholder = "FULL_SHA_TO_BE_REPLACED"

// sendLogsBatch takes already-converted wire Log structs (produced by
// enrichErrorLog) and POSTs them as a single LogsPayload-envelope to every
// configured endpoint.
//
// The pipeline is: marshal → scrub → replace commitSHAPlaceholder with the
// real SHA → compress → endpoint fanout. The SHA is injected after scrubbing
// because the scrubber's appKeyReplacer masks 40-char hex strings (see
// commitSHAPlaceholder for details).
//
// Error semantics: only transport errors are joined into the returned error.
// Non-2xx HTTP statuses are logged at Debug and NOT surfaced to the caller —
// this matches the flushSession contract and keeps the shutdown drain treating
// "enqueue-to-HTTP succeeded" as success regardless of server response.
//
// This function does NOT log at Error. Doing so would re-enter the
// errortracking slog handler and feed logs back into the same flush path;
// callers observe failures via the returned error and log at Debug. The
// invariant is enforced by convention — see comp/core/agenttelemetry/def/component.go.
func (s *senderImpl) sendLogsBatch(ctx context.Context, logs []Log) error {
	if len(logs) == 0 {
		return nil
	}

	payload := s.payloadTemplate
	payload.RequestType = logsPayloadType
	payload.EventTime = time.Now().Unix()
	payload.Payload = LogsPayload{Logs: logs}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", logsPayloadType, err)
	}
	reqBodyRaw, err := scrubber.ScrubJSON(payloadJSON)
	if err != nil {
		return fmt.Errorf("scrub %s payload: %w", logsPayloadType, err)
	}

	// enrichErrorLog embeds commitSHAPlaceholder instead of the real SHA so
	// the scrubber's appKeyReplacer (which masks 40-char hex strings) does not
	// touch it. Replace the placeholder with the real value now that scrubbing
	// is done.
	if sha := agentversion.FullCommit; sha != "" {
		reqBodyRaw = bytes.ReplaceAll(reqBodyRaw, []byte(commitSHAPlaceholder), []byte(sha))
	}

	return s.sendPayloadBytes(ctx, reqBodyRaw, logsPayloadType)
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
//   - Tags   -> git.repository_url + git.commit.sha for Source Code Integration
//   - Message, TraceID, SpanID -> "" (not populated)
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
		ErrorKind:  e.ErrorKind,
		Tags:       "git.repository_url:https://github.com/DataDog/datadog-agent,git.commit.sha:" + commitSHAPlaceholder,
	}
	out.StackTrace = symbolizeStackFrames(e)
	return out
}

// symbolizeStackFrames walks the PCs captured at log time and produces
// a multi-line stack string in Go's standard format:
//
//	github.com/DataDog/datadog-agent/pkg/util/log.Errorf
//		/path/to/file.go:42 +0x1a4
//
// This matches the output of runtime panic traces and debug.Stack(), and
// is the format the Error Tracking parser expects for frame extraction.
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
			offset := uintptr(0)
			if frame.PC >= frame.Entry {
				offset = frame.PC - frame.Entry
			}
			fmt.Fprintf(&b, "%s\n\t%s:%d +0x%x", frame.Function, frame.File, frame.Line, offset)
		}
		if !more {
			break
		}
	}
	return b.String()
}
