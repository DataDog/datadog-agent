// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceutil provides utilities for converting OTel semantics to DD semantics.
package traceutil

import (
	"encoding/binary"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"
	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// Util functions for converting OTel semantics to DD semantics.

var (
	// SignalTypeSet is the OTel attribute set for traces.
	SignalTypeSet = attribute.NewSet(attribute.String("signal", "traces"))
)

// DefaultOTLPServiceName is the default service name for OTel spans when no service name is found in the resource attributes.
const DefaultOTLPServiceName = "otlpresourcenoservicename"

// checkDBType checks if the dbType is a known db type and returns the corresponding span.Type
func checkDBType(dbType string) string {
	spanType, ok := attributes.DBTypes[dbType]
	if ok {
		return spanType
	}
	// span type not found, return generic db type
	return attributes.SpanTypeDB
}

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
		parentSvc := GetOTelService(span, resByID[parentSpan.SpanID()], true)
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
			break
		}
	}

	if normalize {
		val = normalizeutil.NormalizeTagValue(val)
	}

	return val
}

// GetOTelAttrFromEitherMap returns the matched value as a string in either attribute map with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If the key is present in both maps, map1 takes precedence.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrFromEitherMap(map1 pcommon.Map, map2 pcommon.Map, normalize bool, keys ...string) string {
	if val := GetOTelAttrVal(map1, normalize, keys...); val != "" {
		return val
	}
	return GetOTelAttrVal(map2, normalize, keys...)
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

// Semantic Registry Lookup Functions
// These functions use the semantics registry to look up attributes with automatic fallback handling.

// lookupString performs a semantic string lookup using a pre-created accessor,
// avoiding repeated accessor allocation when multiple lookups share the same attribute maps.
func lookupString[A semantics.Accessor](accessor A, concept semantics.Concept, shouldNormalize bool) string {
	val := semantics.LookupString(semantics.DefaultRegistry(), accessor, concept)
	if shouldNormalize && val != "" {
		val = normalizeutil.NormalizeTagValue(val)
	}
	return val
}

// LookupSemanticString looks up a semantic concept from a single OTel attribute map.
// It uses the semantics registry to check all equivalent attribute keys in precedence order.
// If shouldNormalize is true, normalize the return value with NormalizeTagValue.
func LookupSemanticString(attrs pcommon.Map, concept semantics.Concept, shouldNormalize bool) string {
	accessor := semantics.NewPDataMapAccessor(attrs)
	return lookupString(accessor, concept, shouldNormalize)
}

// LookupSemanticStringWithAccessor looks up a semantic concept using a pre-created accessor.
// Use this when performing multiple lookups on the same attribute maps to avoid repeated accessor allocation.
// If shouldNormalize is true, normalize the return value with NormalizeTagValue.
func LookupSemanticStringWithAccessor[A semantics.Accessor](accessor A, concept semantics.Concept, shouldNormalize bool) string {
	return lookupString(accessor, concept, shouldNormalize)
}

// LookupSemanticStringFromDualMaps looks up a semantic concept from two OTel attribute maps.
// The primary map (typically span attributes) takes precedence over secondary (typically resource attributes).
// If shouldNormalize is true, normalize the return value with NormalizeTagValue.
//
// For functions that perform multiple lookups on the same map pair, prefer creating a single
// OTelSpanAccessor and calling lookupString directly to avoid repeated accessor allocation.
func LookupSemanticStringFromDualMaps(primary, secondary pcommon.Map, concept semantics.Concept, shouldNormalize bool) string {
	accessor := semantics.NewOTelSpanAccessor(primary, secondary)
	return lookupString(accessor, concept, shouldNormalize)
}

// LookupSemanticInt64 looks up a semantic concept as an int64 from two OTel attribute maps.
// The primary map takes precedence over secondary.
func LookupSemanticInt64(primary, secondary pcommon.Map, concept semantics.Concept) (int64, bool) {
	accessor := semantics.NewOTelSpanAccessor(primary, secondary)
	return semantics.LookupInt64(semantics.DefaultRegistry(), accessor, concept)
}

// LookupSemanticFloat64 looks up a semantic concept as a float64 from two OTel attribute maps.
// The primary map takes precedence over secondary.
func LookupSemanticFloat64(primary, secondary pcommon.Map, concept semantics.Concept) (float64, bool) {
	accessor := semantics.NewOTelSpanAccessor(primary, secondary)
	return semantics.LookupFloat64(semantics.DefaultRegistry(), accessor, concept)
}

