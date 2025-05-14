// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"context"
	"encoding/binary"
	"strings"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv117 "go.opentelemetry.io/collector/semconv/v1.17.0"
	semconv126 "go.opentelemetry.io/collector/semconv/v1.26.0"
	semconv127 "go.opentelemetry.io/collector/semconv/v1.27.0"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.opentelemetry.io/otel/attribute"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// Util functions for converting OTel semantics to DD semantics.

var (
	// SignalTypeSet is the OTel attribute set for traces.
	SignalTypeSet = attribute.NewSet(attribute.String("signal", "traces"))
)

const (
	// TagStatusCode is the tag key for http status code.
	TagStatusCode = "http.status_code"
)

// span.Type constants for db systems
const (
	spanTypeSQL           = "sql"
	spanTypeCassandra     = "cassandra"
	spanTypeRedis         = "redis"
	spanTypeMemcached     = "memcached"
	spanTypeMongoDB       = "mongodb"
	spanTypeElasticsearch = "elasticsearch"
	spanTypeOpenSearch    = "opensearch"
	spanTypeDB            = "db"
)

// DBTypes are semconv types that should map to span.Type values given in the mapping
var dbTypes = map[string]string{
	// SQL db types
	semconv.AttributeDBSystemOtherSQL:      spanTypeSQL,
	semconv.AttributeDBSystemMSSQL:         spanTypeSQL,
	semconv.AttributeDBSystemMySQL:         spanTypeSQL,
	semconv.AttributeDBSystemOracle:        spanTypeSQL,
	semconv.AttributeDBSystemDB2:           spanTypeSQL,
	semconv.AttributeDBSystemPostgreSQL:    spanTypeSQL,
	semconv.AttributeDBSystemRedshift:      spanTypeSQL,
	semconv.AttributeDBSystemCloudscape:    spanTypeSQL,
	semconv.AttributeDBSystemHSQLDB:        spanTypeSQL,
	semconv.AttributeDBSystemMaxDB:         spanTypeSQL,
	semconv.AttributeDBSystemIngres:        spanTypeSQL,
	semconv.AttributeDBSystemFirstSQL:      spanTypeSQL,
	semconv.AttributeDBSystemEDB:           spanTypeSQL,
	semconv.AttributeDBSystemCache:         spanTypeSQL,
	semconv.AttributeDBSystemFirebird:      spanTypeSQL,
	semconv.AttributeDBSystemDerby:         spanTypeSQL,
	semconv.AttributeDBSystemInformix:      spanTypeSQL,
	semconv.AttributeDBSystemMariaDB:       spanTypeSQL,
	semconv.AttributeDBSystemSqlite:        spanTypeSQL,
	semconv.AttributeDBSystemSybase:        spanTypeSQL,
	semconv.AttributeDBSystemTeradata:      spanTypeSQL,
	semconv.AttributeDBSystemVertica:       spanTypeSQL,
	semconv.AttributeDBSystemH2:            spanTypeSQL,
	semconv.AttributeDBSystemColdfusion:    spanTypeSQL,
	semconv.AttributeDBSystemCockroachdb:   spanTypeSQL,
	semconv.AttributeDBSystemProgress:      spanTypeSQL,
	semconv.AttributeDBSystemHanaDB:        spanTypeSQL,
	semconv.AttributeDBSystemAdabas:        spanTypeSQL,
	semconv.AttributeDBSystemFilemaker:     spanTypeSQL,
	semconv.AttributeDBSystemInstantDB:     spanTypeSQL,
	semconv.AttributeDBSystemInterbase:     spanTypeSQL,
	semconv.AttributeDBSystemNetezza:       spanTypeSQL,
	semconv.AttributeDBSystemPervasive:     spanTypeSQL,
	semconv.AttributeDBSystemPointbase:     spanTypeSQL,
	semconv117.AttributeDBSystemClickhouse: spanTypeSQL, // not in semconv 1.6.1

	// Cassandra db types
	semconv.AttributeDBSystemCassandra: spanTypeCassandra,

	// Redis db types
	semconv.AttributeDBSystemRedis: spanTypeRedis,

	// Memcached db types
	semconv.AttributeDBSystemMemcached: spanTypeMemcached,

	// Mongodb db types
	semconv.AttributeDBSystemMongoDB: spanTypeMongoDB,

	// Elasticsearch db types
	semconv.AttributeDBSystemElasticsearch: spanTypeElasticsearch,

	// Opensearch db types, not in semconv 1.6.1
	semconv117.AttributeDBSystemOpensearch: spanTypeOpenSearch,

	// Generic db types
	semconv.AttributeDBSystemHive:      spanTypeDB,
	semconv.AttributeDBSystemHBase:     spanTypeDB,
	semconv.AttributeDBSystemNeo4j:     spanTypeDB,
	semconv.AttributeDBSystemCouchbase: spanTypeDB,
	semconv.AttributeDBSystemCouchDB:   spanTypeDB,
	semconv.AttributeDBSystemCosmosDB:  spanTypeDB,
	semconv.AttributeDBSystemDynamoDB:  spanTypeDB,
	semconv.AttributeDBSystemGeode:     spanTypeDB,
}

