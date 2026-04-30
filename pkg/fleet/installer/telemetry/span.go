// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type spanContextKey string

const (
	spanIDsKey          = spanContextKey("span_ids")
	serviceKey          = spanContextKey("span_service")
	samplingPriorityKey = spanContextKey("span_sampling_priority")
)

// Span represents a span.
type Span struct {
	mu       sync.Mutex
	span     span
	finished atomic.Bool
}

func newSpan(
	name string,
	parentID uint64,
	traceID uint64,
	service string,
	samplingPriority *int,
) *Span {
	if traceID == 0 {
		traceID = rand.Uint64()
		// Head sampling only applies when no explicit priority was propagated via ctx.
		if samplingPriority == nil && !headSamplingKeep(name, traceID) {
			traceID = dropTraceID
		}
	}
	if samplingPriority != nil && *samplingPriority <= 0 {
		traceID = dropTraceID
	}
	s := &Span{
		span: span{
			TraceID:  traceID,
			ParentID: parentID,
			SpanID:   rand.Uint64(),
			Name:     name,
			Resource: name,
			Service:  service,
			Start:    time.Now().UnixNano(),
			Meta:     make(map[string]string),
			Metrics:  make(map[string]float64),
		},
	}
	if samplingPriority != nil {
		s.span.Metrics["_sampling_priority_v1"] = float64(*samplingPriority)
	}
	if parentID == 0 {
		s.SetTopLevel()
	}

	if globalTracer != nil {
		globalTracer.registerSpan(s)
	}
	return s
}

// Finish finishes the span with an error.
func (s *Span) Finish(err error) {
	s.finished.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.span.Duration = time.Now().UnixNano() - s.span.Start
	if err != nil {
		s.span.Error = 1
		s.setTag("error.message", err.Error())
		s.setTag("error.type", getRootErrorType(err))
		if st := extractStackTrace(err); st != "" {
			s.setTag("error.stack", st)
		} else {
			s.setTag("error.stack", takeStacktrace(1))
		}
		if _, ok := err.(fmt.Formatter); ok {
			s.setTag("error.details", fmt.Sprintf("%+v", err))
		}
	}
	globalTracer.finishSpan(s)
}

// SetResourceName sets the resource name of the span.
func (s *Span) SetResourceName(name string) {
	if s.finished.Load() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.span.Resource = name
}

// SetTopLevel sets the span as a top level span.
func (s *Span) SetTopLevel() {
	s.SetTag("_top_level", 1)
}

// SetTag sets a tag on the span.
func (s *Span) SetTag(key string, value interface{}) {
	if s.finished.Load() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setTag(key, value)
}

// setTag sets the span on the tag, requires s.mu lock to be held
func (s *Span) setTag(key string, value interface{}) {
	if value == nil {
		s.span.Meta[key] = "nil"
	}
	switch v := value.(type) {
	case string:
		s.span.Meta[key] = v
	case bool:
		s.span.Meta[key] = strconv.FormatBool(v)
	case int:
		s.span.Metrics[key] = float64(v)
	case int8:
		s.span.Metrics[key] = float64(v)
	case int16:
		s.span.Metrics[key] = float64(v)
	case int32:
		s.span.Metrics[key] = float64(v)
	case int64:
		s.span.Metrics[key] = float64(v)
	case uint:
		s.span.Metrics[key] = float64(v)
	case uint8:
		s.span.Metrics[key] = float64(v)
	case uint16:
		s.span.Metrics[key] = float64(v)
	case uint32:
		s.span.Metrics[key] = float64(v)
	case uint64:
		s.span.Metrics[key] = float64(v)
	case float32:
		s.span.Metrics[key] = float64(v)
	case float64:
		s.span.Metrics[key] = v
	default:
		s.span.Meta[key] = fmt.Sprint(v)
	}
}

type spanIDs struct {
	traceID uint64
	spanID  uint64
}

func getSpanIDsFromContext(ctx context.Context) (spanIDs, bool) {
	sIDs, ok := ctx.Value(spanIDsKey).(spanIDs)
	if !ok {
		return spanIDs{}, false
	}
	return sIDs, true
}

func setSpanIDsInContext(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanIDsKey, spanIDs{traceID: span.span.TraceID, spanID: span.span.SpanID})
}

func getRootErrorType(err error) string {
	if err == nil {
		return ""
	}

	for u := errors.Unwrap(err); u != nil; u = errors.Unwrap(u) {
		err = u
	}
	return reflect.TypeOf(err).String()
}

// stackTracer is implemented by errors that carry a stack trace captured at creation time.
type stackTracer interface {
	StackTrace() []uintptr
}

// extractStackTrace walks the error chain to find the deepest error with a creation-time
// stack trace, and formats it. Returns empty string if no stack trace is found.
func extractStackTrace(err error) string {
	var st stackTracer
	for u := err; u != nil; u = errors.Unwrap(u) {
		if s, ok := u.(stackTracer); ok {
			st = s
		}
	}
	if st == nil {
		return ""
	}
	return formatStack(st.StackTrace())
}

// takeStacktrace captures the current call stack and returns it as a formatted string.
// skip is the number of additional frames to skip beyond takeStacktrace and runtime.Callers.
func takeStacktrace(skip int) string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip+2, pcs) // +2 to skip runtime.Callers and takeStacktrace
	if n == 0 {
		return ""
	}
	return formatStack(pcs[:n])
}

func formatStack(pcs []uintptr) string {
	if len(pcs) == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs)
	var buf strings.Builder
	first := true
	for {
		frame, more := frames.Next()
		if isInternalFrame(frame.Function) {
			if !more {
				break
			}
			continue
		}
		if !first {
			buf.WriteByte('\n')
		}
		first = false
		buf.WriteString(frame.Function)
		buf.WriteByte('\n')
		buf.WriteByte('\t')
		buf.WriteString(frame.File)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(frame.Line))
		if !more {
			break
		}
	}
	return buf.String()
}

func isInternalFrame(fn string) bool {
	return strings.HasPrefix(fn, "runtime.") ||
		strings.HasPrefix(fn, "runtime/debug.") ||
		strings.Contains(fn, "orchestrion")
}
