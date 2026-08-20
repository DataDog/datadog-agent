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

// Package sdktracestats maps Datadog SDK OTLP trace histograms to APM stats.
package sdktracestats

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	stats "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/sdktracestats/pb"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

const (
	// SDKTraceMetricName is the Datadog SDK OTLP duration histogram converted to APM stats.
	SDKTraceMetricName       = "traces.span.sdk.metrics.duration"
	defaultSDKBucketDuration = 10 * time.Second
)

// SDKTraceStatsError identifies a datapoint that could not be translated.
type SDKTraceStatsError struct {
	DataPointIndex int
	Err            error
}

func (e SDKTraceStatsError) Error() string {
	return fmt.Sprintf("datapoint %d: %v", e.DataPointIndex, e.Err)
}

// IsSDKTraceMetric reports whether name is the Datadog SDK OTLP duration histogram.
func IsSDKTraceMetric(name string) bool {
	return name == SDKTraceMetricName
}

// MarshalStatsPayload serializes a trace stats payload for the APM stats intake.
func MarshalStatsPayload(payload *stats.OTLPIntakeStatsPayload) ([]byte, error) {
	return proto.Marshal(payload)
}

// BuildSDKTraceStatsPayload converts a Datadog SDK OTLP duration histogram into
// an APM stats payload with caller-provided envelope metadata. Invalid
// datapoints are skipped and returned as errors.
func BuildSDKTraceStatsPayload(host, source string, rattrs pcommon.Map, metric pmetric.Metric) (*stats.OTLPIntakeStatsPayload, []SDKTraceStatsError) {
	if !IsSDKTraceMetric(metric.Name()) || metric.Type() != pmetric.MetricTypeHistogram {
		return nil, nil
	}
	if metric.Histogram().AggregationTemporality() != pmetric.AggregationTemporalityDelta {
		return nil, nil
	}

	unit := metric.Unit()
	service := attributes.GetService(rattrs, true)
	env := attributes.GetEnv(rattrs)
	version := attributes.GetVersion(rattrs)
	dps := metric.Histogram().DataPoints()
	buckets := make([]*stats.StatsBucketV3, 0, dps.Len())
	var conversionErrors []SDKTraceStatsError
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		if dp.Flags().NoRecordedValue() {
			continue
		}
		groupedStats, err := sdkGroupedStats(service, env, version, &dp, unit)
		if err != nil {
			conversionErrors = append(conversionErrors, SDKTraceStatsError{DataPointIndex: i, Err: err})
			continue
		}
		start, duration := sdkBucketWindow(dp.StartTimestamp(), dp.Timestamp())
		buckets = append(buckets, &stats.StatsBucketV3{
			Start:    int64(start),
			Duration: int64(duration),
			Stats:    []*stats.StatsBucketV3_GroupedStats{groupedStats},
		})
	}
	if len(buckets) == 0 {
		return nil, conversionErrors
	}

	return &stats.OTLPIntakeStatsPayload{
		HostName:    host,
		Stats:       buckets,
		HostTags:    attributes.TagsFromAttributes(rattrs),
		Source:      source,
		Aggregate:   true,
		ContainerId: attributes.GetContainerID(rattrs),
	}, conversionErrors
}