// SpanKind2Type returns a span's type based on the given kind and other present properties.
// This function is used in Resource V1 logic only. See GetOtelSpanType for Resource V2 logic.
func SpanKind2Type(span ptrace.Span, res pcommon.Resource) string {
	var typ string
	switch span.Kind() {
	case ptrace.SpanKindServer:
		typ = "web"
	case ptrace.SpanKindClient:
		typ = "http"
		db := GetOTelAttrValInResAndSpanAttrs(span, res, true, string(semconv.DBSystemKey))
		if db == "" {
			break
		}
		switch db {
		case "redis", "memcached":
			typ = "cache"
		default:
			typ = "db"
		}
	default:
		typ = "custom"
	}
	return typ
}

// GetOTelSpanTypeWithAccessor returns the DD span type based on OTel span kind and attributes,
// using a pre-created accessor to avoid repeated allocation when multiple lookups share the
// same span and resource attribute maps.
func GetOTelSpanTypeWithAccessor[A semantics.Accessor](span ptrace.Span, accessor A) string {
	typ := lookupString(accessor, semantics.ConceptSpanType, false)
	if typ != "" {
		return typ
	}
	switch span.Kind() {
	case ptrace.SpanKindServer:
		typ = "web"
	case ptrace.SpanKindClient:
		db := lookupString(accessor, semantics.ConceptDBSystem, true)
		if db == "" {
			typ = "http"
		} else {
			typ = checkDBType(db)
		}
	default:
		typ = "custom"
	}
	return typ
}

// GetOTelSpanType returns the DD span type based on OTel span kind and attributes.
// This logic is used in ReceiveResourceSpansV2 logic
func GetOTelSpanType(span ptrace.Span, res pcommon.Resource) string {
	return GetOTelSpanTypeWithAccessor(span, semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes()))
}

// getOTelService returns the DD service name using a pre-created accessor.
func getOTelService[A semantics.Accessor](accessor A, normalize bool) string {
	// No need to normalize with NormalizeTagValue since we will do NormalizeService later
	svc := lookupString(accessor, semantics.ConceptServiceName, false)
	if svc == "" {
		svc = DefaultOTLPServiceName
	}
	if normalize {
		newsvc, err := normalizeutil.NormalizeService(svc, "")
		switch err {
		case normalizeutil.ErrTooLong:
			log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", normalizeutil.MaxServiceLen, svc)
		case normalizeutil.ErrInvalid:
			log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s", svc, newsvc)
		}
		svc = newsvc
	}
	return svc
}

// GetOTelService returns the DD service name based on OTel span and resource attributes.
func GetOTelService(span ptrace.Span, res pcommon.Resource, normalize bool) string {
	return getOTelService(semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes()), normalize)
}

// GetOTelServiceWithAccessor is like GetOTelService but uses a pre-created accessor to avoid
// repeated allocation when multiple lookups share the same span and resource attribute maps.
func GetOTelServiceWithAccessor[A semantics.Accessor](accessor A, normalize bool) string {
	return getOTelService(accessor, normalize)
}

// GetOTelResourceV1 returns the DD resource name based on OTel span and resource attributes.
func GetOTelResourceV1(span ptrace.Span, res pcommon.Resource) (resName string) {
	resName = GetOTelAttrValInResAndSpanAttrs(span, res, false, "resource.name")
	if resName == "" {
		if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, "http.request.method", string(semconv.HTTPMethodKey)); m != "" {
			// use the HTTP method + route (if available)
			resName = m
			if route := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv.HTTPRouteKey)); route != "" {
				resName = resName + " " + route
			}
		} else if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv.MessagingOperationKey)); m != "" {
			resName = m
			// use the messaging operation
			if dest := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv.MessagingDestinationKey), string(semconv117.MessagingDestinationNameKey)); dest != "" {
				resName = resName + " " + dest
			}
		} else if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv.RPCMethodKey)); m != "" {
			resName = m
			// use the RPC method
			if svc := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv.RPCServiceKey)); svc != "" {
				resName = resName + " " + svc
			}
		} else if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv117.GraphqlOperationTypeKey)); m != "" {
			// Enrich GraphQL query resource names.
			// See https://github.com/open-telemetry/semantic-conventions/blob/v1.29.0/docs/graphql/graphql-spans.md
			resName = m
			if name := GetOTelAttrValInResAndSpanAttrs(span, res, false, string(semconv117.GraphqlOperationNameKey)); name != "" {
				resName = resName + " " + name
			}
		} else {
			resName = span.Name()
		}
	}
	if len(resName) > normalizeutil.MaxResourceLen {
		resName = resName[:normalizeutil.MaxResourceLen]
	}
	return
}

