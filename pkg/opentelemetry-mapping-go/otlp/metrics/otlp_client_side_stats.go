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
	"fmt"
	"math"
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

const sdkTraceStatsSource = "otlp-intake-metrics-sdk"

func remapSDKTraceMetrics(logger *zap.Logger, consumer Consumer, otlpStatsOut chan<- []byte, host string, rattrs pcommon.Map, m pmetric.Metric) {
	if m.Type() != pmetric.MetricTypeHistogram {
		return
	}
	if m.Histogram().AggregationTemporality() != pmetric.AggregationTemporalityDelta {
		// Delta only: we keep no state to diff a cumulative datapoint against.
		logger.Debug("Skipping non-delta SDK trace metric", zap.String(metricName, m.Name()))
		return
	}
	otlpConsumer, hasOTLPConsumer := consumer.(OTLPStatsConsumer)
	if otlpStatsOut == nil && !hasOTLPConsumer {
		logger.Debug("No APM stats destination configured; dropping SDK trace metric", zap.String(metricName, m.Name()))
		return
	}

	unit := m.Unit()
	service := attributes.GetService(rattrs, true)
	env := attributes.GetEnv(rattrs)
	version := attributes.GetVersion(rattrs)

	dps := m.Histogram().DataPoints()
	buckets := make([]*pb.StatsBucketV3, 0, dps.Len())
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		if dp.Flags().NoRecordedValue() {
			continue
		}
		groupedStats, err := sdkGroupedStats(service, env, version, &dp, unit)
		if err != nil {
			logger.Debug("Failed to build SDK trace duration stats", zap.Error(err))
			continue
		}
		start, duration := sdkBucketWindow(dp.StartTimestamp(), dp.Timestamp())
		buckets = append(buckets, &pb.StatsBucketV3{
			Start:    int64(start),
			Duration: int64(duration),
			Stats:    []*pb.StatsBucketV3_GroupedStats{groupedStats},
		})
	}
	if len(buckets) == 0 {
		return
	}

	raw, err := proto.Marshal(&pb.OTLPIntakeStatsPayload{
		HostName:    host,
		Stats:       buckets,
		HostTags:    attributes.TagsFromAttributes(rattrs),
		Source:      sdkTraceStatsSource,
		Aggregate:   true,
		ContainerId: attributes.GetContainerID(rattrs),
	})
	if err != nil {
		logger.Debug("Failed to marshal SDK trace stats payload", zap.Error(err))
		return
	}
	if otlpStatsOut != nil {
		otlpStatsOut <- raw
		return
	}
	otlpConsumer.ConsumeOTLPStats(raw)
}

func sdkGroupedStats(service, env, version string, dp *pmetric.HistogramDataPoint, unit string) (*pb.StatsBucketV3_GroupedStats, error) {
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

	duration := sdkDurationNanos(dp, unit)
	gs := &pb.StatsBucketV3_GroupedStats{
		Service:        service,
		Env:            env,
		Version:        version,
		Name:           sdkOperationName(attrs),
		Resource:       resource,
		SpanKind:       spanKindFromAttr(attrs).String(),
		HttpStatusCode: int32(attributes.GetStatusCode(attrs)),
		GrpcStatusCode: sdkGRPCStatusCode(attrs),
		IsTraceRoot:    sdkIsTraceRoot(attrs),
		Hits:           hits,
		HasHits:        hits > 0,
		TopLevelHits:   topLevelHits,
		Duration:       duration,
		HasDuration:    duration > 0,
		Synthetics:     synthetics,
		OtherTags:      sdkOtherTags(attrs),
	}
	if isError {
		gs.Errors = hits
		gs.HasErrors = hits > 0
	}

	sketch, err := sdkDurationSketch(dp, unit)
	if err != nil {
		return nil, err
	}
	if isError {
		gs.ErrorSparseSketch = sketch
	} else {
		gs.OkSparseSketch = sketch
	}
	return gs, nil
}

