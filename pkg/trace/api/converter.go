// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/binary"
	"strconv"
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// convertToIdx converts a TracerPayload to the new string indexed TracerPayload format
func convertToIdx(payload *pb.TracerPayload) *idx.InternalTracerPayload {
	stringTable := idx.NewStringTable()
	payloadAttrs := convertAttributesMap(payload.Tags, stringTable)
	idxChunks := make([]*idx.InternalTraceChunk, len(payload.Chunks))
	for chunkIndex, chunk := range payload.Chunks {
		if chunk == nil || len(chunk.Spans) == 0 {
			continue
		}
		var traceID []byte
		var err error
		traceID, err = chunk.Spans[0].Get128BitTraceID()
		if err != nil {
			log.Errorf("Unable to convert trace to idx, failed to determine 128-bit trace ID from incoming span defaulting to nil: %v", err)
		}
		chunkAttrs := convertAttributesMap(chunk.Tags, stringTable)
		idxSpans := make([]*idx.InternalSpan, len(chunk.Spans))
		for spanIndex, span := range chunk.Spans {
			spanAttrs := make(map[uint32]*idx.AnyValue, len(span.Meta)+len(span.Metrics)+len(span.MetaStruct))
			for k, v := range span.Meta {
				spanAttrs[stringTable.Add(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: stringTable.Add(v),
					},
				}
			}
			for k, v := range span.Metrics {
				spanAttrs[stringTable.Add(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_DoubleValue{
						DoubleValue: v,
					},
				}
			}
			for k, v := range span.MetaStruct {
				spanAttrs[stringTable.Add(k)] = &idx.AnyValue{
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
					TracestateRef: stringTable.Add(link.Tracestate),
					Flags:         link.Flags,
					Attributes:    convertAttributesMap(link.Attributes, stringTable),
				}
			}
			spanEvents := make([]*idx.SpanEvent, len(span.SpanEvents))
			for spanEventIndex, event := range span.SpanEvents {
				spanEvents[spanEventIndex] = &idx.SpanEvent{
					Time:       uint64(event.TimeUnixNano),
					NameRef:    stringTable.Add(event.Name),
					Attributes: convertSpanEventAttributes(event.Attributes, stringTable),
				}
			}
			env := span.Meta["env"]
			version := span.Meta["version"]
			component := span.Meta["component"]
			kindStr := span.Meta["kind"]
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
			protoSpan := &idx.Span{
				ServiceRef:   stringTable.Add(span.Service),
				NameRef:      stringTable.Add(span.Name),
				ResourceRef:  stringTable.Add(span.Resource),
				TypeRef:      stringTable.Add(span.Type),
				SpanID:       span.SpanID,
				ParentID:     span.ParentID,
				Start:        uint64(span.Start),
				Duration:     uint64(span.Duration),
				Error:        span.Error > 0,
				Attributes:   spanAttrs,
				EnvRef:       stringTable.Add(env),
				VersionRef:   stringTable.Add(version),
				ComponentRef: stringTable.Add(component),
				Kind:         kind,
				Links:        spanLinks,
				Events:       spanEvents,
			}
			idxSpans[spanIndex] = idx.NewInternalSpan(stringTable, protoSpan)
		}
		var samplingMechanism uint64
		decisionMaker := chunk.Tags["_dd.p.dm"]
		if decisionMaker != "" {
			decisionMaker, _ = strings.CutPrefix(decisionMaker, "-")
			var err error
			samplingMechanism, err = strconv.ParseUint(decisionMaker, 10, 32)
			if err != nil {
				log.Warnf("Found invalid sampling mechanism %s: %v, Decision maker will be ignored", decisionMaker, err)
			}
		}
		idxChunks[chunkIndex] = &idx.InternalTraceChunk{
			Strings:      stringTable,
			Priority:     int32(chunk.Priority),
			Attributes:   chunkAttrs,
			Spans:        idxSpans,
			DroppedTrace: chunk.DroppedTrace,
			TraceID:      traceID,
		}
		idxChunks[chunkIndex].SetOrigin(chunk.Origin)
		idxChunks[chunkIndex].SetSamplingMechanism(uint32(samplingMechanism))
	}
	idxPayload := &idx.InternalTracerPayload{
		Strings:    stringTable,
		Attributes: payloadAttrs,
		Chunks:     idxChunks,
	}
	idxPayload.SetContainerID(payload.ContainerID)
	idxPayload.SetLanguageName(payload.LanguageName)
	idxPayload.SetLanguageVersion(payload.LanguageVersion)
	idxPayload.SetTracerVersion(payload.TracerVersion)
	idxPayload.SetRuntimeID(payload.RuntimeID)
	idxPayload.SetEnv(payload.Env)
	idxPayload.SetHostname(payload.Hostname)
	idxPayload.SetAppVersion(payload.AppVersion)
	return idxPayload
}

func convertSpanEventAttributes(attrs map[string]*pb.AttributeAnyValue, stringTable *idx.StringTable) map[uint32]*idx.AnyValue {
	spanEventAttrs := make(map[uint32]*idx.AnyValue, len(attrs))
	for k, v := range attrs {
		switch v.Type {
		case pb.AttributeAnyValue_STRING_VALUE:
			spanEventAttrs[stringTable.Add(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: stringTable.Add(v.StringValue),
				},
			}
		case pb.AttributeAnyValue_BOOL_VALUE:
			spanEventAttrs[stringTable.Add(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_BoolValue{
					BoolValue: v.BoolValue,
				},
			}
		case pb.AttributeAnyValue_INT_VALUE:
			spanEventAttrs[stringTable.Add(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_IntValue{
					IntValue: v.IntValue,
				},
			}
		case pb.AttributeAnyValue_DOUBLE_VALUE:
			spanEventAttrs[stringTable.Add(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_DoubleValue{
					DoubleValue: v.DoubleValue,
				},
			}
		case pb.AttributeAnyValue_ARRAY_VALUE:
			spanEventAttrs[stringTable.Add(k)] = &idx.AnyValue{
				Value: &idx.AnyValue_ArrayValue{
					ArrayValue: convertArrayValue(v.ArrayValue, stringTable),
				},
			}
		default:
			log.Errorf("Unknown attribute type: %d", v.Type)
		}
	}
	return spanEventAttrs
}

func convertArrayValue(arrayValue *pb.AttributeArray, stringTable *idx.StringTable) *idx.ArrayValue {
	values := make([]*idx.AnyValue, len(arrayValue.Values))
	for i, value := range arrayValue.Values {
		values[i] = convertAttributeArrayValue(value, stringTable)
	}
	return &idx.ArrayValue{
		Values: values,
	}
}

func convertAttributeArrayValue(arrayValue *pb.AttributeArrayValue, stringTable *idx.StringTable) *idx.AnyValue {
	switch arrayValue.Type {
	case pb.AttributeArrayValue_STRING_VALUE:
		return &idx.AnyValue{
			Value: &idx.AnyValue_StringValueRef{
				StringValueRef: stringTable.Add(arrayValue.StringValue),
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

func convertAttributesMap(attrs map[string]string, stringTable *idx.StringTable) map[uint32]*idx.AnyValue {
	linkAttrs := make(map[uint32]*idx.AnyValue, len(attrs))
	for k, v := range attrs {
		linkAttrs[stringTable.Add(k)] = &idx.AnyValue{
			Value: &idx.AnyValue_StringValueRef{
				StringValueRef: stringTable.Add(v),
			},
		}
	}
	return linkAttrs
}
