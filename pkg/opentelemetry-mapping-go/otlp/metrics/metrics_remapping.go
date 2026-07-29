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

const (
	// divMebibytes specifies the number of bytes in a mebibyte.
	divMebibytes = 1024 * 1024
	// divPercentage specifies the division necessary for converting fractions to percentages.
	divPercentage = 0.01

	// sdkTraceMetricName is the DD-SDK OTLP histogram remapped here so Agent/DDOT
	// customers get trace.* series before the otlp-intake endpoint is GA.
	sdkTraceMetricName = "traces.span.sdk.metrics.duration"
)

var emptyAttributesMapping = attributesMapping{}

// remapMetrics extracts Datadog-specific metrics from m and appends them to all.
func remapMetrics(all pmetric.MetricSlice, m pmetric.Metric) {
	remapSystemMetrics(all, m)
	remapContainerMetrics(all, m)
	remapKafkaMetrics(all, m)
	remapJvmMetrics(all, m)
}

// renameMetrics adds the `otel.` or `otelcol_` prefix to metrics.
func renameMetrics(m pmetric.Metric) {
	renameHostMetrics(m)
	renameKafkaMetrics(m)
	renameAgentInternalOTelMetric(m)
}

// remapSystemMetrics extracts system metrics from m and appends them to all.
func remapSystemMetrics(all pmetric.MetricSlice, m pmetric.Metric) {
	name := m.Name()
	if !isHostMetric(name) {
		return
	}
	switch name {
	case "system.cpu.load_average.1m":
		copyMetricWithAttr(all, m, "system.load.1", 1, emptyAttributesMapping)
	case "system.cpu.load_average.5m":
		copyMetricWithAttr(all, m, "system.load.5", 1, emptyAttributesMapping)
	case "system.cpu.load_average.15m":
		copyMetricWithAttr(all, m, "system.load.15", 1, emptyAttributesMapping)
	case "system.cpu.utilization":
		copyMetricWithAttr(all, m, "system.cpu.idle", divPercentage, emptyAttributesMapping, kv{"state", "idle"})
		copyMetricWithAttr(all, m, "system.cpu.user", divPercentage, emptyAttributesMapping, kv{"state", "user"})
		copyMetricWithAttr(all, m, "system.cpu.system", divPercentage, emptyAttributesMapping, kv{"state", "system"})
		copyMetricWithAttr(all, m, "system.cpu.iowait", divPercentage, emptyAttributesMapping, kv{"state", "wait"})
		copyMetricWithAttr(all, m, "system.cpu.stolen", divPercentage, emptyAttributesMapping, kv{"state", "steal"})
	case "system.memory.usage":
		copyMetricWithAttr(all, m, "system.mem.total", divMebibytes, emptyAttributesMapping)
		copyMetricWithAttr(all, m, "system.mem.usable", divMebibytes, emptyAttributesMapping,
			kv{"state", "free"},
			kv{"state", "cached"},
			kv{"state", "buffered"},
		)
	case "system.network.io":
		copyMetricWithAttr(all, m, "system.net.bytes_rcvd", 1, emptyAttributesMapping, kv{"direction", "receive"})
		copyMetricWithAttr(all, m, "system.net.bytes_sent", 1, emptyAttributesMapping, kv{"direction", "transmit"})
	case "system.paging.usage":
		copyMetricWithAttr(all, m, "system.swap.free", divMebibytes, emptyAttributesMapping, kv{"state", "free"})
		copyMetricWithAttr(all, m, "system.swap.used", divMebibytes, emptyAttributesMapping, kv{"state", "used"})
	case "system.filesystem.utilization":
		copyMetricWithAttr(all, m, "system.disk.in_use", 1, emptyAttributesMapping)
	}
}

// remapContainerMetrics extracts system metrics from m and appends them to all.
func remapContainerMetrics(all pmetric.MetricSlice, m pmetric.Metric) {
	name := m.Name()
	if !strings.HasPrefix(name, "container.") {
		// not a container metric
		return
	}
	switch name {
	case "container.cpu.usage.total":
		if addm, ok := copyMetricWithAttr(all, m, "container.cpu.usage", 1, emptyAttributesMapping); ok {
			addm.SetUnit("nanocore")
		}
	case "container.cpu.usage.usermode":
		if addm, ok := copyMetricWithAttr(all, m, "container.cpu.user", 1, emptyAttributesMapping); ok {
			addm.SetUnit("nanocore")
		}
	case "container.cpu.usage.system":
		if addm, ok := copyMetricWithAttr(all, m, "container.cpu.system", 1, emptyAttributesMapping); ok {
			addm.SetUnit("nanocore")
		}
	case "container.cpu.throttling_data.throttled_time":
		copyMetricWithAttr(all, m, "container.cpu.throttled", 1, emptyAttributesMapping)
	case "container.cpu.throttling_data.throttled_periods":
		copyMetricWithAttr(all, m, "container.cpu.throttled.periods", 1, emptyAttributesMapping)
	case "container.memory.usage.total":
		copyMetricWithAttr(all, m, "container.memory.usage", 1, emptyAttributesMapping)
	case "container.memory.active_anon":
		copyMetricWithAttr(all, m, "container.memory.kernel", 1, emptyAttributesMapping)
	case "container.memory.hierarchical_memory_limit":
		copyMetricWithAttr(all, m, "container.memory.limit", 1, emptyAttributesMapping)
	case "container.memory.usage.limit":
		copyMetricWithAttr(all, m, "container.memory.soft_limit", 1, emptyAttributesMapping)
	case "container.memory.total_cache":
		copyMetricWithAttr(all, m, "container.memory.cache", 1, emptyAttributesMapping)
	case "container.memory.total_swap":
		copyMetricWithAttr(all, m, "container.memory.swap", 1, emptyAttributesMapping)
	case "container.blockio.io_service_bytes_recursive":
		copyMetricWithAttr(all, m, "container.io.write", 1, emptyAttributesMapping, kv{"operation", "write"})
		copyMetricWithAttr(all, m, "container.io.read", 1, emptyAttributesMapping, kv{"operation", "read"})
	case "container.blockio.io_serviced_recursive":
		copyMetricWithAttr(all, m, "container.io.write.operations", 1, emptyAttributesMapping, kv{"operation", "write"})
		copyMetricWithAttr(all, m, "container.io.read.operations", 1, emptyAttributesMapping, kv{"operation", "read"})
	case "container.network.io.usage.tx_bytes":
		copyMetricWithAttr(all, m, "container.net.sent", 1, emptyAttributesMapping)
	case "container.network.io.usage.tx_packets":
		copyMetricWithAttr(all, m, "container.net.sent.packets", 1, emptyAttributesMapping)
	case "container.network.io.usage.rx_bytes":
		copyMetricWithAttr(all, m, "container.net.rcvd", 1, emptyAttributesMapping)
	case "container.network.io.usage.rx_packets":
		copyMetricWithAttr(all, m, "container.net.rcvd.packets", 1, emptyAttributesMapping)
	}
}

