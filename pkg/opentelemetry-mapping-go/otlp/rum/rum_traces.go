// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.5.0"
	"go.uber.org/zap"
)

// ToTraces converts a RUM payload to OpenTelemetry Traces
func ToTraces(logger *zap.Logger, payload map[string]any, req *http.Request) (ptrace.Traces, error) {
	results := ptrace.NewTraces()
	rs := results.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl(semconv.SchemaURL)
	rs.Resource().Attributes().PutStr(string(semconv.ServiceNameKey), "browser-rum-sdk")
	parseDDForwardIntoResource(rs.Resource().Attributes(), req.URL.Query().Get("ddforward"))

	in := rs.ScopeSpans().AppendEmpty()
	in.Scope().SetName(instrumentationScopeName)

	traceID, spanID, err := parseIDs(payload)
	if err != nil {
		return ptrace.NewTraces(), err
	}
	logger.Info("Trace ID", zap.String("traceID", traceID.String()))
	logger.Info("Span ID", zap.String("spanID", spanID.String()))

	newSpan := in.Spans().AppendEmpty()
	if eventType, ok := payload[typeKey].(string); ok {
		newSpan.SetName("datadog.rum." + eventType)
	} else {
		newSpan.SetName("datadog.rum.event")
	}
	newSpan.SetTraceID(traceID)
	newSpan.SetSpanID(spanID)

	flatPayload := flattenJSON(payload)

	setDateForSpan(payload, newSpan)
	setOTLPAttributes(flatPayload, newSpan.Attributes())

	return results, nil
}

func setDateForSpan(payload map[string]any, span ptrace.Span) {
	date, ok := payload["date"]
	if !ok {
		return
	}
	dateFloat, ok := date.(float64)
	if !ok {
		return
	}
	dateNanoseconds := uint64(dateFloat) * 1e6

	var duration float64
	if resource, ok := payload["resource"].(map[string]any); ok {
		if durationVal, ok := resource["duration"].(float64); ok {
			duration = durationVal
		}
	}
	fmt.Println("duration", uint64(duration))

	span.SetStartTimestamp(pcommon.Timestamp(dateNanoseconds))
	span.SetEndTimestamp(pcommon.Timestamp(dateNanoseconds + uint64(duration)))
}
