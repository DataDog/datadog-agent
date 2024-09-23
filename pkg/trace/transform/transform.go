package transform

import (
	"encoding/hex"
	"fmt"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"
	"strconv"
	"strings"
)

// otelSpanToDDSpan converts an OTel span to a DD span.
// The converted DD span only has the minimal number of fields for APM stats calculation and is only meant
// to be used in OTLPTracesToConcentratorInputs. Do not use them for other purposes.
func OtelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	ddspan := &pb.Span{
		Service:  traceutil.GetOTelService(otelspan, otelres, true),
		Name:     traceutil.GetOTelOperationName(otelspan, otelres, lib, conf.OTLPReceiver.SpanNameAsResourceName, conf.OTLPReceiver.SpanNameRemappings, true),
		Resource: traceutil.GetOTelResource(otelspan, otelres),
		TraceID:  traceutil.OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   traceutil.OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: traceutil.OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Type:     traceutil.GetOTelSpanType(otelspan, otelres),
	}
	spanKind := otelspan.Kind()
	traceutil.SetMeta(ddspan, "span.kind", traceutil.OTelSpanKindName(spanKind))
	code := traceutil.GetOTelStatusCode(otelspan)
	if code != 0 {
		traceutil.SetMetric(ddspan, traceutil.TagStatusCode, float64(code))
	}
	if otelspan.Status().Code() == ptrace.StatusCodeError {
		ddspan.Error = 1
	}
	if isTopLevel {
		traceutil.SetTopLevel(ddspan, true)
	}
	if isMeasured := traceutil.GetOTelAttrVal(otelspan.Attributes(), false, "_dd.measured"); isMeasured == "1" {
		traceutil.SetMeasured(ddspan, true)
	} else if topLevelByKind && (spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer) {
		// When enable_otlp_compute_top_level_by_span_kind is true, compute stats for client-side spans
		traceutil.SetMeasured(ddspan, true)
	}
	for _, peerTagKey := range peerTagKeys {
		if peerTagVal := traceutil.GetOTelAttrValInResAndSpanAttrs(otelspan, otelres, false, peerTagKey); peerTagVal != "" {
			traceutil.SetMeta(ddspan, peerTagKey, peerTagVal)
		}
	}
	return ddspan
}