// DefaultOTLPServiceName is the default service name for OTel spans when no service name is found in the resource attributes.
const DefaultOTLPServiceName = "otlpresourcenoservicename"

// checkDBType checks if the dbType is a known db type and returns the corresponding span.Type
func checkDBType(dbType string) string {
	spanType, ok := dbTypes[dbType]
	if ok {
		return spanType
	}
	// span type not found, return generic db type
	return spanTypeDB
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

// SpanKind2Type returns a span's type based on the given kind and other present properties.
// This function is used in Resource V1 logic only. See GetOtelSpanType for Resource V2 logic.
func SpanKind2Type(span ptrace.Span, res pcommon.Resource) string {
	var typ string
	switch span.Kind() {
	case ptrace.SpanKindServer:
		typ = "web"
	case ptrace.SpanKindClient:
		typ = "http"
		db := GetOTelAttrValInResAndSpanAttrs(span, res, true, semconv.AttributeDBSystem)
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

// GetOTelSpanType returns the DD span type based on OTel span kind and attributes.
// This logic is used in ReceiveResourceSpansV2 logic
func GetOTelSpanType(span ptrace.Span, res pcommon.Resource) string {
	sattr := span.Attributes()
	rattr := res.Attributes()

	typ := GetOTelAttrFromEitherMap(sattr, rattr, false, "span.type")
	if typ != "" {
		return typ
	}
	switch span.Kind() {
	case ptrace.SpanKindServer:
		typ = "web"
	case ptrace.SpanKindClient:
		db := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeDBSystem)
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

// GetOTelService returns the DD service name based on OTel span and resource attributes.
func GetOTelService(span ptrace.Span, res pcommon.Resource, normalize bool) string {
	// No need to normalize with NormalizeTagValue since we will do NormalizeService later
	svc := GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), false, semconv.AttributeServiceName)
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

// GetOTelResourceV1 returns the DD resource name based on OTel span and resource attributes.
func GetOTelResourceV1(span ptrace.Span, res pcommon.Resource) (resName string) {
	resName = GetOTelAttrValInResAndSpanAttrs(span, res, false, "resource.name")
	if resName == "" {
		if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, "http.request.method", semconv.AttributeHTTPMethod); m != "" {
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
		} else if m := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv117.AttributeGraphqlOperationType); m != "" {
			// Enrich GraphQL query resource names.
			// See https://github.com/open-telemetry/semantic-conventions/blob/v1.29.0/docs/graphql/graphql-spans.md
			resName = m
			if name := GetOTelAttrValInResAndSpanAttrs(span, res, false, semconv117.AttributeGraphqlOperationName); name != "" {
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

// GetOTelResourceV2 returns the DD resource name based on OTel span and resource attributes.
func GetOTelResourceV2(span ptrace.Span, res pcommon.Resource) (resName string) {
	defer func() {
		if len(resName) > normalizeutil.MaxResourceLen {
			resName = resName[:normalizeutil.MaxResourceLen]
		}
	}()
	// Use span and resource attributes for lookups
	sattr := span.Attributes()
	rattr := res.Attributes()

	if m := GetOTelAttrFromEitherMap(sattr, rattr, false, "resource.name"); m != "" {
		resName = m
		return
	}

	if m := GetOTelAttrFromEitherMap(sattr, rattr, false, "http.request.method", semconv.AttributeHTTPMethod); m != "" {
		if m == "_OTHER" {
			m = "HTTP"
		}
		// use the HTTP method + route (if available)
		resName = m
		if span.Kind() == ptrace.SpanKindServer {
			if route := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeHTTPRoute); route != "" {
				resName = resName + " " + route
			}
		}
		return
	}

	if m := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeMessagingOperation); m != "" {
		resName = m
		// use the messaging operation
		if dest := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeMessagingDestination, semconv117.AttributeMessagingDestinationName); dest != "" {
			resName = resName + " " + dest
		}
		return
	}

	if m := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeRPCMethod); m != "" {
		resName = m
		// use the RPC method
		if svc := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeRPCService); svc != "" {
			// ...and service if available
			resName = resName + " " + svc
		}
		return
	}

	// Enrich GraphQL query resource names.
	// See https://github.com/open-telemetry/semantic-conventions/blob/v1.29.0/docs/graphql/graphql-spans.md
	if m := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv117.AttributeGraphqlOperationType); m != "" {
		resName = m
		if name := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv117.AttributeGraphqlOperationName); name != "" {
			resName = resName + " " + name
		}
		return
	}

	if m := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeDBSystem); m != "" {
		// Since traces are obfuscated by span.Resource in pkg/trace/agent/obfuscate.go, we should use span.Resource as the resource name.
		// https://github.com/DataDog/datadog-agent/blob/62619a69cff9863f5b17215847b853681e36ff15/pkg/trace/agent/obfuscate.go#L32
		if dbStatement := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv.AttributeDBStatement); dbStatement != "" {
			resName = dbStatement
			return
		}
		if dbQuery := GetOTelAttrFromEitherMap(sattr, rattr, false, semconv126.AttributeDBQueryText); dbQuery != "" {
			resName = dbQuery
			return
		}
	}

	resName = span.Name()
	return
}

