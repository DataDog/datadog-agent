// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// ProcessedTrace represents a trace being processed in the agent.
type ProcessedTrace struct {
	TraceChunk             *pb.TraceChunk
	Root                   *pb.Span
	TracerEnv              string
	AppVersion             string
	TracerHostname         string
	ClientDroppedP0sWeight float64
}

// Clone creates a copy of ProcessedTrace, cloning p, p.TraceChunk, and p.Root. This means it is
// safe to modify the returned ProcessedTrace's (pt's) fields along with fields in
// pt.TraceChunk and fields in pt.Root.
//
// The most important consequence of this is that the TraceChunk's Spans field can be assigned,
// *BUT* the Spans value itself should not be modified. i.e. This is ok:
//
//	pt2 := pt.Clone()
//	pt2.TraceChunk.Spans = make([]*pb.Span)
//
// but this is NOT ok:
//
//	pt2 := pt.Clone()
//	pt2.TraceChunk.Spans[0] = &pb.Span{} // This will be visible in pt.
func (pt *ProcessedTrace) Clone() *ProcessedTrace {
	if pt == nil {
		return nil
	}
	ptClone := new(ProcessedTrace)
	*ptClone = *pt
	if pt.TraceChunk != nil {
		c := pt.TraceChunk.ShallowCopy()
		ptClone.TraceChunk = c
	}
	if pt.Root != nil {
		r := pt.Root.ShallowCopy()
		ptClone.Root = r
	}
	return ptClone
}

// func CopyTraceChunk(chunk *pb.TraceChunk) *pb.TraceChunk {
// 	c := new(pb.TraceChunk)
//
// 	c.Origin = chunk.GetOrigin()
// 	c.Tags = chunk.GetTags()
// 	c.Spans = chunk.GetSpans()
// 	c.Priority = chunk.GetPriority()
// 	c.DroppedTrace = chunk.GetDroppedTrace()
//
// 	return c
// }
//
// func CopyTraceSpan(span *pb.Span) *pb.Span {
// 	s := new(pb.Span)
//
// 	s.Service = span.GetService()
// 	s.Name = span.GetName()
// 	s.Resource = span.GetResource()
// 	s.TraceID = span.GetTraceID()
// 	s.SpanID = span.GetSpanID()
// 	s.ParentID = span.GetParentID()
// 	s.Start = span.GetStart()
// 	s.Duration = span.GetDuration()
// 	s.Error = span.GetError()
// 	s.Meta = span.GetMeta()
// 	s.Metrics = span.GetMetrics()
// 	s.Type = span.GetType()
// 	s.MetaStruct = span.GetMetaStruct()
//
// 	return s
// }