// otelSpanToDDSpan converts an OTel span to a DD span.
// The converted DD span only has the minimal number of fields for APM stats calculation and is only meant
// to be used in OTLPTracesToConcentratorInputs. Do not use them for other purposes.
func OtelSpanToDDSpan(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
) *pb.Span {
	spanKind := otelspan.Kind()
	isTopLevel := otelspan.ParentSpanID() == pcommon.NewSpanIDEmpty() || spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer

	ddspan := OtelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, peerTagKeys)

	attr := otelres.Attributes()
	rattr := make(map[string]string, attr.Len())
	attr.Range(func(k string, v pcommon.Value) bool {
		rattr[k] = v.AsString()
		return true
	})

	for k, v := range rattr {
		if k == "service.name" || k == "operation.name" || k == "resource.name" || k == "span.type" {
			continue
		}
		if k == "analytics.event" {
			if v, err := strconv.ParseBool(v); err == nil {
				if v {
					traceutil.SetMetric(ddspan, sampler.KeySamplingRateEventExtraction, 1)
				} else {
					traceutil.SetMetric(ddspan, sampler.KeySamplingRateEventExtraction, 0)
				}
			}
		} else {
			traceutil.SetMeta(ddspan, k, v)
		}
	}

	traceID := otelspan.TraceID()
	traceutil.SetMeta(ddspan, "otel.trace_id", hex.EncodeToString(traceID[:]))
	if _, ok := ddspan.Meta["version"]; !ok {
		if ver := rattr[semconv.AttributeServiceVersion]; ver != "" {
			SetMetaOTLP(ddspan, "version", ver)
		}
	}

	if otelspan.Events().Len() > 0 {
		traceutil.SetMeta(ddspan, "events", MarshalEvents(otelspan.Events()))
	}
	if otelspan.Links().Len() > 0 {
		traceutil.SetMeta(ddspan, "_dd.span_links", MarshalLinks(otelspan.Links()))
	}

	var gotMethodFromNewConv bool
	var gotStatusCodeFromNewConv bool

	otelspan.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			traceutil.SetMetric(ddspan, k, v.Double())
		case pcommon.ValueTypeInt:
			traceutil.SetMetric(ddspan, k, float64(v.Int()))
		default:
			// Exclude Datadog APM conventions.
			// These are handled below explicitly.
			if k != "http.method" && k != "http.status_code" {
				traceutil.SetMeta(ddspan, k, v.AsString())
			}
		}

		// `http.method` was renamed to `http.request.method` in the HTTP stabilization from v1.23.
		// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
		// `http.method` is also the Datadog APM convention for the HTTP method.
		// We check both conventions and use the new one if it is present.
		// See https://datadoghq.atlassian.net/wiki/spaces/APM/pages/2357395856/Span+attributes#[inlineExtension]HTTP
		if k == "http.request.method" {
			gotMethodFromNewConv = true
			traceutil.SetMeta(ddspan, "http.method", v.AsString())
		} else if k == "http.method" && !gotMethodFromNewConv {
			traceutil.SetMeta(ddspan, "http.method", v.AsString())
		}

		// `http.status_code` was renamed to `http.response.status_code` in the HTTP stabilization from v1.23.
		// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
		// `http.status_code` is also the Datadog APM convention for the HTTP status code.
		// We check both conventions and use the new one if it is present.
		// See https://datadoghq.atlassian.net/wiki/spaces/APM/pages/2357395856/Span+attributes#[inlineExtension]HTTP
		if k == "http.response.status_code" {
			gotStatusCodeFromNewConv = true
			traceutil.SetMeta(ddspan, "http.status_code", v.AsString())
		} else if k == "http.status_code" && !gotStatusCodeFromNewConv {
			traceutil.SetMeta(ddspan, "http.status_code", v.AsString())
		}

		return true
	})

	if otelspan.TraceState().AsRaw() != "" {
		traceutil.SetMeta(ddspan, "w3c.tracestate", otelspan.TraceState().AsRaw())
	}
	if lib.Name() != "" {
		traceutil.SetMeta(ddspan, semconv.OtelLibraryName, lib.Name())
	}
	if lib.Version() != "" {
		traceutil.SetMeta(ddspan, semconv.OtelLibraryVersion, lib.Version())
	}
	traceutil.SetMeta(ddspan, semconv.OtelStatusCode, otelspan.Status().Code().String())
	if msg := otelspan.Status().Message(); msg != "" {
		traceutil.SetMeta(ddspan, semconv.OtelStatusDescription, msg)
	}
	Status2Error(otelspan.Status(), otelspan.Events(), ddspan)

	return ddspan
}

// MarshalEvents marshals events into JSON.
func MarshalEvents(events ptrace.SpanEventSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		var wrote bool
		str.WriteString("{")
		if v := e.Timestamp(); v != 0 {
			str.WriteString(`"time_unix_nano":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
			wrote = true
		}
		if v := e.Name(); v != "" {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"name":"`)
			str.WriteString(v)
			str.WriteString(`"`)
			wrote = true
		}
		if e.Attributes().Len() > 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"attributes":{`)
			j := 0
			e.Attributes().Range(func(k string, v pcommon.Value) bool {
				if j > 0 {
					str.WriteString(",")
				}
				str.WriteString(`"`)
				str.WriteString(k)
				str.WriteString(`":"`)
				str.WriteString(v.AsString())
				str.WriteString(`"`)
				j++
				return true
			})
			str.WriteString("}")
			wrote = true
		}
		if v := e.DroppedAttributesCount(); v != 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
}