func sdkDurationSketch(dp *pmetric.HistogramDataPoint, unit string) ([]byte, error) {
	if dp == nil || dp.Count() == 0 {
		return nil, fmt.Errorf("count cannot be zero")
	}

	minimum, maximum, sum := math.NaN(), math.NaN(), math.NaN()
	scale := getTimeUnitScaleToNanos(unit) / float64(time.Second)
	if dp.HasMin() {
		minimum = dp.Min() * scale
	}
	if dp.HasMax() {
		maximum = dp.Max() * scale
	}
	if dp.HasSum() {
		sum = dp.Sum() * scale
	}
	if dp.HasMin() && math.IsNaN(minimum) {
		return nil, fmt.Errorf("histogram minimum is NaN")
	}
	if dp.HasMax() && math.IsNaN(maximum) {
		return nil, fmt.Errorf("histogram maximum is NaN")
	}
	if dp.HasSum() && math.IsNaN(sum) {
		return nil, fmt.Errorf("histogram sum is NaN")
	}
	if dp.HasMin() && dp.HasMax() && minimum > maximum {
		return nil, fmt.Errorf("min %g is greater than max %g", minimum, maximum)
	}

	bounds := dp.ExplicitBounds().AsRaw()
	counts := dp.BucketCounts().AsRaw()
	if len(bounds) > 64 {
		return nil, fmt.Errorf("bounds length %d is too high, maximum 64", len(bounds))
	}
	if len(bounds) > 0 || len(counts) > 0 {
		if len(counts) != len(bounds)+1 {
			return nil, fmt.Errorf("counts length %d must be 1 greater than bounds length %d", len(counts), len(bounds))
		}
		var total uint64
		for _, count := range counts {
			total += count
		}
		if total != dp.Count() {
			return nil, fmt.Errorf("count %d mismatch total bins %d", dp.Count(), total)
		}
	}

	sketch := &pb.SparseSketch{
		K: []int32{-32768},
		N: []uint32{2},
		Basic: &pb.SparseSketchBasic{
			Min: minimum, Max: maximum, Sum: sum, Count: int64(dp.Count()),
		},
	}
	var previous float64
	var previousF32 float32
	for i, bound := range bounds {
		bound *= scale
		boundF32 := float32(bound)
		if math.IsNaN(bound) || math.IsInf(bound, 0) || math.IsInf(float64(boundF32), 0) {
			return nil, fmt.Errorf("bound must be a finite float32, got %v", bound)
		}
		if i > 0 && (previous >= bound || !(previousF32 < boundF32)) {
			return nil, fmt.Errorf("bound %v must stay strictly increasing after float32 conversion", bound)
		}
		sketch.K = append(sketch.K, int32(i))
		sketch.N = append(sketch.N, math.Float32bits(boundF32))
		previous, previousF32 = bound, boundF32
	}
	for i, count := range counts {
		appendSparseBin(sketch, int32(len(bounds)+i), count)
	}
	return proto.Marshal(sketch)
}

func appendSparseBin(sketch *pb.SparseSketch, key int32, count uint64) {
	if count <= math.MaxUint32 {
		sketch.K = append(sketch.K, key)
		sketch.N = append(sketch.N, uint32(count))
		return
	}
	if remainder := count % math.MaxUint32; remainder != 0 {
		sketch.K = append(sketch.K, key)
		sketch.N = append(sketch.N, uint32(remainder))
	}
	for count >= math.MaxUint32 {
		sketch.K = append(sketch.K, key)
		sketch.N = append(sketch.N, math.MaxUint32)
		count -= math.MaxUint32
	}
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

func sdkIsTraceRoot(attrs pcommon.Map) pb.OTLPTrilean {
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.is_trace_root") {
	case "true", "1":
		return pb.OTLPTrilean_OTLP_TRUE
	case "false", "0":
		return pb.OTLPTrilean_OTLP_FALSE
	default:
		return pb.OTLPTrilean_OTLP_NOT_SET
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

func sdkOtherTags(attrs pcommon.Map) []*pb.StatsTag {
	tags := sdkAdditionalMetricTags(attrs)
	if spanType := attributes.GetOTelAttrVal(attrs, false, "datadog.span.type"); spanType != "" {
		tags = append(tags, "span.type:"+spanType)
		sort.Strings(tags)
	}
	otherTags := make([]*pb.StatsTag, 0, len(tags))
	for _, tag := range tags {
		name, value, ok := strings.Cut(tag, ":")
		if ok {
			otherTags = append(otherTags, &pb.StatsTag{Name: name, Value: value})
		}
	}
	return otherTags
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
