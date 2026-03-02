// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/encoding/protowire"
)

// OTLP wire format field numbers (from opentelemetry/proto).
const (
	// ExportTraceServiceRequest
	exportTraceServiceRequestResourceSpans = 1
	// ResourceSpans
	resourceSpansResource   = 1
	resourceSpansScopeSpans = 2
	// Resource
	resourceAttributes = 1
	// ScopeSpans
	scopeSpansScope = 1
	scopeSpansSpans = 2
	// InstrumentationScope
	scopeName    = 1
	scopeVersion = 2
	// Span
	spanTraceID            = 1
	spanSpanID             = 2
	spanTraceState         = 3
	spanParentSpanID       = 4
	spanName               = 5
	spanKind               = 6
	spanStartTimeUnixNano  = 7
	spanEndTimeUnixNano    = 8
	spanAttributes         = 9
	spanEvents             = 11
	spanLinks              = 13
	spanStatus             = 15
	// Status
	statusCode = 3
	// KeyValue
	keyValueKey   = 1
	keyValueValue = 2
	// AnyValue (attribute value)
	anyValueString = 1
	anyValueBool   = 2
	anyValueInt    = 3
	anyValueDouble = 4
	anyValueBytes  = 7
)

// attrVal holds a typed attribute value for span attributes (so numbers go to Metrics).
type attrVal struct {
	str string
	i   int64
	f   float64
	b   bool
	typ byte // 0=string, 1=int, 2=double, 3=bool
}

// otelTraceIDToUint64 converts OTLP trace ID (last 8 bytes, big-endian) to uint64 for pb.Span.
func otelTraceIDToUint64(b [16]byte) uint64 {
	return binary.BigEndian.Uint64(b[8:])
}

// otelSpanIDToUint64 converts OTLP span ID (8 bytes, big-endian) to uint64 for pb.Span.
func otelSpanIDToUint64(b [8]byte) uint64 {
	return binary.BigEndian.Uint64(b[:])
}

// parseSpanWireIntoDDSpan parses raw OTLP Span message bytes directly into the given pb.Span.
// It sets TraceID, SpanID, ParentID, Start, Duration, Error on span; writes string attributes
// to span.Meta and numeric attributes to span.Metrics (using raw OTLP keys); appends events/links
// JSON to the builders. Returns name, kind, status, traceState, traceID, parentSpanID and
// hasExceptionEvent so the caller can set Service/Name/Resource/Type and remap keys.
// Caller must reset the builders (to "[") before each span.
func parseSpanWireIntoDDSpan(
	data []byte,
	span *pb.Span,
	eventsBuf, linksBuf *strings.Builder,
	firstEvent, firstLink *bool,
	eventAttrsReuse map[string]interface{},
	linkAttrsReuse map[string]string,
) (name string, kind int32, statusCode int32, statusMessage string, traceState string, traceID [16]byte, parentSpanID [8]byte, hasExceptionEvent bool, err error) {
	kind = 1
	var startTimeUnixNano, endTimeUnixNano uint64
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case spanTraceID:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				copy(traceID[:], v)
				span.TraceID = otelTraceIDToUint64(traceID)
			}
		case spanSpanID:
			if wireType == protowire.BytesType {
				var sid [8]byte
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				copy(sid[:], v)
				span.SpanID = otelSpanIDToUint64(sid)
			}
		case spanTraceState:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				traceState = string(v)
			}
		case spanParentSpanID:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				copy(parentSpanID[:], v)
				span.ParentID = otelSpanIDToUint64(parentSpanID)
			}
		case spanName:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				name = string(v)
			}
		case spanKind:
			if wireType == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data)
				data = data[n:]
				kind = int32(v)
			}
		case spanStartTimeUnixNano:
			if wireType == protowire.Fixed64Type {
				v, n := protowire.ConsumeFixed64(data)
				data = data[n:]
				startTimeUnixNano = v
			}
		case spanEndTimeUnixNano:
			if wireType == protowire.Fixed64Type {
				v, n := protowire.ConsumeFixed64(data)
				data = data[n:]
				endTimeUnixNano = v
			}
		case spanAttributes:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				k, v, parseErr := parseKeyValueTyped(msg)
				if parseErr != nil {
					continue
				}
				if k != "" {
					switch v.typ {
					case 1:
						span.Metrics[k] = float64(v.i)
					case 2:
						span.Metrics[k] = v.f
					default:
						span.Meta[k] = v.str
					}
				}
			}
		case spanEvents:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				evName, appendErr := parseSpanEventAppendJSON(msg, eventsBuf, firstEvent, eventAttrsReuse)
				if appendErr == nil && evName == "exception" {
					hasExceptionEvent = true
				}
			}
		case spanLinks:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				_ = parseSpanLinkAppendJSON(msg, linksBuf, firstLink, linkAttrsReuse)
			}
		case spanStatus:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				statusCode, statusMessage = parseStatus(msg)
				if statusCode == 2 {
					span.Error = 1
				}
			}
		default:
			data = skipField(data, wireType)
		}
	}
	span.Start = int64(startTimeUnixNano)
	span.Duration = int64(endTimeUnixNano) - int64(startTimeUnixNano)
	return name, kind, statusCode, statusMessage, traceState, traceID, parentSpanID, hasExceptionEvent, nil
}

