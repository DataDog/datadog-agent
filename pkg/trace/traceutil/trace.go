// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/tracerpayload"
)

const (
	envKey     = "env"
	versionKey = "version"
)

// GetEnv returns the first "env" tag found in trace t.
// Search starts by root
func GetEnv(root tracerpayload.Span, t tracerpayload.TraceChunk) string {
	if v, ok := root.Meta(envKey); ok {
		return v
	}
	for i := 0; i < t.NumSpans(); i++ {
		s := t.Span(i)
		//TODO: UHM WHAT Why do we assume the list of spans has some kind of ORDERING
		if s.SpanID() == root.SpanID() {
			continue
		}
		if v, ok := s.Meta(envKey); ok {
			return v
		}
	}
	return ""
}

// GetAppVersion returns the first "version" tag found in trace t.
// Search starts by root
func GetAppVersion(root tracerpayload.Span, t tracerpayload.TraceChunk) string {
	if v, ok := root.Meta(versionKey); ok {
		return v
	}
	for i := 0; i < t.NumSpans(); i++ {
		s := t.Span(i)
		//TODO: UHM WHAT Why do we assume the list of spans has some kind of ORDERING (AGAIN?!)
		if s.SpanID() == root.SpanID() {
			continue
		}
		if v, ok := s.Meta(versionKey); ok {
			return v
		}
	}
	return ""
}

// GetRoot extracts the root span from a trace
func GetRoot(t tracerpayload.TraceChunk) tracerpayload.Span {
	// That should be caught beforehand
	if t.NumSpans() == 0 {
		return nil
	}
	// General case: go over all spans and check for one which matching parent
	parentIDToChild := map[uint64]tracerpayload.Span{}

	for i := t.NumSpans() - 1; i >= 0; i-- {
		if t.Span(i).ParentID() == 0 {
			return t.Span(i)
		}
		parentIDToChild[t.Span(i).ParentID()] = t.Span(i)
	}

	for i := 0; i < t.NumSpans(); i++ {
		delete(parentIDToChild, t.Span(i).SpanID())
	}

	// Here, if the trace is valid, we should have len(parentIDToChild) == 1
	if len(parentIDToChild) != 1 {
		log.Debugf("Didn't reliably find the root span for traceID:%v", t.Span(0).TraceID())
	}

	// Have a safe behavior if that's not the case
	// Pick the first span without its parent
	for parentID := range parentIDToChild {
		return parentIDToChild[parentID]
	}

	// Gracefully fail with the last span of the trace
	return t.Span(t.NumSpans() - 1)
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
		childrenMap[span.ParentID] = append(childrenMap[span.ParentID], span)
	}

	return childrenMap
}

// ComputeTopLevel updates all the spans top-level attribute.
//
// A span is considered top-level if:
//   - it's a root span
//   - OR its parent is unknown (other part of the code, distributed trace)
//   - OR its parent belongs to another service (in that case it's a "local root"
//     being the highest ancestor of other spans belonging to this service and
//     attached to it).
func ComputeTopLevel(trace tracerpayload.TraceChunk) {
	spanIDToIndex := make(map[uint64]int, trace.NumSpans())
	for i := 0; i < trace.NumSpans(); i++ {
		s := trace.Span(i)
		spanIDToIndex[s.SpanID()] = i
	}
	for i := 0; i < trace.NumSpans(); i++ {
		s := trace.Span(i)
		if s.ParentID() == 0 {
			// span is a root span
			SetTopLevel(s, true)
			continue
		}
		parentIndex, ok := spanIDToIndex[s.ParentID()]
		if !ok {
			// span has no parent in chunk
			SetTopLevel(s, true)
			continue
		}
		if trace.Span(parentIndex).Service() != s.Service() {
			// parent is not in the same service
			SetTopLevel(s, true)
			continue
		}
	}
}
