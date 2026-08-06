// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metrics converts the Datadog SDK OTLP trace metric into APM client-side stats.
package metrics

import (
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

// isSDKTraceMetric reports whether name is the DD-SDK OTLP duration histogram this module
// converts into APM client-side stats (rather than a trace.* metric series).
func isSDKTraceMetric(name string) bool {
	return name == "traces.span.sdk.metrics.duration"
}

// defaultSDKBucketDuration matches the 10s trace-stats bucket window.
const defaultSDKBucketDuration = 10 * time.Second

func remapSDKTraceMetrics(logger *zap.Logger, consumer Consumer, statsOut chan<- []byte, host string, rattrs pcommon.Map, m pmetric.Metric) {
	if m.Type() != pmetric.MetricTypeHistogram {
		return
	}
	if m.Histogram().AggregationTemporality() != pmetric.AggregationTemporalityDelta {
		// Delta only: we keep no state to diff a cumulative datapoint against.
		logger.Debug("Skipping non-delta SDK trace metric", zap.String(metricName, m.Name()))
		return
	}
	apmConsumer, hasAPMConsumer := consumer.(APMStatsConsumer)
	if statsOut == nil && !hasAPMConsumer {
		logger.Debug("No APM stats destination configured; dropping SDK trace metric", zap.String(metricName, m.Name()))
		return
	}

	unit := m.Unit()
	service := attributes.GetService(rattrs, true)
	// A blank (but valid) sketch fills the non-matching ok/error slot on every row.
	blankSketch, err := sdkDurationSketch(nil, unit)
	if err != nil {
		logger.Debug("Failed to build empty SDK trace duration sketch", zap.Error(err))
	}

	dps := m.Histogram().DataPoints()
	buckets := make([]*pb.ClientStatsBucket, 0, dps.Len())
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		if dp.Flags().NoRecordedValue() {
			continue
		}
		start, duration := sdkBucketWindow(dp.StartTimestamp(), dp.Timestamp())
		buckets = append(buckets, &pb.ClientStatsBucket{
			Start:    start,
			Duration: duration,
			Stats:    []*pb.ClientGroupedStats{sdkGroupedStats(logger, service, blankSketch, &dp, unit)},
		})
	}
	if len(buckets) == 0 {
		return
	}

	clientPayload := &pb.ClientStatsPayload{
		Hostname:      host,
		Env:           attributes.GetEnv(rattrs),
		Version:       attributes.GetVersion(rattrs),
		Service:       service,
		ContainerID:   attributes.GetContainerID(rattrs),
		Lang:          attributes.GetOTelAttrVal(rattrs, false, "telemetry.sdk.language"),
		TracerVersion: attributes.GetOTelAttrVal(rattrs, false, "telemetry.sdk.version"),
		RuntimeID:     attributes.GetOTelAttrVal(rattrs, false, "datadog.runtime_id"),
		ProcessTags:   sdkProcessTags(rattrs),
		Stats:         buckets,
	}
	if statsOut == nil {
		apmConsumer.ConsumeAPMStats(clientPayload)
		return
	}

	raw, err := proto.Marshal(&pb.StatsPayload{
		AgentHostname:  host,
		AgentEnv:       attributes.GetEnv(rattrs),
		ClientComputed: false,
		Stats:          []*pb.ClientStatsPayload{clientPayload},
	})
	if err != nil {
		logger.Debug("Failed to marshal SDK trace stats payload", zap.Error(err))
		return
	}
	statsOut <- raw
}

func sdkGroupedStats(logger *zap.Logger, service string, blankSketch []byte, dp *pmetric.HistogramDataPoint, unit string) *pb.ClientGroupedStats {
	attrs := dp.Attributes()
	hits := dp.Count()

	// isError gates on status.code only (no HTTP-status/error.type heuristics).
	isError := attributes.GetOTelAttrVal(attrs, false, "status.code") == "STATUS_CODE_ERROR"

	if svc := attributes.GetOTelAttrVal(attrs, false, "service.name"); svc != "" {
		service, _ = normalizeutil.NormalizeService(svc, "")
	}
	resource := "unspecified"
	if v := attributes.GetOTelAttrVal(attrs, false, "span.name"); v != "" {
		resource = v
	}
	var topLevelHits uint64
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.span.top_level") {
	case "true", "1":
		topLevelHits = hits
	}
	synthetics := false
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.origin") {
	case "synthetics", "synthetics-browser":
		synthetics = true
	}

	gs := &pb.ClientGroupedStats{
		Service:              service,
		Name:                 sdkOperationName(attrs),
		Resource:             resource,
		SpanKind:             strings.ToLower(spanKindFromAttr(attrs).String()),
		HTTPStatusCode:       attributes.GetStatusCode(attrs),
		GRPCStatusCode:       sdkGRPCStatusCode(attrs),
		HTTPMethod:           attributes.GetOTelAttrVal(attrs, false, "http.request.method"),
		HTTPEndpoint:         attributes.GetOTelAttrVal(attrs, false, "http.route"),
		IsTraceRoot:          sdkIsTraceRoot(attrs),
		Hits:                 hits,
		TopLevelHits:         topLevelHits,
		Duration:             sdkDurationNanos(dp, unit),
		Synthetics:           synthetics,
		AdditionalMetricTags: sdkAdditionalMetricTags(attrs),
	}
	if isError {
		gs.Errors = hits
	}
	if v := attributes.GetOTelAttrVal(attrs, false, "datadog.span.type"); v != "" {
		gs.Type = v
	}

	sketch, err := sdkDurationSketch(dp, unit)
	if err != nil {
		logger.Debug("Failed to build SDK trace duration sketch",
			zap.String(metricName, gs.Name), zap.Error(err))
		sketch = blankSketch
	}
	if isError {
		gs.OkSummary, gs.ErrorSummary = blankSketch, sketch
	} else {
		gs.OkSummary, gs.ErrorSummary = sketch, blankSketch
	}
	return gs
}