// isHostMetric determines whether a metric is a system metric.
func isHostMetric(name string) bool {
	return strings.HasPrefix(name, "process.") || strings.HasPrefix(name, "system.")
}

type (
	// kv represents a key/value pair.
	kv struct{ K, V string }

	// attributesMapping contains to mapping of attributes from OTel to DD.
	attributesMapping struct {
		// fixed represents attributes that need to be mapped where the value is
		// already known based on the OTel metric name.
		fixed map[string]string
		// dynamic represents attributes that need to be mapped where the value needs
		// to be dynamically pulled from a data point attribute. Typically when the OTel
		// metric and DD metric have different conventions (e.g. group vs consumer_group).
		dynamic map[string]string
	}
)

// copyMetric copies metric m to dest. The new metric's name will be newname, and all of its datapoints will
// be divided by div. If filter is provided, only the data points that have *either* of the specified string
// attributes will be copied over. If the filtering results in no datapoints, no new metric is added to dest.
// It will add any attributes specified in attributesMapping, by either pulling the value from the datapoint
// for dynamic attributes, or setting the given attribute for fixed attributes.
//
// copyMetric returns the new metric and reports whether it was added to dest.
//
// Please note that copyMetric is restricted to the metric types Sum and Gauge.
func copyMetricWithAttr(dest pmetric.MetricSlice, m pmetric.Metric, newname string, div float64, attributesMapping attributesMapping, filter ...kv) (pmetric.Metric, bool) {
	newm := pmetric.NewMetric()
	m.CopyTo(newm)
	newm.SetName(newname)
	var dps pmetric.NumberDataPointSlice
	switch newm.Type() {
	case pmetric.MetricTypeGauge:
		dps = newm.Gauge().DataPoints()
	case pmetric.MetricTypeSum:
		dps = newm.Sum().DataPoints()
	default:
		// invalid metric type
		return newm, false
	}
	dps.RemoveIf(func(dp pmetric.NumberDataPoint) bool {
		if !hasAny(dp, filter...) {
			return true
		}
		switch dp.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			if div >= 1 {
				// avoid division by zero
				dp.SetIntValue(dp.IntValue() / int64(div))
			}
		case pmetric.NumberDataPointValueTypeDouble:
			if div != 0 {
				dp.SetDoubleValue(dp.DoubleValue() / div)
			}
		}
		// attributes mapping
		for k, v := range attributesMapping.fixed {
			dp.Attributes().PutStr(k, v)
		}
		for old, new := range attributesMapping.dynamic {
			if v, ok := dp.Attributes().Get(old); ok {
				v.CopyTo(dp.Attributes().PutEmpty(new))
			}
		}
		return false
	})
	if dps.Len() > 0 {
		// if we have datapoints, copy it
		addm := dest.AppendEmpty()
		newm.CopyTo(addm)
		return addm, true
	}
	return newm, false
}

// hasAny reports whether point has any of the given string tags.
// If no tags are provided it returns true.
func hasAny(point pmetric.NumberDataPoint, tags ...kv) bool {
	if len(tags) == 0 {
		return true
	}
	attr := point.Attributes()
	for _, tag := range tags {
		v, ok := attr.Get(tag.K)
		if !ok {
			continue
		}
		if v.Str() == tag.V {
			return true
		}
	}
	return false
}

// renameHostMetrics renames otel host metrics to avoid conflicts with Datadog metrics.
func renameHostMetrics(m pmetric.Metric) {
	if isHostMetric(m.Name()) {
		m.SetName("otel." + m.Name())
	}
}

// isAgentInternalOTelMetric determines whether a metric is a internal metric in Agent on OTLP
func isAgentInternalOTelMetric(name string) bool {
	return strings.HasPrefix(name, "datadog_trace_agent") || strings.HasPrefix(name, "datadog_otlp")
}

// renameAgentInternalOTelMetric adds prefix to internal metrics in Agent on OTLP
func renameAgentInternalOTelMetric(m pmetric.Metric) {
	if isAgentInternalOTelMetric(m.Name()) {
		m.SetName("otelcol_" + m.Name())
	}
}

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

// defaultSDKBucketDuration matches the 10s trace-stats bucket window.
const defaultSDKBucketDuration = 10 * time.Second

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
