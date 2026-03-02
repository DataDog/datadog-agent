// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"strconv"

	semconv117 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv126 "go.opentelemetry.io/otel/semconv/v1.26.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	traceutilotel "github.com/DataDog/datadog-agent/pkg/trace/otel/traceutil"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// getFirstFromMaps returns the first non-empty value for the given keys, checking span attrs then resource attrs (span takes precedence).
func getFirstFromMaps(sattr, rattr map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := sattr[k]; v != "" {
			return v
		}
		if v := rattr[k]; v != "" {
			return v
		}
	}
	return ""
}

// getFirstFromMetaMetricsRes returns the first non-empty value for the given keys, checking span Meta (string),
// then span Metrics (formatted as string), then resource attrs. Used when attributes are stored directly on the span.
func getFirstFromMetaMetricsRes(meta map[string]string, metrics map[string]float64, rattr map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := meta[k]; v != "" {
			return v
		}
		if f, ok := metrics[k]; ok {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
		if v := rattr[k]; v != "" {
			return v
		}
	}
	return ""
}

// getOTelServiceFromMaps returns the DD service name from resource/span attribute maps (V2 logic uses resource service.name).
func getOTelServiceFromMaps(sattr, rattr map[string]string, normalize bool) string {
	svc := getFirstFromMaps(sattr, rattr, string(semconv.ServiceNameKey))
	if svc == "" {
		svc = traceutilotel.DefaultOTLPServiceName
	}
	if normalize {
		svc, _ = normalizeutil.NormalizeService(svc, "")
	}
	return svc
}

// getOTelSpanTypeFromMaps returns the DD span type from kind and attribute maps (receiveResourceSpansV2 logic).
func getOTelSpanTypeFromMaps(sattr, rattr map[string]string, kind int32) string {
	if typ := getFirstFromMaps(sattr, rattr, "span.type"); typ != "" {
		return typ
	}
	switch kind {
	case 2: // SpanKindServer
		return "web"
	case 3: // SpanKindClient
		if db := getFirstFromMaps(sattr, rattr, string(semconv.DBSystemKey)); db != "" {
			if t, ok := attributes.DBTypes[db]; ok {
				return t
			}
			return attributes.SpanTypeDB
		}
		return "http"
	default:
		return "custom"
	}
}

