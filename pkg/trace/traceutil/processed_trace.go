// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// ProcessedTrace represents a trace being processed in the agent.
type ProcessedTrace struct {
	TraceChunk             *pb.TraceChunk
	Root                   *pb.Span
	TracerEnv              string
	AppVersion             string
	TracerHostname         string
	ClientDroppedP0sWeight float64
	GitCommitSha           string
	ImageTag               string
	Lang                   string
}

// ProcessedTraceV1 represents a trace being processed in the agent.
type ProcessedTraceV1 struct {
	TraceChunk *idx.InternalTraceChunk
	Root       *idx.InternalSpan
	// We copy these fields from the tracer payload to enable processing each chunk independently
	TracerEnv              string
	AppVersion             string
	TracerHostname         string
	ClientDroppedP0sWeight float64
	GitCommitSha           string
	ImageTag               string
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

// Clone creates a copy of ProcessedTraceV1, cloning p, p.TraceChunk, and p.Root.
// TODO: can we avoid needing this at all?
func (pt *ProcessedTraceV1) Clone() *ProcessedTraceV1 {
	if pt == nil {
		return nil
	}
	ptClone := new(ProcessedTraceV1)
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