// getOTelResourceV2 returns the DD resource name using a pre-created accessor.
func getOTelResourceV2[A semantics.Accessor](span ptrace.Span, accessor A) (resName string) {
	defer func() {
		if len(resName) > normalizeutil.MaxResourceLen {
			resName = resName[:normalizeutil.MaxResourceLen]
		}
	}()

	if m := lookupString(accessor, semantics.ConceptResourceName, false); m != "" {
		resName = m
		return
	}

	// HTTP: use method + route (if available)
	if m := lookupString(accessor, semantics.ConceptHTTPMethod, false); m != "" {
		if m == "_OTHER" {
			m = "HTTP"
		}
		resName = m
		if span.Kind() == ptrace.SpanKindServer {
			if route := lookupString(accessor, semantics.ConceptHTTPRoute, false); route != "" {
				resName = resName + " " + route
			}
		}
		return
	}

	// Messaging: use operation + destination
	if m := lookupString(accessor, semantics.ConceptMessagingOperation, false); m != "" {
		resName = m
		if dest := lookupString(accessor, semantics.ConceptMessagingDest, false); dest != "" {
			resName = resName + " " + dest
		}
		return
	}

	// RPC: use method + service
	if m := lookupString(accessor, semantics.ConceptRPCMethod, false); m != "" {
		resName = m
		if svc := lookupString(accessor, semantics.ConceptRPCService, false); svc != "" {
			resName = resName + " " + svc
		}
		return
	}

	// GraphQL: use operation type + name
	// See https://github.com/open-telemetry/semantic-conventions/blob/v1.29.0/docs/graphql/graphql-spans.md
	if m := lookupString(accessor, semantics.ConceptGraphQLOperationType, false); m != "" {
		resName = m
		if name := lookupString(accessor, semantics.ConceptGraphQLOperationName, false); name != "" {
			resName = resName + " " + name
		}
		return
	}

	// Database: use db.statement/db.query.text as resource (for obfuscation)
	// See https://github.com/DataDog/datadog-agent/blob/62619a69cff9863f5b17215847b853681e36ff15/pkg/trace/agent/obfuscate.go#L32
	if m := lookupString(accessor, semantics.ConceptDBSystem, false); m != "" {
		if dbStatement := lookupString(accessor, semantics.ConceptDBStatement, false); dbStatement != "" {
			resName = dbStatement
			return
		}
	}

	resName = span.Name()
	return
}

// GetOTelResourceV2 returns the DD resource name based on OTel span and resource attributes.
func GetOTelResourceV2(span ptrace.Span, res pcommon.Resource) string {
	return getOTelResourceV2(span, semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes()))
}

// GetOTelResourceV2WithAccessor is like GetOTelResourceV2 but uses a pre-created accessor to avoid
// repeated allocation when multiple lookups share the same span and resource attribute maps.
func GetOTelResourceV2WithAccessor[A semantics.Accessor](span ptrace.Span, accessor A) string {
	return getOTelResourceV2(span, accessor)
}

// getOTelOperationNameV2 returns the DD operation name using a pre-created accessor.
func getOTelOperationNameV2[A semantics.Accessor](span ptrace.Span, accessor A) string {

	if operationName := lookupString(accessor, semantics.ConceptOperationName, true); operationName != "" {
		return operationName
	}

	isClient := span.Kind() == ptrace.SpanKindClient
	isServer := span.Kind() == ptrace.SpanKindServer

	// HTTP
	if method := lookupString(accessor, semantics.ConceptHTTPMethod, false); method != "" {
		if isServer {
			return "http.server.request"
		}
		if isClient {
			return "http.client.request"
		}
	}

	// Database
	if v := lookupString(accessor, semantics.ConceptDBSystem, true); v != "" && isClient {
		return v + ".query"
	}

	// Messaging
	system := lookupString(accessor, semantics.ConceptMessagingSystem, true)
	op := lookupString(accessor, semantics.ConceptMessagingOperation, true)
	if system != "" && op != "" {
		switch span.Kind() {
		case ptrace.SpanKindClient, ptrace.SpanKindServer, ptrace.SpanKindConsumer, ptrace.SpanKindProducer:
			return system + "." + op
		}
	}

	// RPC & AWS
	rpcValue := lookupString(accessor, semantics.ConceptRPCSystem, true)
	isRPC := rpcValue != ""
	isAws := isRPC && (rpcValue == "aws-api")
	// AWS client
	if isAws && isClient {
		if service := lookupString(accessor, semantics.ConceptRPCService, true); service != "" {
			return "aws." + service + ".request"
		}
		return "aws.client.request"
	}

	// RPC client
	if isRPC && isClient {
		return rpcValue + ".client.request"
	}
	// RPC server
	if isRPC && isServer {
		return rpcValue + ".server.request"
	}

	// FAAS client
	provider := lookupString(accessor, semantics.ConceptFaaSInvokedProvider, true)
	invokedName := lookupString(accessor, semantics.ConceptFaaSInvokedName, true)
	if provider != "" && invokedName != "" && isClient {
		return provider + "." + invokedName + ".invoke"
	}

	// FAAS server
	trigger := lookupString(accessor, semantics.ConceptFaaSTrigger, true)
	if trigger != "" && isServer {
		return trigger + ".invoke"
	}

	// GraphQL
	if lookupString(accessor, semantics.ConceptGraphQLOperationType, true) != "" {
		return "graphql.server.request"
	}

	// If nothing matches, check for generic http server/client
	protocol := lookupString(accessor, semantics.ConceptNetworkProtocolName, true)
	if isServer {
		if protocol != "" {
			return protocol + ".server.request"
		}
		return "server.request"
	} else if isClient {
		if protocol != "" {
			return protocol + ".client.request"
		}
		return "client.request"
	}

	if span.Kind() != ptrace.SpanKindUnspecified {
		return span.Kind().String()
	}
	return ptrace.SpanKindInternal.String()
}

