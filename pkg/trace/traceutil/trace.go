// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traceutil

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/trace/traces"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetEnv returns the meta value for the "env" key for
// the first trace it finds or an empty string
func GetEnv(t traces.Trace) string {
	// TODO: Fix me.

	// exit this on first success
	// for _, s := range t.Spans {
	// for k, v := range s.Meta {
	// 	if k == "env" {
	// 		return v
	// 	}
	// }
	// }
	return ""
}

// GetRoot extracts the root span from a trace
func GetRoot(t traces.Trace) traces.Span {
	// That should be caught beforehand
	if len(t.Spans) == 0 {
		return nil
	}
	// General case: go over all spans and check for one which matching parent
	parentIDToChild := map[uint64]traces.Span{}

	for i := range t.Spans {
		// Common case optimization: check for span with ParentID == 0, starting from the end,
		// since some clients report the root last
		j := len(t.Spans) - 1 - i
		if t.Spans[j].ParentID() == 0 {
			return t.Spans[j]
		}
		parentIDToChild[t.Spans[j].ParentID()] = t.Spans[j]
	}

	for i := range t.Spans {
		if _, ok := parentIDToChild[t.Spans[i].SpanID()]; ok {
			delete(parentIDToChild, t.Spans[i].SpanID())
		}
	}

	// Here, if the trace is valid, we should have len(parentIDToChild) == 1
	if len(parentIDToChild) != 1 {
		log.Debugf("Didn't reliably find the root span for traceID:%v", t.Spans[0].TraceID())
	}

	// Have a safe bahavior if that's not the case
	// Pick the first span without its parent
	for parentID := range parentIDToChild {
		return parentIDToChild[parentID]
	}

	// Gracefully fail with the last span of the trace
	return t.Spans[len(t.Spans)-1]
}

type APITrace struct {
	TraceID   uint64
	Trace     traces.Trace
	StartTime int64
	EndTime   int64
}

// NewAPITrace returns an APITrace from t, as required by the Datadog API.
// It also returns an estimated size in bytes.
func NewAPITrace(t traces.Trace) APITrace {
	earliest, latest := int64(math.MaxInt64), int64(0)
	for _, s := range t.Spans {
		start := s.Start()
		if start < earliest {
			earliest = start
		}
		end := s.Start() + s.Duration()
		if end > latest {
			latest = end
		}
	}
	return APITrace{
		TraceID:   t.Spans[0].TraceID(),
		Trace:     t,
		StartTime: earliest,
		EndTime:   latest,
	}
}

// ChildrenMap returns a map containing for each span id the list of its
// direct children.
func ChildrenMap(t traces.Trace) map[uint64][]traces.Span {
	childrenMap := make(map[uint64][]traces.Span)

	for i := range t.Spans {
		span := t.Spans[i]
		if span.ParentID() == 0 {
			continue
		}
		_, ok := childrenMap[span.SpanID()]
		if !ok {
			childrenMap[span.SpanID()] = []traces.Span{}
		}
		children, ok := childrenMap[span.ParentID()]
		if ok {
			children = append(children, span)
		} else {
			children = []traces.Span{span}
		}
		childrenMap[span.ParentID()] = children
	}

	return childrenMap
}

// ComputeTopLevel updates all the spans top-level attribute.
//
// A span is considered top-level if:
// - it's a root span
// - its parent is unknown (other part of the code, distributed trace)
// - its parent belongs to another service (in that case it's a "local root"
//   being the highest ancestor of other spans belonging to this service and
//   attached to it).
func ComputeTopLevel(t traces.Trace) {
	// build a lookup map
	spanIDToIdx := make(map[uint64]int, len(t.Spans))
	for i, span := range t.Spans {
		spanIDToIdx[span.SpanID()] = i
	}

	// iterate on each span and mark them as top-level if relevant
	for _, span := range t.Spans {
		if span.ParentID() != 0 {
			if parentIdx, ok := spanIDToIdx[span.ParentID()]; ok && t.Spans[parentIdx].UnsafeService() == span.UnsafeService() {
				continue
			}
		}
		SetTopLevel(span, true)
	}
}
