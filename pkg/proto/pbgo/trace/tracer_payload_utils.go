// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

// traceChunkCopiedFields records the fields that are copied in ShallowCopy.
// This should match exactly the fields set in (*TraceChunk).ShallowCopy.
// This is used by tests to enforce the correctness of ShallowCopy.
var traceChunkCopiedFields = map[string]struct{}{
	"Priority":     {},
	"Origin":       {},
	"Spans":        {},
	"Tags":         {},
	"DroppedTrace": {},
}

// ShallowCopy returns a shallow copy of the copy-able portion of a TraceChunk. These are the
// public fields which will have a Get* method for them. The completeness of this
// method is enforced by the init function above. Instead of using pkg/proto/utils.ProtoCopier,
// which incurs heavy reflection cost for every copy at runtime, we use reflection once at
// startup to ensure our method is complete.
func (t *TraceChunk) ShallowCopy() *TraceChunk {
	if t == nil {
		return nil
	}
	return &TraceChunk{
		Priority:     t.Priority,
		Origin:       t.Origin,
		Spans:        t.Spans,
		Tags:         t.Tags,
		DroppedTrace: t.DroppedTrace,
	}
}
