// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv117 "go.opentelemetry.io/collector/semconv/v1.17.0"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.opentelemetry.io/otel/attribute"
)

// Util functions for converting OTel semantics to DD semantics.
// TODO(OTEL-1726): reuse the same mapping code for ReceiveResourceSpans and Concentrator

var (
	// SignalTypeSet is the OTel attribute set for traces.
	SignalTypeSet = attribute.NewSet(attribute.String("signal", "traces"))
)

// IndexOTelSpans iterates over the input OTel spans and returns 3 maps:
// OTel spans indexed by span ID, OTel resources indexed by span ID, OTel instrumentation scopes indexed by span ID.
// Skips spans with invalid trace ID or span ID. If there are multiple spans with the same (non-zero) span ID, the last one wins.
func IndexOTelSpans(traces ptrace.Traces) (map[pcommon.SpanID]ptrace.Span, map[pcommon.SpanID]pcommon.Resource, map[pcommon.SpanID]pcommon.InstrumentationScope) {
	spanByID := make(map[pcommon.SpanID]ptrace.Span)
	resByID := make(map[pcommon.SpanID]pcommon.Resource)
	scopeByID := make(map[pcommon.SpanID]pcommon.InstrumentationScope)
	rspanss := traces.ResourceSpans()
	for i := 0; i < rspanss.Len(); i++ {
		rspans := rspanss.At(i)
		res := rspans.Resource()
		for j := 0; j < rspans.ScopeSpans().Len(); j++ {
			libspans := rspans.ScopeSpans().At(j)
			for k := 0; k < libspans.Spans().Len(); k++ {
				span := libspans.Spans().At(k)
				if span.TraceID().IsEmpty() || span.SpanID().IsEmpty() {
					continue
				}
				spanByID[span.SpanID()] = span
				resByID[span.SpanID()] = res
				scopeByID[span.SpanID()] = libspans.Scope()
			}
		}
	}
	return spanByID, resByID, scopeByID
}

// GetTopLevelOTelSpans returns the span IDs of the top level OTel spans.
func GetTopLevelOTelSpans(spanByID map[pcommon.SpanID]ptrace.Span, resByID map[pcommon.SpanID]pcommon.Resource, topLevelByKind bool) map[pcommon.SpanID]struct{} {
	topLevelSpans := make(map[pcommon.SpanID]struct{})
	for spanID, span := range spanByID {
		if span.ParentSpanID().IsEmpty() {
			// case 1: root span
			topLevelSpans[spanID] = struct{}{}
			continue
		}

		if topLevelByKind {
			// New behavior for computing top level OTel spans, see computeTopLevelAndMeasured in pkg/trace/api/otlp.go
			spanKind := span.Kind()
			if spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer {
				// span is a server-side span, mark as top level
				topLevelSpans[spanID] = struct{}{}
			}
			continue
		}

		// Otherwise, fall back to old behavior in ComputeTopLevel
		parentSpan, ok := spanByID[span.ParentSpanID()]
		if !ok {
			// case 2: parent span not in the same chunk, presumably it belongs to another service
			topLevelSpans[spanID] = struct{}{}
			continue
		}

		svc := GetOTelService(span, resByID[spanID], true)
		parentSvc := GetOTelService(parentSpan, resByID[parentSpan.SpanID()], true)
		if svc != parentSvc {
			// case 3: parent is not in the same service
			topLevelSpans[spanID] = struct{}{}
		}
	}
	return topLevelSpans
}

// GetOTelAttrVal returns the matched value as a string in the input map with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrVal(attrs pcommon.Map, normalize bool, keys ...string) string {
	val := ""
	for _, key := range keys {
		attrval, exists := attrs.Get(key)
		if exists {
			val = attrval.AsString()
		}
	}

	if normalize {
		val = NormalizeTagValue(val)
	}

	return val
}

// GetOTelAttrValInResAndSpanAttrs returns the matched value as a string in the OTel resource attributes and span attributes with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If the key is present in both resource attributes and span attributes, resource attributes take precedence.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrValInResAndSpanAttrs(span ptrace.Span, res pcommon.Resource, normalize bool, keys ...string) string {
	if val := GetOTelAttrVal(res.Attributes(), normalize, keys...); val != "" {
		return val
	}
	return GetOTelAttrVal(span.Attributes(), normalize, keys...)
}

// GetOTelSpanType returns the DD span type based on OTel span kind and attributes.
func GetOTelSpanType(span ptrace.Span, res pcommon.Resource) string {
	var typ string
	switch span.Kind() {
	case ptrace.SpanKindServer:
		typ = "web"
	case ptrace.SpanKindClient:
		db := GetOTelAttrValInResAndSpanAttrs(span, res, true, semconv.AttributeDBSystem)
		if db == "redis" || db == "memcached" {
			typ = "cache"
		} else if db != "" {
			typ = "db"
		} else {
			typ = "http"
		}
	default:
		typ = "custom"
	}
	return typ
}