func sdkGroupedStats(service, env, version string, dp *pmetric.HistogramDataPoint, unit string) (*stats.StatsBucketV3_GroupedStats, error) {
	attrs := dp.Attributes()
	hits := dp.Count()
	isError := attributes.GetOTelAttrVal(attrs, false, "status.code") == "STATUS_CODE_ERROR"

	if svc := attributes.GetOTelAttrVal(attrs, false, "service.name"); svc != "" {
		service, _ = normalizeutil.NormalizeService(svc, "")
	}
	resource := "unspecified"
	if value := attributes.GetOTelAttrVal(attrs, false, "span.name"); value != "" {
		resource = value
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
	groupedStats := &stats.StatsBucketV3_GroupedStats{
		Service:        service,
		Env:            env,
		Version:        version,
		Name:           sdkOperationName(attrs),
		Resource:       resource,
		SpanKind:       strings.ToLower(spanKindFromAttr(attrs).String()),
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
		groupedStats.Errors = hits
		groupedStats.HasErrors = hits > 0
	}

	sketch, err := sdkDurationSketch(dp, unit)
	if err != nil {
		return nil, err
	}
	if isError {
		groupedStats.ErrorSparseSketch = sketch
	} else {
		groupedStats.OkSparseSketch = sketch
	}
	return groupedStats, nil
}

func sdkDurationSketch(dp *pmetric.HistogramDataPoint, unit string) ([]byte, error) {
	if dp == nil || dp.Count() == 0 {
		return nil, errors.New("count cannot be zero")
	}
	if dp.Count() > math.MaxInt64 {
		return nil, fmt.Errorf("count %d exceeds maximum %d", dp.Count(), uint64(math.MaxInt64))
	}

	minimum, maximum, sum := math.NaN(), math.NaN(), math.NaN()
	scale := timeUnitScaleToNanos(unit) / float64(time.Second)
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
		return nil, errors.New("histogram minimum is NaN")
	}
	if dp.HasMax() && math.IsNaN(maximum) {
		return nil, errors.New("histogram maximum is NaN")
	}
	if dp.HasSum() && math.IsNaN(sum) {
		return nil, errors.New("histogram sum is NaN")
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
		for i, count := range counts {
			if count > math.MaxUint64-total {
				return nil, errors.New("bucket counts overflow uint64")
			}
			total += count
			if count > math.MaxUint32 {
				return nil, fmt.Errorf("bucket count %d at index %d exceeds maximum %d", count, i, uint64(math.MaxUint32))
			}
		}
		if total != dp.Count() {
			return nil, fmt.Errorf("count %d mismatch total bins %d", dp.Count(), total)
		}
	}

	sketch := &stats.SparseSketch{
		K: []int32{-32768},
		N: []uint32{2},
		Basic: &stats.SparseSketchBasic{
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
		sketch.K = append(sketch.K, int32(len(bounds)+i))
		sketch.N = append(sketch.N, uint32(count))
	}
	return proto.Marshal(sketch)
}

func sdkDurationNanos(dp *pmetric.HistogramDataPoint, unit string) uint64 {
	if !dp.HasSum() {
		return 0
	}
	nanoseconds := dp.Sum() * timeUnitScaleToNanos(unit)
	if nanoseconds < 0 || nanoseconds >= 0x1p64 {
		return 0
	}
	return uint64(nanoseconds)
}

func timeUnitScaleToNanos(unit string) float64 {
	switch unit {
	case "ns":
		return float64(time.Nanosecond)
	case "us", "μs":
		return float64(time.Microsecond)
	case "ms":
		return float64(time.Millisecond)
	case "s":
		return float64(time.Second)
	case "min":
		return float64(time.Minute)
	case "h":
		return float64(time.Hour)
	default:
		return float64(time.Second)
	}
}

func sdkBucketWindow(startTimestamp, endTimestamp pcommon.Timestamp) (start, duration uint64) {
	if startTimestamp == 0 || endTimestamp <= startTimestamp {
		return uint64(endTimestamp), uint64(defaultSDKBucketDuration)
	}
	return uint64(startTimestamp), uint64(endTimestamp - startTimestamp)
}

func sdkIsTraceRoot(attrs pcommon.Map) stats.Trilean {
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.is_trace_root") {
	case "true", "1":
		return stats.Trilean_TRUE
	case "false", "0":
		return stats.Trilean_FALSE
	default:
		return stats.Trilean_NOT_SET
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
	"OK": "0", "CANCELLED": "1", "UNKNOWN": "2", "INVALID_ARGUMENT": "3",
	"DEADLINE_EXCEEDED": "4", "NOT_FOUND": "5", "ALREADY_EXISTS": "6",
	"PERMISSION_DENIED": "7", "RESOURCE_EXHAUSTED": "8", "FAILED_PRECONDITION": "9",
	"ABORTED": "10", "OUT_OF_RANGE": "11", "UNIMPLEMENTED": "12", "INTERNAL": "13",
	"UNAVAILABLE": "14", "DATA_LOSS": "15", "UNAUTHENTICATED": "16",
}

var sdkGroupedStatsAttributeKeys = map[string]struct{}{
	"datadog.is_trace_root": {}, "datadog.operation.name": {}, "datadog.span.top_level": {},
	"datadog.span.type": {}, "http.request.method": {}, "http.response.status_code": {},
	"http.route": {}, "rpc.response.status_code": {}, "service.name": {}, "span.kind": {},
	"span.name": {}, "status.code": {},
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

func sdkOtherTags(attrs pcommon.Map) []*stats.Tag {
	tags := sdkAdditionalMetricTags(attrs)
	if spanType := attributes.GetOTelAttrVal(attrs, false, "datadog.span.type"); spanType != "" {
		tags = append(tags, "span.type:"+spanType)
		sort.Strings(tags)
	}
	otherTags := make([]*stats.Tag, 0, len(tags))
	for _, tag := range tags {
		name, value, ok := strings.Cut(tag, ":")
		if ok {
			otherTags = append(otherTags, &stats.Tag{Name: name, Value: value})
		}
	}
	return otherTags
}

func sdkOperationName(attrs pcommon.Map) string {
	if operation := attributes.GetOTelAttrVal(attrs, false, "datadog.operation.name"); operation != "" {
		return operation
	}
	spanKind := spanKindFromAttr(attrs)
	if operation := attributes.GetOperationName(attrs, spanKind); operation != "" {
		return operation
	}
	return "unknown"
}

var sdkSpanKinds = map[string]ptrace.SpanKind{
	"SERVER": ptrace.SpanKindServer, "CLIENT": ptrace.SpanKindClient,
	"PRODUCER": ptrace.SpanKindProducer, "CONSUMER": ptrace.SpanKindConsumer,
	"INTERNAL": ptrace.SpanKindInternal,
}

func spanKindFromAttr(attrs pcommon.Map) ptrace.SpanKind {
	value := strings.TrimPrefix(strings.ToUpper(attributes.GetOTelAttrVal(attrs, false, "span.kind")), "SPAN_KIND_")
	if kind, ok := sdkSpanKinds[value]; ok {
		return kind
	}
	return ptrace.SpanKindUnspecified
}
