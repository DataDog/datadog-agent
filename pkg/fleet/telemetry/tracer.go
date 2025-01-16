// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"math"
	"runtime/debug"
	"strings"
	"sync"
)

const (
	dropTraceID      = 1
	maxSpansInFlight = 1000
)

var (
	globalTracer  *tracer
	samplingRates = map[string]float64{
		"cdn":             0.1,
		"garbage_collect": 0.05,
		"HTTPClient":      0.05,
	}
	defaultMeta = map[string]string{}
)

func init() {
	globalTracer = &tracer{
		spans: make(map[uint64]*Span),
	}
	defaultMeta = getTagsFromBinary()
}

func initMeta() map[string]string {
	cloned := make(map[string]string, len(defaultMeta))
	for k, v := range defaultMeta {
		cloned[k] = v
	}
	return cloned
}

func getTagsFromBinary() map[string]string {
	res := make(map[string]string)
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return res
	}
	goPath := info.Path
	var commitSha string
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			commitSha = s.Value
		}
	}
	res["_dd.git.commit.sha"] = commitSha
	res["_dd.go_path"] = goPath
	res["git.repository_url"] = "https://github.com/DataDog/datadog-agent"
	res["git.repository.id"] = "github.com/datadog/datadog-agent"
	return res
}

type tracer struct {
	mu             sync.Mutex
	spans          map[uint64]*Span
	completedSpans []*Span
}

func (t *tracer) registerSpan(span *Span) {
	if span.span.TraceID == dropTraceID {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// naive maxSpansInFlight check as this is just telemetry
	// next iteration if needed would be to flush long running spans to troubleshoot
	if len(t.spans) >= maxSpansInFlight {
		return
	}
	t.spans[span.span.SpanID] = span
}

func (t *tracer) getSpan(spanID uint64) (*Span, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	span, ok := t.spans[spanID]
	return span, ok
}

func (t *tracer) finishSpan(span *Span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.spans, span.span.SpanID)
	t.completedSpans = append(t.completedSpans, span)
}

func (t *tracer) flushCompletedSpans() []*Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	newSpanArray := make([]*Span, 0)
	completedSpans := t.completedSpans
	t.completedSpans = newSpanArray
	return completedSpans
}

func headSamplingKeep(spanName string, traceID uint64) bool {
	for k, r := range samplingRates {
		if strings.Contains(spanName, k) {
			return sampledByRate(traceID, r)
		}
	}
	return true
}

func sampledByRate(n uint64, rate float64) bool {
	if rate < 1 {
		return n*uint64(1111111111111111111) < uint64(rate*math.MaxUint64)
	}
	return true
}
