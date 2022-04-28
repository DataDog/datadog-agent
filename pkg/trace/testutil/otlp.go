package testutil

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

var (
	// OTLPFixedSpanID specifies a fixed test SpanID.
	OTLPFixedSpanID = pcommon.NewSpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
	// OTLPFixedTraceID specifies a fixed test TraceID.
	OTLPFixedTraceID = pcommon.NewTraceID([16]byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
)

// OTLPSpanEvent defines an OTLP test span event.
type OTLPSpanEvent struct {
	Timestamp  uint64                 `json:"time_unix_nano"`
	Name       string                 `json:"name"`
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
		span.SetTraceID(pcommon.NewTraceID(s.TraceID))
	}
	if isZero(s.SpanID[:]) {
		span.SetSpanID(OTLPFixedSpanID)
	} else {
		span.SetSpanID(pcommon.NewSpanID(s.SpanID))
	}
	span.SetTraceState(ptrace.TraceState(s.TraceState))
	span.SetParentSpanID(pcommon.NewSpanID(s.ParentID))
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
func NewOTLPTracesRequest(defs []OTLPResourceSpan) ptraceotlp.Request {
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

	tr := ptraceotlp.NewRequest()
	tr.SetTraces(td)
	return tr
}

func insertAttributes(attr pcommon.Map, from map[string]interface{}) {
	for k, anyv := range from {
		switch v := anyv.(type) {
		case string:
			attr.Insert(k, pcommon.NewValueString(v))
		case bool:
			attr.Insert(k, pcommon.NewValueBool(v))
		case int:
			attr.Insert(k, pcommon.NewValueInt(int64(v)))
		case int64:
			attr.Insert(k, pcommon.NewValueInt(v))
		case float64:
			attr.Insert(k, pcommon.NewValueDouble(v))
		default:
			attr.Insert(k, pcommon.NewValueString(fmt.Sprint(v)))
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
