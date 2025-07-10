// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const spanKey = spanContextKey("span_context")

type spanContextKey string

// Span represents a span.
type Span struct {
	mu       sync.Mutex
	span     span
	finished atomic.Bool
}

func newSpan(name string, parentID, traceID uint64) *Span {
	if traceID == 0 {
		traceID = rand.Uint64()
		if !headSamplingKeep(name, traceID) {
			traceID = dropTraceID
		}
	}
	s := &Span{
		span: span{
			TraceID:  traceID,
			ParentID: parentID,
			SpanID:   rand.Uint64(),
			Name:     name,
			Resource: name,
			Start:    time.Now().UnixNano(),
			Meta:     make(map[string]string),
			Metrics:  make(map[string]float64),
		},
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
		s.span.Meta = map[string]string{
			"error.message": err.Error(),
			"error.stack":   string(debug.Stack()),
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
	sIDs, ok := ctx.Value(spanKey).(spanIDs)
	if !ok {
		return spanIDs{}, false
	}
	return sIDs, true
}

func setSpanIDsInContext(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanKey, spanIDs{traceID: span.span.TraceID, spanID: span.span.SpanID})
}
