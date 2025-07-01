// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"encoding/binary"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// convertToIdx converts a TracerPayload to the new string indexed TracerPayload format
func convertToIdx(payload *pb.TracerPayload) *idx.TracerPayload {
	internedStrings := newInternedStrings()
	payloadAttrs := convertAttributesMap(payload.Tags, internedStrings)
	idxChunks := make([]*idx.TraceChunk, len(payload.Chunks))
	tracerRuntimeID := ""
	for chunkIndex, chunk := range payload.Chunks {
		// We MUST have at least one span to be sending this chunk
		// But let's check anyway to avoid a panic, we'll default to nil otherwise
		var traceID []byte
		if len(chunk.Spans) >= 0 {
			var err error
			traceID, err = chunk.Spans[0].Get128BitTraceID()
			if err != nil {
				log.Errorf("Unable to convert trace to idx, failed to determine 128-bit trace ID from incoming span defaulting to nil: %v", err)
			}
		}
		chunkAttrs := convertAttributesMap(chunk.Tags, internedStrings)
		idxSpans := make([]*idx.Span, len(chunk.Spans))
		for spanIndex, span := range chunk.Spans {
			spanAttrs := make(map[uint32]*idx.AnyValue, len(span.Meta)+len(span.Metrics)+len(span.MetaStruct))
			for k, v := range span.Meta {
				spanAttrs[internedStrings.addString(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: internedStrings.addString(v),
					},
				}
			}
			for k, v := range span.Metrics {
				spanAttrs[internedStrings.addString(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_DoubleValue{
						DoubleValue: v,
					},
				}
			}
			for k, v := range span.MetaStruct {
				spanAttrs[internedStrings.addString(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_BytesValue{
						BytesValue: v,
					},
				}
			}
			spanLinks := make([]*idx.SpanLink, len(span.SpanLinks))
			for spanLinkIndex, link := range span.SpanLinks {
				linkTraceID := make([]byte, 16)
				binary.BigEndian.PutUint64(linkTraceID[8:], link.TraceID)
				binary.BigEndian.PutUint64(linkTraceID[:8], link.TraceIDHigh)
				spanLinks[spanLinkIndex] = &idx.SpanLink{
					TraceID:       linkTraceID,
					SpanID:        link.SpanID,
					TracestateRef: internedStrings.addString(link.Tracestate),
					FlagsRef:      link.Flags,
					Attributes:    convertAttributesMap(link.Attributes, internedStrings),
				}
			}
			spanEvents := make([]*idx.SpanEvent, len(span.SpanEvents))
			for spanEventIndex, event := range span.SpanEvents {
				spanEvents[spanEventIndex] = &idx.SpanEvent{
					Time:       uint64(event.TimeUnixNano),
					NameRef:    internedStrings.addString(event.Name),
					Attributes: convertSpanEventAttributes(event.Attributes, internedStrings),
				}
			}
			env := span.Meta["env"]
			version := span.Meta["version"]
			component := span.Meta["component"]
			kindStr := span.Meta["kind"]
			// We don't actually copy the tracer runtime ID into the payload field for v04 payloads
			if tracerRuntimeID == "" && span.Meta["runtime-id"] != "" {
				tracerRuntimeID = span.Meta["runtime-id"]
			}
			var kind idx.SpanKind
			switch kindStr {
			case "server":
				kind = idx.SpanKind_SPAN_KIND_SERVER
			case "client":
				kind = idx.SpanKind_SPAN_KIND_CLIENT
			case "producer":
				kind = idx.SpanKind_SPAN_KIND_PRODUCER
			case "consumer":
				kind = idx.SpanKind_SPAN_KIND_CONSUMER
			case "internal":
				kind = idx.SpanKind_SPAN_KIND_INTERNAL
			default:
				kind = idx.SpanKind_SPAN_KIND_INTERNAL // OTEL spec says default should be internal
			}
			idxSpans[spanIndex] = &idx.Span{
				ServiceRef:   internedStrings.addString(span.Service),
				NameRef:      internedStrings.addString(span.Name),
				ResourceRef:  internedStrings.addString(span.Resource),
				TypeRef:      internedStrings.addString(span.Type),
				SpanID:       span.SpanID,
				ParentID:     span.ParentID,
				Start:        uint64(span.Start),
				Duration:     uint64(span.Duration),
				Error:        span.Error > 0,
				Attributes:   spanAttrs,
				EnvRef:       internedStrings.addString(env),
				VersionRef:   internedStrings.addString(version),
				ComponentRef: internedStrings.addString(component),
				Kind:         kind,
				SpanLinks:    spanLinks,
				SpanEvents:   spanEvents,
			}
		}
		decisionMaker := chunk.Tags["_dd.p.dm"]
		idxChunks[chunkIndex] = &idx.TraceChunk{
			Priority:         int32(chunk.Priority),
			OriginRef:        internedStrings.addString(chunk.Origin),
			Attributes:       chunkAttrs,
			TraceID:          traceID,
			Spans:            idxSpans,
			DroppedTrace:     chunk.DroppedTrace,
			DecisionMakerRef: internedStrings.addString(decisionMaker),
		}
	}
	if payload.RuntimeID != "" {
		tracerRuntimeID = payload.RuntimeID
	}
	idxPayload := &idx.TracerPayload{
		ContainerIDRef:     internedStrings.addString(payload.ContainerID),
		LanguageNameRef:    internedStrings.addString(payload.LanguageName),
		LanguageVersionRef: internedStrings.addString(payload.LanguageVersion),
		TracerVersionRef:   internedStrings.addString(payload.TracerVersion),
		RuntimeIDRef:       internedStrings.addString(tracerRuntimeID),
		EnvRef:             internedStrings.addString(payload.Env),
		HostnameRef:        internedStrings.addString(payload.Hostname),
		VersionRef:         internedStrings.addString(payload.AppVersion),
		Attributes:         payloadAttrs,
		Chunks:             idxChunks,
	}
	idxPayload.Strings = internedStrings.strings() // Set last to ensure all strings are interned
	return idxPayload
}

func convertSpanEventAttributes(attrs map[string]*pb.AttributeAnyValue, internedStrings *internedstrings) map[uint32]*idx.AnyValue {
	spanEventAttrs := make(map[uint32]*idx.AnyValue, len(attrs))
	for k, v := range attrs {
		switch v.Type {
		case pb.AttributeAnyValue_STRING_VALUE:
			spanEventAttrs[internedStrings.addString(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: internedStrings.addString(v.StringValue),
				},
			}
		case pb.AttributeAnyValue_BOOL_VALUE:
			spanEventAttrs[internedStrings.addString(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_BoolValue{
					BoolValue: v.BoolValue,
				},
			}
		case pb.AttributeAnyValue_INT_VALUE:
			spanEventAttrs[internedStrings.addString(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_IntValue{
					IntValue: v.IntValue,
				},
			}
		case pb.AttributeAnyValue_DOUBLE_VALUE:
			spanEventAttrs[internedStrings.addString(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_DoubleValue{
					DoubleValue: v.DoubleValue,
				},
			}
		case pb.AttributeAnyValue_ARRAY_VALUE:
			spanEventAttrs[internedStrings.addString(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_ArrayValue{
					ArrayValue: convertArrayValue(v.ArrayValue, internedStrings),
				},
			}
		default:
			log.Errorf("Unknown attribute type: %d", v.Type)
		}
	}
	return spanEventAttrs
}

func convertArrayValue(arrayValue *pb.AttributeArray, internedStrings *internedstrings) *idx.ArrayValue {
	values := make([]*idx.AnyValue, len(arrayValue.Values))
	for i, value := range arrayValue.Values {
		values[i] = convertAttributeArrayValue(value, internedStrings)
	}
	return &idx.ArrayValue{
		Values: values,
	}
}

func convertAttributeArrayValue(arrayValue *pb.AttributeArrayValue, internedStrings *internedstrings) *idx.AnyValue {
	switch arrayValue.Type {
	case pb.AttributeArrayValue_STRING_VALUE:
		return &idx.AnyValue{
			Value: &idx.AnyValue_StringValueRef{
				StringValueRef: internedStrings.addString(arrayValue.StringValue),
			},
		}
	case pb.AttributeArrayValue_BOOL_VALUE:
		return &idx.AnyValue{
			Value: &idx.AnyValue_BoolValue{
				BoolValue: arrayValue.BoolValue,
			},
		}
	case pb.AttributeArrayValue_INT_VALUE:
		return &idx.AnyValue{
			Value: &idx.AnyValue_IntValue{
				IntValue: arrayValue.IntValue,
			},
		}
	case pb.AttributeArrayValue_DOUBLE_VALUE:
		return &idx.AnyValue{
			Value: &idx.AnyValue_DoubleValue{
				DoubleValue: arrayValue.DoubleValue,
			},
		}
	default:
		log.Errorf("Unknown attribute array value type: %d", arrayValue.Type)
	}
	return nil
}

func convertAttributesMap(attrs map[string]string, internedStrings *internedstrings) map[uint32]*idx.AnyValue {
	linkAttrs := make(map[uint32]*idx.AnyValue, len(attrs))
	for k, v := range attrs {
		linkAttrs[internedStrings.addString(k)] = &idx.AnyValue{
			Value: &idx.AnyValue_StringValueRef{
				StringValueRef: internedStrings.addString(v),
			},
		}
	}
	return linkAttrs
}

type internedstrings struct {
	strMap map[string]*istring
}

type istring struct {
	idx           uint32
	serializedIdx uint32 // During serialization set this index, 0 if not serialized yet
}

func newInternedStrings() *internedstrings {
	return &internedstrings{
		strMap: map[string]*istring{"": {
			idx:           0,
			serializedIdx: 0,
		}},
	}
}

// addString s if new, return index in array where it is
func (is *internedstrings) addString(s string) uint32 {
	if i, ok := is.strMap[s]; ok {
		return i.idx
	}
	is.strMap[s] = &istring{
		idx:           uint32(len(is.strMap)),
		serializedIdx: 0,
	}
	return is.strMap[s].idx
}

// strings returns the list of strings interned at their appropriate location
func (is *internedstrings) strings() []string {
	strs := make([]string, len(is.strMap))
	for s, sIndex := range is.strMap {
		strs[sIndex.idx] = s
	}
	return strs
}
