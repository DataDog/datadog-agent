// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traceutil

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetEnv returns the first "env" tag found in trace t.
func GetEnv(t pb.Trace) string {
	for _, s := range t {
		if v, ok := s.Meta["env"]; ok {
			// exit this on first success
			return v
		}
	}
	return ""
}

// GetRoot extracts the root span from a trace
func GetRoot(t pb.Trace) *pb.Span {
	// That should be caught beforehand
	if len(t) == 0 {
		return nil
	}
	// General case: go over all spans and check for one which matching parent
	parentIDToChild := map[uint64]*pb.Span{}

	for i := range t {
		// Common case optimization: check for span with ParentID == 0, starting from the end,
		// since some clients report the root last
		j := len(t) - 1 - i
		if t[j].ParentID == 0 {
			return t[j]
		}
		parentIDToChild[t[j].ParentID] = t[j]
	}

	for i := range t {
		if _, ok := parentIDToChild[t[i].SpanID]; ok {
			delete(parentIDToChild, t[i].SpanID)
		}
	}

	// Here, if the trace is valid, we should have len(parentIDToChild) == 1
	if len(parentIDToChild) != 1 {
		log.Debugf("Didn't reliably find the root span for traceID:%v", t[0].TraceID)
	}

	// Have a safe bahavior if that's not the case
	// Pick the first span without its parent
	for parentID := range parentIDToChild {
		return parentIDToChild[parentID]
	}

	// Gracefully fail with the last span of the trace
	return t[len(t)-1]
}

// APITrace returns an APITrace from t, as required by the Datadog API.
// It also returns an estimated size in bytes.
func APITrace(t pb.Trace) *pb.APITrace {
	earliest, latest := int64(math.MaxInt64), int64(0)
	for _, s := range t {
		start := s.Start
		if start < earliest {
			earliest = start
		}
		end := s.Start + s.Duration
		if end > latest {
			latest = end
		}
	}
	return &pb.APITrace{
		TraceID:   t[0].TraceID,
		Spans:     t,
		StartTime: earliest,
		EndTime:   latest,
	}
}

// ChildrenMap returns a map containing for each span id the list of its
// direct children.
func ChildrenMap(t pb.Trace) map[uint64][]*pb.Span {
	childrenMap := make(map[uint64][]*pb.Span)

	for i := range t {
		span := t[i]
		if span.ParentID == 0 {
			continue
		}
		_, ok := childrenMap[span.SpanID]
		if !ok {
			childrenMap[span.SpanID] = []*pb.Span{}
		}
		children, ok := childrenMap[span.ParentID]
		if ok {
			children = append(children, span)
		} else {
			children = []*pb.Span{span}
		}
		childrenMap[span.ParentID] = children
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
func ComputeTopLevel(t pb.Trace) {
	// build a lookup map
	spanIDToIdx := make(map[uint64]int, len(t))
	for i, span := range t {
		spanIDToIdx[span.SpanID] = i
	}

	// iterate on each span and mark them as top-level if relevant
	for _, span := range t {
		if span.ParentID != 0 {
			if parentIdx, ok := spanIDToIdx[span.ParentID]; ok && t[parentIdx].Service == span.Service {
				continue
			}
		}
		SetTopLevel(span, true)
	}
}