func sdkDurationSketch(dp *pmetric.HistogramDataPoint, unit string) ([]byte, error) {
	sketch, err := CreateDDSketchFromHistogramOfDuration(dp, unit)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(sketch.ToProto())
}

// sdkDurationNanos is the total duration in nanoseconds, guarding against out-of-range sums.
func sdkDurationNanos(dp *pmetric.HistogramDataPoint, unit string) uint64 {
	if !dp.HasSum() {
		return 0
	}
	ns := dp.Sum() * getTimeUnitScaleToNanos(unit)
	if ns < 0 || ns >= 0x1p64 {
		return 0
	}
	return uint64(ns)
}

func sdkBucketWindow(startTS, endTS pcommon.Timestamp) (start, duration uint64) {
	if startTS == 0 || endTS <= startTS {
		return uint64(endTS), uint64(defaultSDKBucketDuration) // fall back to the default 10s window
	}
	return uint64(startTS), uint64(endTS - startTS)
}

func sdkProcessTags(attrs pcommon.Map) string {
	return strings.Join(sdkTagList(attrs, "datadog.process_tags"), ",")
}

// sdkTagList reads an arrayValue attribute of colon-joined "key:value" strings.
func sdkTagList(attrs pcommon.Map, key string) []string {
	v, ok := attrs.Get(key)
	if !ok || v.Type() != pcommon.ValueTypeSlice {
		return nil
	}
	slice := v.Slice()
	tags := make([]string, 0, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		tags = append(tags, slice.At(i).AsString())
	}
	return tags
}

func sdkIsTraceRoot(attrs pcommon.Map) pb.Trilean {
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.is_trace_root") {
	case "true", "1":
		return pb.Trilean_TRUE
	case "false", "0":
		return pb.Trilean_FALSE
	default:
		return pb.Trilean_NOT_SET
	}
}

func sdkGRPCStatusCode(attrs pcommon.Map) string {
	value := strings.ToUpper(attributes.GetOTelAttrVal(attrs, false, "rpc.response.status_code"))
	if code, ok := sdkGRPCStatusCodes[value]; ok {
		return code
	}
	return value
}

var sdkGRPCStatusCodes = map[string]string{
	"OK":                  "0",
	"CANCELLED":           "1",
	"UNKNOWN":             "2",
	"INVALID_ARGUMENT":    "3",
	"DEADLINE_EXCEEDED":   "4",
	"NOT_FOUND":           "5",
	"ALREADY_EXISTS":      "6",
	"PERMISSION_DENIED":   "7",
	"RESOURCE_EXHAUSTED":  "8",
	"FAILED_PRECONDITION": "9",
	"ABORTED":             "10",
	"OUT_OF_RANGE":        "11",
	"UNIMPLEMENTED":       "12",
	"INTERNAL":            "13",
	"UNAVAILABLE":         "14",
	"DATA_LOSS":           "15",
	"UNAUTHENTICATED":     "16",
}

var sdkGroupedStatsAttributeKeys = map[string]struct{}{
	"datadog.is_trace_root":     {},
	"datadog.operation.name":    {},
	"datadog.span.top_level":    {},
	"datadog.span.type":         {},
	"http.request.method":       {},
	"http.response.status_code": {},
	"http.route":                {},
	"rpc.response.status_code":  {},
	"service.name":              {},
	"span.kind":                 {},
	"span.name":                 {},
	"status.code":               {},
}

func sdkAdditionalMetricTags(attrs pcommon.Map) []string {
	tags := make([]string, 0, attrs.Len())
	for key, value := range attrs.All() {
		if _, mapped := sdkGroupedStatsAttributeKeys[key]; mapped {
			continue
		}
		if key == "datadog.origin" {
			key = "origin"
		}
		tags = append(tags, normalizeutil.NormalizeTag(key+":"+value.AsString()))
	}
	sort.Strings(tags)
	return tags
}

func sdkOperationName(attrs pcommon.Map) string {
	if op := attributes.GetOTelAttrVal(attrs, false, "datadog.operation.name"); op != "" {
		return op
	}
	spanKind := spanKindFromAttr(attrs)
	if op := attributes.GetOperationName(attrs, spanKind); op != "" {
		return op
	}
	return "unknown"
}

var sdkSpanKinds = map[string]ptrace.SpanKind{
	"SPAN_KIND_SERVER":   ptrace.SpanKindServer,
	"SPAN_KIND_CLIENT":   ptrace.SpanKindClient,
	"SPAN_KIND_PRODUCER": ptrace.SpanKindProducer,
	"SPAN_KIND_CONSUMER": ptrace.SpanKindConsumer,
	"SPAN_KIND_INTERNAL": ptrace.SpanKindInternal,
}

func spanKindFromAttr(attrs pcommon.Map) ptrace.SpanKind {
	if kind, ok := sdkSpanKinds[strings.ToUpper(attributes.GetOTelAttrVal(attrs, false, "span.kind"))]; ok {
		return kind
	}
	return ptrace.SpanKindUnspecified
}
