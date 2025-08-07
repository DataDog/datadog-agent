package translator

import (
	"net/http"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.5.0"
	"go.uber.org/zap"
)

func ToTraces(logger *zap.Logger, payload map[string]any, req *http.Request, reqBytes []byte) (ptrace.Traces, error) {
	results := ptrace.NewTraces()
	rs := results.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl(semconv.SchemaURL)
	parseRUMRequestIntoResource(rs.Resource(), payload, req, reqBytes)

	in := rs.ScopeSpans().AppendEmpty()
	in.Scope().SetName(InstrumentationScopeName)

	traceID, spanID, err := parseIDs(payload, req)
	if err != nil {
		return ptrace.NewTraces(), err
	}
	logger.Info("Trace ID", zap.String("traceID", traceID.String()))
	logger.Info("Span ID", zap.String("spanID", spanID.String()))

	newSpan := in.Spans().AppendEmpty()
	if eventType, ok := payload[AttrType].(string); ok {
		newSpan.SetName("datadog.rum." + eventType)
	} else {
		newSpan.SetName("datadog.rum.event")
	}
	newSpan.SetTraceID(traceID)
	newSpan.SetSpanID(spanID)

	flatPayload := flattenJSON(payload)

	setDateForSpan(payload, newSpan)
	setAttributes(flatPayload, newSpan.Attributes())

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

	// default duration to 0 if not found
	var duration float64 = 0
	if resource, ok := payload["resource"].(map[string]any); ok {
		if durationVal, ok := resource["duration"].(float64); ok {
			duration = durationVal
		}
	}

	span.SetStartTimestamp(pcommon.Timestamp(dateNanoseconds))
	span.SetEndTimestamp(pcommon.Timestamp(dateNanoseconds + uint64(duration)))
}