// GetOTelService returns the DD service name based on OTel span and resource attributes.
func GetOTelService(span ptrace.Span, res pcommon.Resource, normalize bool) string {
	// No need to normalize with NormalizeTagValue since we will do NormalizeService later
	svc := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeServiceName)
	if svc == "" {
		svc = "otlpresourcenoservicename"
	}
	if normalize {
		newsvc, err := NormalizeService(svc, "")
		switch err {
		case ErrTooLong:
			log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", MaxServiceLen, svc)
		case ErrInvalid:
			log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s", svc, newsvc)
		}
		svc = newsvc
	}
	return svc
}

// GetOTelResource returns the DD resource name based on OTel span and resource attributes.
func GetOTelResource(span ptrace.Span, res pcommon.Resource) (resName string) {
	resName = GetOTelAttrValInResAndSpanAttrs(span, res, false, "resource.name")
	if resName == "" {
		if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeHTTPMethod); m != "" {
			// use the HTTP method + route (if available)
			resName = m
			if route := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeHTTPRoute); route != "" {
				resName = resName + " " + route
			}
		} else if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeMessagingOperation); m != "" {
			resName = m
			// use the messaging operation
			if dest := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeMessagingDestination, semconv117.AttributeMessagingDestinationName); dest != "" {
				resName = resName + " " + dest
			}
		} else if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeRPCMethod); m != "" {
			resName = m
			// use the RPC method
			if svc := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv.AttributeRPCService); m != "" {
				// ...and service if available
				resName = resName + " " + svc
			}
		} else {
			resName = span.Name()
		}
	}
	if len(resName) > MaxResourceLen {
		resName = resName[:MaxResourceLen]
	}
	return
}

// GetOTelOperationName returns the DD operation name based on OTel span and resource attributes and given configs.
func GetOTelOperationName(
	span ptrace.Span,
	res pcommon.Resource,
	lib pcommon.InstrumentationScope,
	spanNameAsResourceName bool,
	spanNameRemappings map[string]string,
	normalize bool) string {
	// No need to normalize with NormalizeTagValue since we will do NormalizeName later
	name := GetOTelAttrValInResAndSpanAttrs(span, res, false, "operation.name")
	if name == "" {
		if spanNameAsResourceName {
			name = span.Name()
		} else {
			name = strings.ToLower(span.Kind().String())
			if lib.Name() != "" {
				name = lib.Name() + "." + name
			} else {
				name = "opentelemetry." + name
			}
		}
	}
	if v, ok := spanNameRemappings[name]; ok {
		name = v
	}

	if normalize {
		normalizeName, err := NormalizeName(name)
		switch err {
		case ErrEmpty:
			log.Debugf("Fixing malformed trace. Name is empty (reason:span_name_empty), setting span.name=%s", normalizeName)
		case ErrTooLong:
			log.Debugf("Fixing malformed trace. Name is too long (reason:span_name_truncate), truncating span.name to length=%d", MaxServiceLen)
		case ErrInvalid:
			log.Debugf("Fixing malformed trace. Name is invalid (reason:span_name_invalid), setting span.name=%s", normalizeName)
		}
		name = normalizeName
	}

	return name
}

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes.
func GetOTelHostname(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator, fallbackHost string) string {
	ctx := context.Background()
	src, srcok := tr.ResourceToSource(ctx, res, SignalTypeSet)
	if !srcok {
		if v := GetOTelAttrValInResAndSpanAttrs(span, res, false, "_dd.hostname"); v != "" {
			src = source.Source{Kind: source.HostnameKind, Identifier: v}
			srcok = true
		}
	}
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			return src.Identifier
		default:
			// We are not on a hostname (serverless), hence the hostname is empty
			return ""
		}
	} else {
		// fallback hostname from Agent conf.Hostname
		return fallbackHost
	}
}

// GetOTelStatusCode returns the DD status code of the OTel span.
func GetOTelStatusCode(span ptrace.Span) uint32 {
	if code, ok := span.Attributes().Get(semconv.AttributeHTTPStatusCode); ok {
		return uint32(code.Int())
	}
	return 0
}

// GetOTelContainerTags returns a list of DD container tags in the OTel resource attributes.
// Tags are always normalized.
func GetOTelContainerTags(rattrs pcommon.Map) []string {
	var containerTags []string
	containerTagsMap := attributes.ContainerTagsFromResourceAttributes(rattrs)
	for k, v := range containerTagsMap {
		t := NormalizeTag(k + ":" + v)
		containerTags = append(containerTags, t)
	}
	return containerTags
}
