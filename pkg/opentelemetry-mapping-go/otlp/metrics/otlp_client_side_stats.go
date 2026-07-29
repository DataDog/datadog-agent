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
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// sdkTraceMetricName is the DD-SDK OTLP duration histogram this module converts into an
// APM client-side stats payload (trace.* stats), rather than trace.* metric series.
const sdkTraceMetricName = "traces.span.sdk.metrics.duration"

// defaultSDKBucketDuration matches the 10s trace-stats bucket window.
const defaultSDKBucketDuration = 10 * time.Second

// remapSDKTraceMetrics converts the DD-SDK OTLP duration histogram into an APM
// trace-stats payload and submits it through the APM stats intake. trace.* is a
// reserved trace-stats namespace, so emitting these as metric series/sketches gets the
// points dropped; ClientStatsPayload is the supported producer path. The field mapping
// mirrors dd-source's OTLP trace-metrics exporter so the Agent and agentless
// (/v1/metrics) paths converge on the same trace.* stats.
func remapSDKTraceMetrics(logger *zap.Logger, statsOut chan<- []byte, baseDims *Dimensions, rattrs pcommon.Map, m pmetric.Metric) {
	if m.Type() != pmetric.MetricTypeHistogram {
		return
	}
	if m.Histogram().AggregationTemporality() != pmetric.AggregationTemporalityDelta {
		// Diffing a cumulative datapoint against a stored prior value would need state we
		// don't keep here; skip rather than double-count hits/errors/duration.
		logger.Debug("Skipping non-delta SDK trace metric", zap.String(metricName, m.Name()))
		return
	}
	if statsOut == nil {
		// This remap only runs in collector deployments that wire an APM stats channel
		// (drained to the trace-stats intake). Without one there's nowhere to send.
		logger.Debug("No APM stats channel configured; dropping SDK trace metric",
			zap.String(metricName, m.Name()))
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

	// The channel carries a proto-marshaled StatsPayload, the same contract the OTLP
	// stats connector uses; the datadog exporter drains it and forwards to the backend.
	raw, err := proto.Marshal(&pb.StatsPayload{
		Stats: []*pb.ClientStatsPayload{{
			Hostname:    baseDims.Host(),
			Env:         attributes.GetEnv(rattrs),
			Version:     attributes.GetVersion(rattrs),
			Service:     service,
			ContainerID: attributes.GetContainerID(rattrs),
			Stats:       buckets,
		}},
	})
	if err != nil {
		logger.Debug("Failed to marshal SDK trace stats payload", zap.Error(err))
		return
	}
	statsOut <- raw
}

// sdkGroupedStats maps one histogram datapoint to a ClientGroupedStats row. Name is the
// operation (the backend derives trace.<op>.* from it); the latency DDSketch goes to
// OkSummary or ErrorSummary depending on the datapoint's error status.
func sdkGroupedStats(logger *zap.Logger, service string, blankSketch []byte, dp *pmetric.HistogramDataPoint, unit string) *pb.ClientGroupedStats {
	attrs := dp.Attributes()
	hits := dp.Count()
	isError := sdkIsError(attrs)

	gs := &pb.ClientGroupedStats{
		Service:        service,
		Name:           sdkOperationName(attrs),
		Resource:       sdkResourceName(attrs),
		SpanKind:       strings.ToLower(spanKindFromAttr(attrs).String()),
		HTTPStatusCode: attributes.GetStatusCode(attrs),
		Hits:           hits,
		TopLevelHits:   sdkTopLevelHits(hits, attrs),
		Duration:       sdkDurationNanos(dp, unit),
		Synthetics:     sdkIsSynthetics(attrs),
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

// sdkDurationSketch builds the nanosecond-scaled latency DDSketch for a datapoint and
// marshals it to the ddsketch proto bytes ClientGroupedStats expects. A nil dp yields an
// empty (but valid) sketch.
func sdkDurationSketch(dp *pmetric.HistogramDataPoint, unit string) ([]byte, error) {
	sketch, err := CreateDDSketchFromHistogramOfDuration(dp, unit)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(sketch.ToProto())
}

// sdkDurationNanos is the total duration in nanoseconds, guarding against a negative or
// out-of-uint64-range sum.
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

// sdkBucketWindow derives the stats bucket [start, start+duration) in nanoseconds from a
// datapoint's time window. The APM stats aggregator realigns buckets to its own interval,
// so exact boundaries aren't required; we only need a sane, in-range start.
func sdkBucketWindow(startTS, endTS pcommon.Timestamp) (start, duration uint64) {
	if startTS == 0 || endTS <= startTS {
		return uint64(endTS), uint64(defaultSDKBucketDuration)
	}
	return uint64(startTS), uint64(endTS - startTS)
}

func sdkIsSynthetics(attrs pcommon.Map) bool {
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.origin") {
	case "synthetics", "synthetics-browser":
		return true
	}
	return false
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

// sdkIsError gates on status.code only; the HTTP-status/error.type heuristics used by the
// SMC path don't apply to this metric.
func sdkIsError(attrs pcommon.Map) bool {
	switch attributes.GetOTelAttrVal(attrs, false, "status.code") {
	case "ERROR", "STATUS_CODE_ERROR", "2":
		return true
	}
	return false
}

func sdkTopLevelHits(hits uint64, attrs pcommon.Map) uint64 {
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.span.top_level") {
	case "true", "1":
		return hits
	}
	return 0
}

func sdkResourceName(attrs pcommon.Map) string {
	if v := attributes.GetOTelAttrVal(attrs, false, "span.name"); v != "" {
		return v
	}
	return "unspecified"
}

func spanKindFromAttr(attrs pcommon.Map) ptrace.SpanKind {
	switch strings.ToUpper(attributes.GetOTelAttrVal(attrs, false, "span.kind")) {
	case "SERVER":
		return ptrace.SpanKindServer
	case "CLIENT":
		return ptrace.SpanKindClient
	case "PRODUCER":
		return ptrace.SpanKindProducer
	case "CONSUMER":
		return ptrace.SpanKindConsumer
	case "INTERNAL":
		return ptrace.SpanKindInternal
	default:
		return ptrace.SpanKindUnspecified
	}
}