// getOTelResourceV2FromMaps returns the DD resource name from attribute maps and span name (V2 logic).
func getOTelResourceV2FromMaps(sattr, rattr map[string]string, spanName string, kind int32) string {
	if m := getFirstFromMaps(sattr, rattr, "resource.name"); m != "" {
		return truncateResource(m)
	}
	if m := getFirstFromMaps(sattr, rattr, "http.request.method", string(semconv.HTTPMethodKey)); m != "" {
		if m == "_OTHER" {
			m = "HTTP"
		}
		resName := m
		if kind == 2 { // server
			if route := getFirstFromMaps(sattr, rattr, string(semconv.HTTPRouteKey)); route != "" {
				resName = resName + " " + route
			}
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMaps(sattr, rattr, string(semconv.MessagingOperationKey)); m != "" {
		resName := m
		if dest := getFirstFromMaps(sattr, rattr, string(semconv.MessagingDestinationKey), string(semconv117.MessagingDestinationNameKey)); dest != "" {
			resName = resName + " " + dest
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMaps(sattr, rattr, string(semconv.RPCMethodKey)); m != "" {
		resName := m
		if svc := getFirstFromMaps(sattr, rattr, string(semconv.RPCServiceKey)); svc != "" {
			resName = resName + " " + svc
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMaps(sattr, rattr, string(semconv117.GraphqlOperationTypeKey)); m != "" {
		resName := m
		if name := getFirstFromMaps(sattr, rattr, string(semconv117.GraphqlOperationNameKey)); name != "" {
			resName = resName + " " + name
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMaps(sattr, rattr, string(semconv.DBSystemKey)); m != "" {
		if dbStmt := getFirstFromMaps(sattr, rattr, string(semconv.DBStatementKey)); dbStmt != "" {
			return truncateResource(dbStmt)
		}
		if dbQuery := getFirstFromMaps(sattr, rattr, string(semconv126.DBQueryTextKey)); dbQuery != "" {
			return truncateResource(dbQuery)
		}
	}
	return truncateResource(spanName)
}

func truncateResource(s string) string {
	if len(s) > normalizeutil.MaxResourceLen {
		return s[:normalizeutil.MaxResourceLen]
	}
	return s
}

// getOTelOperationNameV2FromMaps returns the DD operation name from attribute maps and span name (V2 logic).
func getOTelOperationNameV2FromMaps(sattr, rattr map[string]string, spanName string, kind int32) string {
	if operationName := getFirstFromMaps(sattr, rattr, "operation.name"); operationName != "" {
		return operationName
	}
	isClient := kind == 3
	isServer := kind == 2
	if method := getFirstFromMaps(sattr, rattr, "http.request.method", string(semconv.HTTPMethodKey)); method != "" {
		if isServer {
			return "http.server.request"
		}
		if isClient {
			return "http.client.request"
		}
	}
	if v := getFirstFromMaps(sattr, rattr, string(semconv.DBSystemKey)); v != "" && isClient {
		return v + ".query"
	}
	system := getFirstFromMaps(sattr, rattr, string(semconv.MessagingSystemKey))
	op := getFirstFromMaps(sattr, rattr, string(semconv.MessagingOperationKey))
	if system != "" && op != "" && (kind == 3 || kind == 2 || kind == 5 || kind == 4) {
		return system + "." + op
	}
	rpcValue := getFirstFromMaps(sattr, rattr, string(semconv.RPCSystemKey))
	isRPC := rpcValue != ""
	isAws := isRPC && (rpcValue == "aws-api")
	if isAws && isClient {
		if service := getFirstFromMaps(sattr, rattr, string(semconv.RPCServiceKey)); service != "" {
			return "aws." + service + ".request"
		}
		return "aws.client.request"
	}
	if isRPC && isClient {
		return rpcValue + ".client.request"
	}
	if isRPC && isServer {
		return rpcValue + ".server.request"
	}
	provider := getFirstFromMaps(sattr, rattr, string(semconv.FaaSInvokedProviderKey))
	invokedName := getFirstFromMaps(sattr, rattr, string(semconv.FaaSInvokedNameKey))
	if provider != "" && invokedName != "" && isClient {
		return provider + "." + invokedName + ".invoke"
	}
	trigger := getFirstFromMaps(sattr, rattr, string(semconv.FaaSTriggerKey))
	if trigger != "" && isServer {
		return trigger + ".invoke"
	}
	if getFirstFromMaps(sattr, rattr, "graphql.operation.type") != "" {
		return "graphql.server.request"
	}
	protocol := getFirstFromMaps(sattr, rattr, "network.protocol.name")
	if isServer {
		if protocol != "" {
			return protocol + ".server.request"
		}
		return "server.request"
	}
	if isClient {
		if protocol != "" {
			return protocol + ".client.request"
		}
		return "client.request"
	}
	if kind != 0 {
		return otelSpanKindName(kind)
	}
	return "internal"
}

// otelSpanKindName returns the string name for OTLP SpanKind (0=unspecified, 1=internal, 2=server, 3=client, 4=producer, 5=consumer).
func otelSpanKindName(kind int32) string {
	names := map[int32]string{0: "unspecified", 1: "internal", 2: "server", 3: "client", 4: "producer", 5: "consumer"}
	if n, ok := names[kind]; ok {
		return n
	}
	return "unspecified"
}

// Identity helpers that read from span Meta + Metrics (raw keys) and resource attrs, for use when attributes are written directly to the span.

func getOTelServiceFromMetaMetrics(meta, rattr map[string]string, metrics map[string]float64, normalize bool) string {
	svc := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.ServiceNameKey))
	if svc == "" {
		svc = traceutilotel.DefaultOTLPServiceName
	}
	if normalize {
		svc, _ = normalizeutil.NormalizeService(svc, "")
	}
	return svc
}

func getOTelSpanTypeFromMetaMetrics(meta, rattr map[string]string, metrics map[string]float64, kind int32) string {
	if typ := getFirstFromMetaMetricsRes(meta, metrics, rattr, "span.type"); typ != "" {
		return typ
	}
	switch kind {
	case 2:
		return "web"
	case 3:
		if db := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.DBSystemKey)); db != "" {
			if t, ok := attributes.DBTypes[db]; ok {
				return t
			}
			return attributes.SpanTypeDB
		}
		return "http"
	default:
		return "custom"
	}
}

func getOTelResourceV2FromMetaMetrics(meta, rattr map[string]string, metrics map[string]float64, spanName string, kind int32) string {
	if m := getFirstFromMetaMetricsRes(meta, metrics, rattr, "resource.name"); m != "" {
		return truncateResource(m)
	}
	if m := getFirstFromMetaMetricsRes(meta, metrics, rattr, "http.request.method", string(semconv.HTTPMethodKey)); m != "" {
		if m == "_OTHER" {
			m = "HTTP"
		}
		resName := m
		if kind == 2 {
			if route := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.HTTPRouteKey)); route != "" {
				resName = resName + " " + route
			}
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.MessagingOperationKey)); m != "" {
		resName := m
		if dest := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.MessagingDestinationKey), string(semconv117.MessagingDestinationNameKey)); dest != "" {
			resName = resName + " " + dest
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.RPCMethodKey)); m != "" {
		resName := m
		if svc := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.RPCServiceKey)); svc != "" {
			resName = resName + " " + svc
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv117.GraphqlOperationTypeKey)); m != "" {
		resName := m
		if name := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv117.GraphqlOperationNameKey)); name != "" {
			resName = resName + " " + name
		}
		return truncateResource(resName)
	}
	if m := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.DBSystemKey)); m != "" {
		if dbStmt := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.DBStatementKey)); dbStmt != "" {
			return truncateResource(dbStmt)
		}
		if dbQuery := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv126.DBQueryTextKey)); dbQuery != "" {
			return truncateResource(dbQuery)
		}
	}
	return truncateResource(spanName)
}