// GetOTelOperationNameV2 returns the DD operation name based on OTel span and resource attributes.
func GetOTelOperationNameV2(span ptrace.Span, res pcommon.Resource) string {
	return getOTelOperationNameV2(span, semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes()))
}

// GetOTelOperationNameV2WithAccessor is like GetOTelOperationNameV2 but uses a pre-created accessor to avoid
// repeated allocation when multiple lookups share the same span and resource attribute maps.
func GetOTelOperationNameV2WithAccessor[A semantics.Accessor](span ptrace.Span, accessor A) string {
	return getOTelOperationNameV2(span, accessor)
}

// GetOTelOperationNameV1 returns the DD operation name based on OTel span and resource attributes and given configs.
func GetOTelOperationNameV1(
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
		normalizeName, err := normalizeutil.NormalizeName(name)
		switch err {
		case normalizeutil.ErrEmpty:
			log.Debugf("Fixing malformed trace. Name is empty (reason:span_name_empty), setting span.name=%s", normalizeName)
		case normalizeutil.ErrTooLong:
			log.Debugf("Fixing malformed trace. Name is too long (reason:span_name_truncate), truncating span.name to length=%d", normalizeutil.MaxServiceLen)
		case normalizeutil.ErrInvalid:
			log.Debugf("Fixing malformed trace. Name is invalid (reason:span_name_invalid), setting span.name=%s", normalizeName)
		}
		name = normalizeName
	}

	return name
}

// GetOTelContainerTags returns a list of DD container tags in the OTel resource attributes.
// Tags are always normalized.
func GetOTelContainerTags(rattrs pcommon.Map, tagKeys []string) []string {
	var containerTags []string
	containerTagsMap := attributes.ContainerTagsFromResourceAttributes(rattrs)
	for _, key := range tagKeys {
		if mappedKey, ok := attributes.ContainerMappings[key]; ok {
			// If the key has a mapping in ContainerMappings, use the mapped key
			if val, ok := containerTagsMap[mappedKey]; ok {
				t := normalizeutil.NormalizeTag(mappedKey + ":" + val)
				containerTags = append(containerTags, t)
			}
		} else {
			// Otherwise populate as additional container tags
			if val := GetOTelAttrVal(rattrs, false, key); val != "" {
				t := normalizeutil.NormalizeTag(key + ":" + val)
				containerTags = append(containerTags, t)
			}
		}
	}
	return containerTags
}

// OTelTraceIDToUint64 converts an OTel trace ID to an uint64
func OTelTraceIDToUint64(b [16]byte) uint64 {
	return binary.BigEndian.Uint64(b[len(b)-8:])
}

// OTelSpanIDToUint64 converts an OTel span ID to an uint64
func OTelSpanIDToUint64(b [8]byte) uint64 {
	return binary.BigEndian.Uint64(b[:])
}

var spanKindNames = map[ptrace.SpanKind]string{
	ptrace.SpanKindUnspecified: "unspecified",
	ptrace.SpanKindInternal:    "internal",
	ptrace.SpanKindServer:      "server",
	ptrace.SpanKindClient:      "client",
	ptrace.SpanKindProducer:    "producer",
	ptrace.SpanKindConsumer:    "consumer",
}

// OTelSpanKindName converts the given SpanKind to a valid Datadog span kind name.
func OTelSpanKindName(k ptrace.SpanKind) string {
	name, ok := spanKindNames[k]
	if !ok {
		return "unspecified"
	}
	return name
}