// MarshalLinks marshals span links into JSON.
func MarshalLinks(links ptrace.SpanLinkSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < links.Len(); i++ {
		l := links.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		t := l.TraceID()
		str.WriteString(`{"trace_id":"`)
		str.WriteString(hex.EncodeToString(t[:]))
		s := l.SpanID()
		str.WriteString(`","span_id":"`)
		str.WriteString(hex.EncodeToString(s[:]))
		str.WriteString(`"`)
		if ts := l.TraceState().AsRaw(); len(ts) > 0 {
			str.WriteString(`,"trace_state":"`)
			str.WriteString(ts)
			str.WriteString(`"`)
		}
		if l.Attributes().Len() > 0 {
			str.WriteString(`,"attributes":{`)
			var b bool
			l.Attributes().Range(func(k string, v pcommon.Value) bool {
				if b {
					str.WriteString(",")
				}
				b = true
				str.WriteString(`"`)
				str.WriteString(k)
				str.WriteString(`":"`)
				str.WriteString(v.AsString())
				str.WriteString(`"`)
				return true
			})
			str.WriteString("}")
		}
		if l.DroppedAttributesCount() > 0 {
			str.WriteString(`,"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(l.DroppedAttributesCount()), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
}

// setMetaOTLP sets the k/v OTLP attribute pair as a tag on span s.
func SetMetaOTLP(s *pb.Span, k, v string) {
	switch k {
	case "operation.name":
		s.Name = v
	case "service.name":
		s.Service = v
	case "resource.name":
		s.Resource = v
	case "span.type":
		s.Type = v
	case "analytics.event":
		if v, err := strconv.ParseBool(v); err == nil {
			if v {
				s.Metrics[sampler.KeySamplingRateEventExtraction] = 1
			} else {
				s.Metrics[sampler.KeySamplingRateEventExtraction] = 0
			}
		}
	default:
		s.Meta[k] = v
	}
}

// setMetricOTLP sets the k/v OTLP attribute pair as a metric on span s.
func SetMetricOTLP(s *pb.Span, k string, v float64) {
	switch k {
	case "sampling.priority":
		s.Metrics["_sampling_priority_v1"] = v
	default:
		s.Metrics[k] = v
	}
}

// status2Error checks the given status and events and applies any potential error and messages
// to the given span attributes.
func Status2Error(status ptrace.Status, events ptrace.SpanEventSlice, span *pb.Span) {
	if status.Code() != ptrace.StatusCodeError {
		return
	}
	span.Error = 1
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if strings.ToLower(e.Name()) != "exception" {
			continue
		}
		attrs := e.Attributes()
		if v, ok := attrs.Get(semconv.AttributeExceptionMessage); ok {
			span.Meta["error.msg"] = v.AsString()
		}
		if v, ok := attrs.Get(semconv.AttributeExceptionType); ok {
			span.Meta["error.type"] = v.AsString()
		}
		if v, ok := attrs.Get(semconv.AttributeExceptionStacktrace); ok {
			span.Meta["error.stack"] = v.AsString()
		}
	}
	if _, ok := span.Meta["error.msg"]; !ok {
		// no error message was extracted, find alternatives
		if status.Message() != "" {
			// use the status message
			span.Meta["error.msg"] = status.Message()
		} else if _, httpcode := GetFirstFromMap(span.Meta, "http.response.status_code", "http.status_code"); httpcode != "" {
			// `http.status_code` was renamed to `http.response.status_code` in the HTTP stabilization from v1.23.
			// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes

			// http.status_text was removed in spec v0.7.0 (https://github.com/open-telemetry/opentelemetry-specification/pull/972)
			// TODO (OTEL-1791) Remove this and use a map from status code to status text.
			if httptext, ok := span.Meta["http.status_text"]; ok {
				span.Meta["error.msg"] = fmt.Sprintf("%s %s", httpcode, httptext)
			} else {
				span.Meta["error.msg"] = httpcode
			}
		}
	}
}

// GetFirstFromMap checks each key in the given keys in the map and returns the first key-value pair whose
// key matches, or empty strings if none matches.
func GetFirstFromMap(m map[string]string, keys ...string) (string, string) {
	for _, key := range keys {
		if val := m[key]; val != "" {
			return key, val
		}
	}
	return "", ""
}
