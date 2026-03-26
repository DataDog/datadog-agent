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
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// isPromotedTag returns true if the key is a promoted tag that should be set as a field instead of an attribute
func isPromotedTag(key string) bool {
	return key == "env" || key == "version" || key == "component" || key == "span.kind"
}

// ConvertToIdx converts a TracerPayload to the new string indexed TracerPayload format
// originPayloadVersion is the version of the original payload, this is used to set the _dd.convertedv1 attribute on the spans (for debugging purposes)
func ConvertToIdx(payload *pb.TracerPayload, originPayloadVersion string) *idx.InternalTracerPayload {
	stringTable := idx.NewStringTable()
	payloadAttrs := convertAttributesMap(payload.Tags, stringTable)
	idxChunks := make([]*idx.InternalTraceChunk, len(payload.Chunks))
	chunkConvertedFields := idx.ChunkConvertedFields{}
	for chunkIndex, chunk := range payload.Chunks {
		if chunk == nil || len(chunk.Spans) == 0 {
			continue
		}
		spanConvertedFields := idx.NewSpanConvertedFields()
		tidUpper, tidLower, err := chunk.Spans[0].Get128BitTraceID()
		if err != nil {
			log.Errorf("Failed to determine full 128-bit trace ID from incoming span: %v. Resulting trace chunk(%d) will be missing upper 64 bits of the trace ID.", err, tidLower)
		}
		spanConvertedFields.TraceIDUpper = tidUpper
		spanConvertedFields.TraceIDLower = tidLower
		chunkAttrs := convertAttributesMap(chunk.Tags, stringTable)
		idxSpans := make([]*idx.InternalSpan, len(chunk.Spans))
		for spanIndex, span := range chunk.Spans {
			spanAttrs := make(map[uint32]*idx.AnyValue, len(span.Meta)+len(span.Metrics)+len(span.MetaStruct))
			for k, v := range span.Meta {
				if isPromotedTag(k) {
					continue
				}
				spanAttrs[stringTable.Add(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: stringTable.Add(v),
					},
				}
			}
			for k, v := range span.Metrics {
				if isPromotedTag(k) {
					continue
				}
				if k == "_sampling_priority_v1" {
					spanConvertedFields.SamplingPriority = int32(v)
				}
				spanAttrs[stringTable.Add(k)] = &idx.AnyValue{
					Value: &idx.AnyValue_DoubleValue{
						DoubleValue: v,
					},
				}
			}
			for k, v := range span.MetaStruct {
				if isPromotedTag(k) {
					continue
				}
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

			// Each span gets its own env/version based on its meta, but we also promote
			// the first occurrence to chunk/payload level via spanConvertedFields
			var spanEnvRef, spanVersionRef uint32
			if env, ok := span.Meta["env"]; ok && env != "" {
				spanEnvRef = stringTable.Add(env)
				if spanConvertedFields.EnvRef == 0 {
					spanConvertedFields.EnvRef = spanEnvRef
				}
			}
			if spanHost, ok := span.Meta["_dd.hostname"]; ok && spanConvertedFields.HostnameRef == 0 {
				spanConvertedFields.HostnameRef = stringTable.Add(spanHost)
			}
			if spanVersion, ok := span.Meta["version"]; ok && spanVersion != "" {
				spanVersionRef = stringTable.Add(spanVersion)
				if spanConvertedFields.AppVersionRef == 0 {
					spanConvertedFields.AppVersionRef = spanVersionRef
				}
			}
			if spanGitCommitSha, ok := span.Meta["_dd.git.commit.sha"]; ok && spanConvertedFields.GitCommitShaRef == 0 {

				spanConvertedFields.GitCommitShaRef = stringTable.Add(spanGitCommitSha)
			}
			if spanDecisionMaker, ok := span.Meta["_dd.p.dm"]; ok && spanConvertedFields.SamplingMechanism == 0 {
				spanDecisionMaker, _ = strings.CutPrefix(spanDecisionMaker, "-")
				samplingMechanism, err := strconv.ParseUint(spanDecisionMaker, 10, 32)
				if err != nil {
					log.Debugf("Found invalid sampling mechanism %s: %v, Decision maker will be ignored", spanDecisionMaker, err)
				}
				spanConvertedFields.SamplingMechanism = uint32(samplingMechanism)
			}
			if spanAPMMode, ok := span.Meta["_dd.apm_mode"]; ok && spanConvertedFields.APMModeRef == 0 {
				spanConvertedFields.APMModeRef = stringTable.Add(spanAPMMode)
			}
			if spanOrigin, ok := span.Meta["_dd.origin"]; ok && spanConvertedFields.OriginRef == 0 {
				spanConvertedFields.OriginRef = stringTable.Add(spanOrigin)
			}
			component := span.Meta["component"]
			kindStr := span.Meta["span.kind"]
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
				EnvRef:       spanEnvRef,
				VersionRef:   spanVersionRef,
				ComponentRef: stringTable.Add(component),
				Kind:         kind,
				Links:        spanLinks,
				Events:       spanEvents,
			}
			idxSpans[spanIndex] = idx.NewInternalSpan(stringTable, protoSpan)
			if originPayloadVersion != "" {
				idxSpans[spanIndex].SetStringAttribute("_dd.convertedv1", originPayloadVersion)
			}
		}
		idxChunks[chunkIndex] = &idx.InternalTraceChunk{
			Strings:      stringTable,
			Attributes:   chunkAttrs,
			Spans:        idxSpans,
			DroppedTrace: chunk.DroppedTrace,
		}
		idxChunks[chunkIndex].SetOrigin(chunk.Origin)
		idxChunks[chunkIndex].ApplyPromotedFields(spanConvertedFields, &chunkConvertedFields)
		if chunk.Priority != int32(sampler.PriorityNone) && idxChunks[chunkIndex].Priority == int32(sampler.PriorityNone) {
			// If the chunk has a priority set and none on any internal span then use the chunk's priority
			idxChunks[chunkIndex].Priority = chunk.Priority
		}
		if chunkDm, ok := idxChunks[chunkIndex].GetAttributeAsString("_dd.p.dm"); ok && idxChunks[chunkIndex].SamplingMechanism() == 0 {
			chunkDm, _ = strings.CutPrefix(chunkDm, "-")
			samplingMechanism, err := strconv.ParseUint(chunkDm, 10, 32)
			if err != nil {
				log.Debugf("Found invalid sampling mechanism %s: %v, Decision maker will be ignored", chunkDm, err)
			}
			idxChunks[chunkIndex].SetSamplingMechanism(uint32(samplingMechanism))
		}
	}
	idxPayload := &idx.InternalTracerPayload{
		Strings:    stringTable,
		Attributes: payloadAttrs,
		Chunks:     idxChunks,
	}
	idxPayload.SetEnv(payload.Env)
	idxPayload.SetHostname(payload.Hostname)
	idxPayload.SetAppVersion(payload.AppVersion)
	idxPayload.SetContainerID(payload.ContainerID)
	idxPayload.SetLanguageName(payload.LanguageName)
	idxPayload.SetLanguageVersion(payload.LanguageVersion)
	idxPayload.SetTracerVersion(payload.TracerVersion)
	idxPayload.SetRuntimeID(payload.RuntimeID)
	idxPayload.ApplyPromotedFields(&chunkConvertedFields)
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
