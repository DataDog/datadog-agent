// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// sendLogsBatch ships a batch of slog records to the COAT intake using the
// logs-track payload (request_type=logs). It mirrors flushSession's
// transport behavior (marshal, scrub, optionally compress, POST to each
// endpoint) but skips the metric/event session abstraction since logs
// batches are single-shot.
//
// Error semantics (per pkg/util/log/errortracking.Sender contract):
//   - empty batch is a no-op
//   - 5xx response or network failure: non-nil error (calling Pipeline
//     retries once then drops the batch)
//   - 4xx response: log internally and return nil (terminal; retrying a
//     malformed payload wastes the Pipeline's single retry slot)
func (s *senderImpl) sendLogsBatch(ctx context.Context, batch []slog.Record) error {
	if len(batch) == 0 {
		return nil
	}

	logs := make([]Log, len(batch))
	for i, r := range batch {
		logs[i] = slogRecordToLog(r)
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

	bodyLen := strconv.Itoa(len(body))
	var errs error
	for _, ep := range s.endpoints.Endpoints {
		url := buildURL(ep)
		req, reqErr := http.NewRequest("POST", url, bytes.NewReader(body))
		if reqErr != nil {
			errs = errors.Join(errs, fmt.Errorf("new logs request to %s: %w", url, reqErr))
			continue
		}
		s.addHeaders(req, logsPayloadType, ep.GetAPIKey(), bodyLen, compressed)
		resp, doErr := s.client.Do(req.WithContext(ctx))
		if doErr != nil {
			errs = errors.Join(errs, fmt.Errorf("post logs to %s: %w", url, doErr))
			continue
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			s.logComp.Debugf("Logs intake response status:%s, request type:%s, status code:%d",
				resp.Status, logsPayloadType, resp.StatusCode)
		case resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout:
			// Retryable per the Pipeline contract.
			errs = errors.Join(errs,
				fmt.Errorf("logs intake returned %d at %s", resp.StatusCode, url))
		default:
			// 4xx — terminal. Log and treat as delivered so the Pipeline
			// does not retry a request that will not succeed.
			s.logComp.Errorf("logs intake returned terminal %d at %s; dropping batch (%d records)",
				resp.StatusCode, url, len(batch))
		}
	}
	return errs
}

// slogRecordToLog maps an slog.Record to a wire-level Log per the schema
// in ~/repos/COAT/errortracking/errortracking.md §18:
//   - Time → tracer_time (unix seconds)
//   - Level → uppercase LogLevel
//   - Message → message
//   - PC → single-frame stack_trace ("file:line")
//   - attrs.trace_id / attrs.span_id → reserved typed fields (extracted)
//   - remaining attrs → sorted CSV "key:value" tags
//   - count: 1 (no client-side dedup in v1)
//   - is_crash: false (this path does not emit crash logs in v1)
func slogRecordToLog(r slog.Record) Log {
	out := Log{
		Message:    r.Message,
		Level:      slogLevelToLogLevel(r.Level),
		TracerTime: r.Time.Unix(),
		Count:      1,
		IsCrash:    false,
	}

	if r.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		if frame.File != "" {
			out.StackTrace = fmt.Sprintf("%s:%d", frame.File, frame.Line)
		}
	}

	var pairs []string
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "trace_id":
			out.TraceID = a.Value.String()
		case "span_id":
			out.SpanID = a.Value.String()
		default:
			pairs = append(pairs, a.Key+":"+a.Value.String())
		}
		return true
	})
	sort.Strings(pairs)
	out.Tags = strings.Join(pairs, ",")

	return out
}

// slogLevelToLogLevel maps slog.Level to the UPPERCASE wire LogLevel
// constants accepted by dd-go's logs intake. The handler at
// pkg/util/log/errortracking only forwards Level >= Error in v1, so in
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