// parseOTLPExportTraceServiceRequestRaw parses the raw OTLP ExportTraceServiceRequest bytes
// and returns each ResourceSpans message as raw bytes. Used by the raw path to build
// TracerPayload directly without pdata.
func parseOTLPExportTraceServiceRequestRaw(data []byte) ([][]byte, error) {
	var out [][]byte
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		if num != exportTraceServiceRequestResourceSpans || wireType != protowire.BytesType {
			data = skipField(data, wireType)
			continue
		}
		msg, n := protowire.ConsumeBytes(data)
		data = data[n:]
		out = append(out, msg)
	}
	return out, nil
}

// parseResourceSpansToData parses one ResourceSpans message and returns resource attributes
// and scope-spans with raw span message bytes (no parsedSpan).
func parseResourceSpansToData(data []byte) (resourceAttrs map[string]string, scopeSpansList []scopeSpansData, err error) {
	resourceAttrs = make(map[string]string)
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case resourceSpansResource:
			if wireType != protowire.BytesType {
				data = skipField(data, wireType)
				continue
			}
			msg, n := protowire.ConsumeBytes(data)
			data = data[n:]
			attrs, err := parseResource(msg)
			if err != nil {
				return nil, nil, err
			}
			for k, v := range attrs {
				resourceAttrs[k] = v
			}
		case resourceSpansScopeSpans:
			if wireType != protowire.BytesType {
				data = skipField(data, wireType)
				continue
			}
			msg, n := protowire.ConsumeBytes(data)
			data = data[n:]
			scopeName, scopeVersion, spanBytes, err := parseScopeSpansToSpanBytes(msg)
			if err != nil {
				return nil, nil, err
			}
			scopeSpansList = append(scopeSpansList, scopeSpansData{
				ScopeName:    scopeName,
				ScopeVersion: scopeVersion,
				SpanBytes:    spanBytes,
			})
		default:
			data = skipField(data, wireType)
		}
	}
	return resourceAttrs, scopeSpansList, nil
}

// parseScopeSpansToSpanBytes parses one ScopeSpans message and returns scope name, version, and raw span message bytes.
func parseScopeSpansToSpanBytes(data []byte) (scopeName, scopeVersion string, spanBytes [][]byte, err error) {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case scopeSpansScope:
			if wireType != protowire.BytesType {
				data = skipField(data, wireType)
				continue
			}
			msg, n := protowire.ConsumeBytes(data)
			data = data[n:]
			scopeName, scopeVersion = parseInstrumentationScope(msg)
		case scopeSpansSpans:
			if wireType != protowire.BytesType {
				data = skipField(data, wireType)
				continue
			}
			msg, n := protowire.ConsumeBytes(data)
			data = data[n:]
			spanBytes = append(spanBytes, msg)
		default:
			data = skipField(data, wireType)
		}
	}
	return scopeName, scopeVersion, spanBytes, nil
}

func parseResource(data []byte) (map[string]string, error) {
	attrs := make(map[string]string)
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		if num != resourceAttributes || wireType != protowire.BytesType {
			data = skipField(data, wireType)
			continue
		}
		msg, n := protowire.ConsumeBytes(data)
		data = data[n:]
		k, v, err := parseKeyValue(msg)
		if err != nil {
			return nil, err
		}
		if k != "" {
			attrs[k] = v
		}
	}
	return attrs, nil
}

func parseInstrumentationScope(data []byte) (name, version string) {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case scopeName:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				name = string(v)
			}
		case scopeVersion:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				version = string(v)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	return name, version
}

