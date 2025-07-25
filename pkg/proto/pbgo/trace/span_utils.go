// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	binary "encoding/binary"
	"encoding/hex"
	"strconv"
)

// spanCopiedFields records the fields that are copied in ShallowCopy.
// This should match exactly the fields set in (*Span).ShallowCopy.
// This is used by tests to enforce the correctness of ShallowCopy.
var spanCopiedFields = map[string]struct{}{
	"Service":    {},
	"Name":       {},
	"Resource":   {},
	"TraceID":    {},
	"SpanID":     {},
	"ParentID":   {},
	"Start":      {},
	"Duration":   {},
	"Error":      {},
	"Meta":       {},
	"Metrics":    {},
	"Type":       {},
	"MetaStruct": {},
	"SpanLinks":  {},
	"SpanEvents": {},
}

// ShallowCopy returns a shallow copy of the copy-able portion of a Span. These are the
// public fields which will have a Get* method for them. The completeness of this
// method is enforced by the init function above. Instead of using pkg/proto/utils.ProtoCopier,
// which incurs heavy reflection cost for every copy at runtime, we use reflection once at
// startup to ensure our method is complete.
func (s *Span) ShallowCopy() *Span {
	if s == nil {
		return &Span{}
	}
	return &Span{
		Service:    s.Service,
		Name:       s.Name,
		Resource:   s.Resource,
		TraceID:    s.TraceID,
		SpanID:     s.SpanID,
		ParentID:   s.ParentID,
		Start:      s.Start,
		Duration:   s.Duration,
		Error:      s.Error,
		Meta:       s.Meta,
		Metrics:    s.Metrics,
		Type:       s.Type,
		MetaStruct: s.MetaStruct,
		SpanLinks:  s.SpanLinks,
		SpanEvents: s.SpanEvents,
	}
}

// Get128BitTraceID returns the 128-bit trace ID for the span.
func (s *Span) Get128BitTraceID() ([]byte, error) {
	// If it's an otel span the whole trace ID is in otel.trace
	if tid, ok := s.Meta["otel.trace_id"]; ok {
		bs, err := hex.DecodeString(tid)
		if err != nil {
			return nil, err
		}
		return bs, nil
	}
	tid := make([]byte, 16)
	binary.BigEndian.PutUint64(tid[8:], s.TraceID)
	// Get hex encoded upper bits for datadog spans
	// If no value is found we can use the default `0` value as that's what will have been propagated
	if upper, ok := s.Meta["_dd.p.tid"]; ok {
		u, err := strconv.ParseUint(upper, 16, 64)
		if err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint64(tid[:8], u)
	}
	return tid, nil
}
