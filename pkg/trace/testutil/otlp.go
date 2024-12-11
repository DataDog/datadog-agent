// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package testutil

import (
	"encoding/hex"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

var (
	// OTLPFixedSpanID specifies a fixed test SpanID.
	OTLPFixedSpanID = pcommon.SpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
	// OTLPFixedTraceID specifies a fixed test TraceID.
	OTLPFixedTraceID = pcommon.TraceID([16]byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
)

// OTLPSpanEvent defines an OTLP test span event.
type OTLPSpanEvent struct {
	Timestamp  uint64                 `json:"time_unix_nano"`
	Name       string                 `json:"name"`
	Attributes map[string]interface{} `json:"attributes"`
	Dropped    uint32                 `json:"dropped_attributes_count"`
}

// OTLPSpanLink defines an OTLP test span link.
type OTLPSpanLink struct {
	TraceID    string                 `json:"trace_id"`
	SpanID     string                 `json:"span_id"`
	TraceState string                 `json:"trace_state"`
	Attributes map[string]interface{} `json:"attributes"`
	Dropped    uint32                 `json:"dropped_attributes_count"`
}

// OTLPSpan defines an OTLP test span.
type OTLPSpan struct {
	TraceID    [16]byte
	SpanID     [8]byte
	TraceState string
	ParentID   [8]byte
	Name       string
	Kind       ptrace.SpanKind
	Start, End uint64
	Attributes map[string]interface{}
	Events     []OTLPSpanEvent
	Links      []OTLPSpanLink
	StatusMsg  string
	StatusCode ptrace.StatusCode
}

// OTLPResourceSpan specifies the configuration for generating an OTLP ResourceSpan.
type OTLPResourceSpan struct {
	LibName    string
	LibVersion string
	Attributes map[string]interface{}
	Spans      []*OTLPSpan
}

// setOTLPSpan configures span based on s.
func setOTLPSpan(span ptrace.Span, s *OTLPSpan) {
	if isZero(s.TraceID[:]) {
		span.SetTraceID(OTLPFixedTraceID)
	} else {
		span.SetTraceID(pcommon.TraceID(s.TraceID))
	}
	if isZero(s.SpanID[:]) {
		span.SetSpanID(OTLPFixedSpanID)
	} else {
		span.SetSpanID(pcommon.SpanID(s.SpanID))
	}
	span.TraceState().FromRaw(s.TraceState)
	span.SetParentSpanID(pcommon.SpanID(s.ParentID))
	span.SetName(s.Name)
	span.SetKind(s.Kind)
	if s.Start == 0 {
		span.SetStartTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	} else {
		span.SetStartTimestamp(pcommon.Timestamp(s.Start))
	}
	if s.End == 0 {
		span.SetEndTimestamp(span.StartTimestamp() + 200000000)
	} else {
		span.SetEndTimestamp(pcommon.Timestamp(s.End))
	}
	insertAttributes(span.Attributes(), s.Attributes)
	events := span.Events()
	for _, e := range s.Events {
		ev := events.AppendEmpty()
		ev.SetTimestamp(pcommon.Timestamp(e.Timestamp))
		ev.SetName(e.Name)
		insertAttributes(ev.Attributes(), e.Attributes)
		ev.SetDroppedAttributesCount(e.Dropped)
	}
	ls := span.Links()
	for _, l := range s.Links {
		li := ls.AppendEmpty()
		buf, err := hex.DecodeString(l.TraceID)
		if err != nil {
			panic(err)
		}
		li.SetTraceID(*(*pcommon.TraceID)(buf))
		buf, err = hex.DecodeString(l.SpanID)
		if err != nil {
			panic(err)
		}
		li.SetSpanID(*(*pcommon.SpanID)(buf))
		li.TraceState().FromRaw(l.TraceState)
		insertAttributes(li.Attributes(), l.Attributes)
		li.SetDroppedAttributesCount(l.Dropped)
	}
	span.Status().SetCode(s.StatusCode)
	span.Status().SetMessage(s.StatusMsg)
}

// NewOTLPSpan creates a new OTLP Span with the given options.
func NewOTLPSpan(s *OTLPSpan) ptrace.Span {
	span := ptrace.NewSpan()
	setOTLPSpan(span, s)
	return span
}

// NewOTLPTracesRequest creates a new TracesRequest based on the given definitions.
func NewOTLPTracesRequest(defs []OTLPResourceSpan) ptraceotlp.ExportRequest {
	td := ptrace.NewTraces()
	rspans := td.ResourceSpans()

	for _, def := range defs {
		rspan := rspans.AppendEmpty()
		ilibspan := rspan.ScopeSpans().AppendEmpty()
		ilibspan.Scope().SetName(def.LibName)
		ilibspan.Scope().SetVersion(def.LibVersion)
		insertAttributes(rspan.Resource().Attributes(), def.Attributes)
		for _, spandef := range def.Spans {
			span := ilibspan.Spans().AppendEmpty()
			setOTLPSpan(span, spandef)
		}
	}

	tr := ptraceotlp.NewExportRequestFromTraces(td)
	return tr
}

func insertAttributes(attr pcommon.Map, from map[string]interface{}) {
	for k, anyv := range from {
		switch v := anyv.(type) {
		case string:
			_, ok := attr.Get(k)
			if !ok {
				attr.PutStr(k, v)
			}
		case bool:
			_, ok := attr.Get(k)
			if !ok {
				attr.PutBool(k, v)
			}
		case int:
			_, ok := attr.Get(k)
			if !ok {
				attr.PutInt(k, int64(v))
			}
		case int64:
			_, ok := attr.Get(k)
			if !ok {
				attr.PutInt(k, v)
			}
		case float64:
			_, ok := attr.Get(k)
			if !ok {
				attr.PutDouble(k, v)
			}
		default:
			_, ok := attr.Get(k)
			if !ok {
				attr.PutStr(k, fmt.Sprint(v))
			}
		}
	}
}

func isZero(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return false
		}
	}
	return true
}