// parseSpanEventAppendJSON parses one SpanEvent message and appends its JSON to w. Uses eventAttrsReuse
// (caller clears before first event per span). first is updated so a comma is written before subsequent events.
func parseSpanEventAppendJSON(data []byte, w *strings.Builder, first *bool, eventAttrsReuse map[string]interface{}) (eventName string, err error) {
	var timeUnixNano uint64
	var name string
	var dropped uint32
	for k := range eventAttrsReuse {
		delete(eventAttrsReuse, k)
	}
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case spanEventTimeUnixNano:
			if wireType == protowire.Fixed64Type {
				v, n := protowire.ConsumeFixed64(data)
				data = data[n:]
				timeUnixNano = v
			}
		case spanEventName:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				name = string(v)
			}
		case spanEventAttributes:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				k, v, _ := parseKeyValueTyped(msg)
				if k != "" {
					switch v.typ {
					case 1:
						eventAttrsReuse[k] = v.i
					case 2:
						eventAttrsReuse[k] = v.f
					case 3:
						eventAttrsReuse[k] = v.b
					default:
						eventAttrsReuse[k] = v.str
					}
				}
			}
		case spanEventDroppedAttributesCount:
			if wireType == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data)
				data = data[n:]
				dropped = uint32(v)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	if !*first {
		w.WriteString(",")
	}
	*first = false
	w.WriteString(`{"time_unix_nano":`)
	w.WriteString(strconv.FormatUint(timeUnixNano, 10))
	w.WriteString(`,"name":`)
	b, _ := json.Marshal(name)
	w.Write(b)
	if len(eventAttrsReuse) > 0 {
		w.WriteString(`,"attributes":`)
		b, _ = json.Marshal(eventAttrsReuse)
		w.Write(b)
	}
	if dropped != 0 {
		w.WriteString(`,"dropped_attributes_count":`)
		w.WriteString(strconv.FormatUint(uint64(dropped), 10))
	}
	w.WriteString("}")
	return name, nil
}

// parseSpanLinkAppendJSON parses one SpanLink message and appends its JSON to w. Uses linkAttrsReuse
// (caller clears before first link per span). first is updated for comma before subsequent links.
func parseSpanLinkAppendJSON(data []byte, w *strings.Builder, first *bool, linkAttrsReuse map[string]string) error {
	var linkTraceID [16]byte
	var linkSpanID [8]byte
	var traceState string
	var dropped uint32
	for k := range linkAttrsReuse {
		delete(linkAttrsReuse, k)
	}
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case spanLinkTraceID:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				copy(linkTraceID[:], v)
			}
		case spanLinkSpanID:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				copy(linkSpanID[:], v)
			}
		case spanLinkTraceState:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				traceState = string(v)
			}
		case spanLinkAttributes:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				k, v, _ := parseKeyValueTyped(msg)
				if k != "" {
					switch v.typ {
					case 1:
						linkAttrsReuse[k] = strconv.FormatInt(v.i, 10)
					case 2:
						linkAttrsReuse[k] = strconv.FormatFloat(v.f, 'g', -1, 64)
					case 3:
						linkAttrsReuse[k] = strconv.FormatBool(v.b)
					default:
						linkAttrsReuse[k] = v.str
					}
				}
			}
		case spanLinkDroppedAttributesCount:
			if wireType == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data)
				data = data[n:]
				dropped = uint32(v)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	if !*first {
		w.WriteString(",")
	}
	*first = false
	w.WriteString(`{"trace_id":"`)
	w.WriteString(hexEncodeBytes(linkTraceID[:]))
	w.WriteString(`","span_id":"`)
	w.WriteString(hexEncodeBytes(linkSpanID[:]))
	w.WriteString(`"`)
	if traceState != "" {
		w.WriteString(`,"tracestate":`)
		b, _ := json.Marshal(traceState)
		w.Write(b)
	}
	if len(linkAttrsReuse) > 0 {
		w.WriteString(`,"attributes":`)
		b, _ := json.Marshal(linkAttrsReuse)
		w.Write(b)
	}
	if dropped != 0 {
		w.WriteString(`,"dropped_attributes_count":`)
		w.WriteString(strconv.FormatUint(uint64(dropped), 10))
	}
	w.WriteString("}")
	return nil
}

func hexEncodeBytes(b []byte) string {
	const hexChars = "0123456789abcdef"
	buf := make([]byte, 0, len(b)*2)
	for _, c := range b {
		buf = append(buf, hexChars[c>>4], hexChars[c&0x0f])
	}
	return string(buf)
}

// Span Event: time_unix_nano=1, name=2, attributes=3, dropped_attributes_count=4
const (
	spanEventTimeUnixNano           = 1
	spanEventName                   = 2
	spanEventAttributes             = 3
	spanEventDroppedAttributesCount = 4
)

// Span Link: trace_id=1, span_id=2, trace_state=3, attributes=4, dropped_attributes_count=5
const (
	spanLinkTraceID                = 1
	spanLinkSpanID                 = 2
	spanLinkTraceState             = 3
	spanLinkAttributes             = 4
	spanLinkDroppedAttributesCount = 5
)

// Status: message=2, code=3
const statusMessage = 2

func parseStatus(data []byte) (code int32, message string) {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case statusCode:
			if wireType == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data)
				data = data[n:]
				code = int32(v)
			}
		case statusMessage:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				message = string(v)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	return code, message
}