func getOTelOperationNameV2FromMetaMetrics(meta, rattr map[string]string, metrics map[string]float64, spanName string, kind int32) string {
	if operationName := getFirstFromMetaMetricsRes(meta, metrics, rattr, "operation.name"); operationName != "" {
		return operationName
	}
	isClient := kind == 3
	isServer := kind == 2
	if method := getFirstFromMetaMetricsRes(meta, metrics, rattr, "http.request.method", string(semconv.HTTPMethodKey)); method != "" {
		if isServer {
			return "http.server.request"
		}
		if isClient {
			return "http.client.request"
		}
	}
	if v := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.DBSystemKey)); v != "" && isClient {
		return v + ".query"
	}
	system := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.MessagingSystemKey))
	op := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.MessagingOperationKey))
	if system != "" && op != "" && (kind == 3 || kind == 2 || kind == 5 || kind == 4) {
		return system + "." + op
	}
	rpcValue := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.RPCSystemKey))
	isRPC := rpcValue != ""
	isAws := isRPC && (rpcValue == "aws-api")
	if isAws && isClient {
		if service := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.RPCServiceKey)); service != "" {
			return "aws." + service + ".request"
		}
		return "aws.client.request"
	}
	if isRPC && isClient {
		return rpcValue + ".client.request"
	}
	if isRPC && isServer {
		return rpcValue + ".server.request"
	}
	provider := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.FaaSInvokedProviderKey))
	invokedName := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.FaaSInvokedNameKey))
	if provider != "" && invokedName != "" && isClient {
		return provider + "." + invokedName + ".invoke"
	}
	trigger := getFirstFromMetaMetricsRes(meta, metrics, rattr, string(semconv.FaaSTriggerKey))
	if trigger != "" && isServer {
		return trigger + ".invoke"
	}
	if getFirstFromMetaMetricsRes(meta, metrics, rattr, "graphql.operation.type") != "" {
		return "graphql.server.request"
	}
	protocol := getFirstFromMetaMetricsRes(meta, metrics, rattr, "network.protocol.name")
	if isServer {
		if protocol != "" {
			return protocol + ".server.request"
		}
		return "server.request"
	}
	if isClient {
		if protocol != "" {
			return protocol + ".client.request"
		}
		return "client.request"
	}
	if kind != 0 {
		return otelSpanKindName(kind)
	}
	return "internal"
}

func getOTelStatusCodeFromMetaMetrics(meta, rattr map[string]string, metrics map[string]float64) uint32 {
	keys := []string{string(semconv.HTTPStatusCodeKey), "http.response.status_code"}
	for _, k := range keys {
		if v := getFirstFromMetaMetricsRes(meta, metrics, rattr, k); v != "" {
			if i, err := strconv.ParseUint(v, 10, 32); err == nil {
				return uint32(i)
			}
		}
	}
	return 0
}