// GetOTelOperationNameV2 returns the DD operation name based on OTel span and resource attributes and given configs.
func GetOTelOperationNameV2(
	span ptrace.Span,
	res pcommon.Resource,
) string {
	sattr := span.Attributes()
	rattr := res.Attributes()

	if operationName := GetOTelAttrFromEitherMap(sattr, rattr, true, "operation.name"); operationName != "" {
		return operationName
	}

	isClient := span.Kind() == ptrace.SpanKindClient
	isServer := span.Kind() == ptrace.SpanKindServer

	// http
	if method := GetOTelAttrFromEitherMap(sattr, rattr, false, "http.request.method", semconv.AttributeHTTPMethod); method != "" {
		if isServer {
			return "http.server.request"
		}
		if isClient {
			return "http.client.request"
		}
	}

	// database
	if v := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeDBSystem); v != "" && isClient {
		return v + ".query"
	}

	// messaging
	system := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeMessagingSystem)
	op := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeMessagingOperation)
	if system != "" && op != "" {
		switch span.Kind() {
		case ptrace.SpanKindClient, ptrace.SpanKindServer, ptrace.SpanKindConsumer, ptrace.SpanKindProducer:
			return system + "." + op
		}
	}

	// RPC & AWS
	rpcValue := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeRPCSystem)
	isRPC := rpcValue != ""
	isAws := isRPC && (rpcValue == "aws-api")
	// AWS client
	if isAws && isClient {
		if service := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeRPCService); service != "" {
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
	provider := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeFaaSInvokedProvider)
	invokedName := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeFaaSInvokedName)
	if provider != "" && invokedName != "" && isClient {
		return provider + "." + invokedName + ".invoke"
	}

	// FAAS server
	trigger := GetOTelAttrFromEitherMap(sattr, rattr, true, semconv.AttributeFaaSTrigger)
	if trigger != "" && isServer {
		return trigger + ".invoke"
	}

	// GraphQL
	if GetOTelAttrFromEitherMap(sattr, rattr, true, "graphql.operation.type") != "" {
		return "graphql.server.request"
	}

	// if nothing matches, checking for generic http server/client
	protocol := GetOTelAttrFromEitherMap(sattr, rattr, true, "network.protocol.name")
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

// GetOtelSource returns the source based on OTel span and resource attributes.
func GetOtelSource(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator) (source.Source, bool) {
	ctx := context.Background()
	src, srcok := tr.ResourceToSource(ctx, res, SignalTypeSet, nil)
	if !srcok {
		if v := GetOTelAttrValInResAndSpanAttrs(span, res, false, "_dd.hostname"); v != "" {
			src = source.Source{Kind: source.HostnameKind, Identifier: v}
			srcok = true
		}
	}
	return src, srcok
}

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes.
func GetOTelHostname(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator, fallbackHost string) string {
	src, srcok := GetOtelSource(span, res, tr)
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
func GetOTelStatusCode(span ptrace.Span, res pcommon.Resource) uint32 {
	if code, ok := span.Attributes().Get(semconv.AttributeHTTPStatusCode); ok {
		return uint32(code.Int())
	}
	if code, ok := span.Attributes().Get("http.response.status_code"); ok {
		return uint32(code.Int())
	}
	if code, ok := res.Attributes().Get(semconv.AttributeHTTPStatusCode); ok {
		return uint32(code.Int())
	}
	if code, ok := res.Attributes().Get("http.response.status_code"); ok {
		return uint32(code.Int())
	}
	return 0
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

// GetOTelEnv returns the environment based on OTel span and resource attributes, with span taking precedence.
func GetOTelEnv(span ptrace.Span, res pcommon.Resource) string {
	return GetOTelAttrFromEitherMap(span.Attributes(), res.Attributes(), true, semconv127.AttributeDeploymentEnvironmentName, semconv.AttributeDeploymentEnvironment)
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