func parseKeyValue(data []byte) (key, value string, err error) {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case keyValueKey:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				key = string(v)
			}
		case keyValueValue:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				value = parseAnyValueString(msg)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	return key, value, nil
}

// parseKeyValueTyped parses KeyValue and returns key + typed value for span attributes.
func parseKeyValueTyped(data []byte) (key string, v attrVal, err error) {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case keyValueKey:
			if wireType == protowire.BytesType {
				b, n := protowire.ConsumeBytes(data)
				data = data[n:]
				key = string(b)
			}
		case keyValueValue:
			if wireType == protowire.BytesType {
				msg, n := protowire.ConsumeBytes(data)
				data = data[n:]
				v = parseAnyValueTyped(msg)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	return key, v, nil
}

func parseAnyValueTyped(data []byte) (v attrVal) {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case anyValueString:
			if wireType == protowire.BytesType {
				b, n := protowire.ConsumeBytes(data)
				data = data[n:]
				v.str = string(b)
				v.typ = 0
				return v
			}
		case anyValueBool:
			if wireType == protowire.VarintType {
				b, n := protowire.ConsumeVarint(data)
				data = data[n:]
				v.b = b != 0
				v.typ = 3
				return v
			}
		case anyValueInt:
			if wireType == protowire.VarintType {
				i, n := protowire.ConsumeVarint(data)
				data = data[n:]
				v.i = int64(i)
				v.typ = 1
				return v
			}
		case anyValueDouble:
			if wireType == protowire.Fixed64Type {
				f, n := protowire.ConsumeFixed64(data)
				data = data[n:]
				v.f = math.Float64frombits(f)
				v.typ = 2
				return v
			}
		case anyValueBytes:
			if wireType == protowire.BytesType {
				b, n := protowire.ConsumeBytes(data)
				data = data[n:]
				v.str = string(b)
				v.typ = 0
				return v
			}
		default:
			data = skipField(data, wireType)
		}
	}
	return v
}

// parseAnyValueString extracts a string representation from an AnyValue message.
// It handles string_value(1), bool_value(2), int_value(3), double_value(4), bytes_value(7).
func parseAnyValueString(data []byte) string {
	for len(data) > 0 {
		num, wireType, n := protowire.ConsumeTag(data)
		data = data[n:]
		switch num {
		case anyValueString:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				return string(v)
			}
		case anyValueBool:
			if wireType == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data)
				data = data[n:]
				if v != 0 {
					return "true"
				}
				return "false"
			}
		case anyValueInt:
			if wireType == protowire.VarintType {
				v, n := protowire.ConsumeVarint(data)
				data = data[n:]
				return fmt.Sprintf("%d", int64(v))
			}
		case anyValueDouble:
			if wireType == protowire.Fixed64Type {
				v, n := protowire.ConsumeFixed64(data)
				data = data[n:]
				return fmt.Sprintf("%g", math.Float64frombits(v))
			}
		case anyValueBytes:
			if wireType == protowire.BytesType {
				v, n := protowire.ConsumeBytes(data)
				data = data[n:]
				return string(v)
			}
		default:
			data = skipField(data, wireType)
		}
	}
	return ""
}

func skipField(data []byte, wireType protowire.Type) []byte {
	switch wireType {
	case protowire.VarintType:
		_, n := protowire.ConsumeVarint(data)
		return data[n:]
	case protowire.Fixed32Type:
		_, n := protowire.ConsumeFixed32(data)
		return data[n:]
	case protowire.Fixed64Type:
		_, n := protowire.ConsumeFixed64(data)
		return data[n:]
	case protowire.BytesType:
		_, n := protowire.ConsumeBytes(data)
		return data[n:]
	default:
		return data
	}
}

// processRequestRaw processes raw OTLP ExportTraceServiceRequest bytes using the custom
// deserializer: parses incrementally and builds pb.TracerPayload directly (no pdata, no ReceiveResourceSpans).
func (o *OTLPReceiver) processRequestRaw(ctx context.Context, header http.Header, data []byte) error {
	resourceSpansBytes, err := parseOTLPExportTraceServiceRequestRaw(data)
	if err != nil {
		return err
	}
	var spanCount int64
	for _, rsBytes := range resourceSpansBytes {
		n, err := o.buildPayloadFromResourceSpansBytes(ctx, header, rsBytes)
		if err != nil {
			return err
		}
		spanCount += n
	}
	OTLPIngestAgentTracesRequests.Inc()
	OTLPIngestAgentTracesEvents.Add(float64(spanCount))
	return nil
}

// otlpExportRequestToRawBytes returns the protobuf bytes for the given ExportRequest.
// Used by tests to obtain the same bytes that would be sent to ExportTracesRaw.
func otlpExportRequestToRawBytes(req ptraceotlp.ExportRequest) ([]byte, error) {
	return req.MarshalProto()
}
