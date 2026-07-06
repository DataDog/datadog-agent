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
	"context"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

const (
	// divMebibytes specifies the number of bytes in a mebibyte.
	divMebibytes = 1024 * 1024
	// divPercentage specifies the division necessary for converting fractions to percentages.
	divPercentage = 0.01

	// sdkTraceMetricName is the histogram emitted by Datadog SDKs that ship trace
	// metrics via OTLP (see the OTLP Trace Metrics Export RFC). The otlp-intake
	// backend remaps this metric into the trace.* namespace, but that intake
	// endpoint (otlp.datadoghq.com) is not yet GA. Duplicating the remapping here
	// lets customers sending OTLP through the Agent/DDOT obtain trace metric
	// series without relying on it.
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

// remapSDKTraceMetrics maps traces.span.sdk.metrics.duration datapoints into
// trace.<operation>.{hits,errors,hits.by_type} delta Sum series (appended to all)
// and a trace.<operation>.duration DDSketch consumed directly via consumer.
//
// The counts are emitted as delta Sums so the translator maps them to Datadog
// counts. Duration is emitted as a sketch (rather than a Sum of dp.Sum()) to
// preserve the histogram bucket distribution and produce latency percentiles.
func remapSDKTraceMetrics(ctx context.Context, logger *zap.Logger, consumer Consumer, baseDims *Dimensions, all pmetric.MetricSlice, m pmetric.Metric) {
	if m.Type() != pmetric.MetricTypeHistogram {
		return
	}
	unit := m.Unit()
	dps := m.Histogram().DataPoints()
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		if dp.Flags().NoRecordedValue() {
			continue
		}
		attrs := dp.Attributes()
		operation := sdkOperationName(attrs)

		hits := dp.Count()
		isError := sdkIsError(attrs)
		topLevelHits := sdkTopLevelHits(hits, attrs)

		ts := dp.Timestamp()
		start := dp.StartTimestamp()
		tags := sdkTraceTags(attrs)

		appendSDKTraceSum(all, "trace."+operation+".hits", ts, start, float64(hits), tags)
		if isError {
			appendSDKTraceSum(all, "trace."+operation+".errors", ts, start, float64(hits), tags)
		}
		if topLevelHits > 0 {
			appendSDKTraceSum(all, "trace."+operation+".hits.by_type", ts, start, float64(topLevelHits), tags)
		}

		consumeSDKTraceDuration(ctx, logger, consumer, baseDims, "trace."+operation+".duration", dp, unit, tags)
	}
}

// consumeSDKTraceDuration converts a histogram datapoint into a DDSketch and
// consumes it as a Datadog sketch, preserving the latency distribution.
func consumeSDKTraceDuration(ctx context.Context, logger *zap.Logger, consumer Consumer, baseDims *Dimensions, name string, dp pmetric.HistogramDataPoint, unit string, tags []kv) {
	ddSketch, err := CreateDDSketchFromHistogramOfDuration(&dp, unit)
	if err != nil {
		logger.Debug("Failed to convert SDK trace histogram into DDSketch",
			zap.String(metricName, name), zap.Error(err))
		return
	}
	agentSketch, err := quantile.ConvertDDSketchIntoSketch(ddSketch)
	if err != nil {
		logger.Debug("Failed to convert DDSketch into Sketch",
			zap.String(metricName, name), zap.Error(err))
		return
	}
	dims := baseDims.AddTags(sdkTagStrings(tags)...)
	dims.name = name
	consumer.ConsumeSketch(ctx, dims, uint64(dp.Timestamp()), 0, agentSketch)
}

// sdkTagStrings renders kv tags as Datadog "key:value" tag strings.
func sdkTagStrings(tags []kv) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.K+":"+t.V)
	}
	return out
}

// appendSDKTraceSum appends a delta monotonic Sum metric with a single datapoint.
// Delta sums are mapped to Datadog counts by the translator.
func appendSDKTraceSum(all pmetric.MetricSlice, name string, ts, start pcommon.Timestamp, value float64, tags []kv) {
	metric := all.AppendEmpty()
	metric.SetName(name)
	sum := metric.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	sum.SetIsMonotonic(true)
	ndp := sum.DataPoints().AppendEmpty()
	ndp.SetTimestamp(ts)
	ndp.SetStartTimestamp(start)
	ndp.SetDoubleValue(value)
	for _, t := range tags {
		ndp.Attributes().PutStr(t.K, t.V)
	}
}

// sdkOperationName resolves the DD operation name for the SDK trace metric.
// Default-mode SDK payloads pre-split the operation via datadog.operation.name;
// OTel-semantics payloads omit it and fall back to semconv.
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

// sdkIsError gates errors on the RFC status.code wire forms. HTTP heuristics and
// error.type used by the SMC path do not apply to the SDK duration metric.
func sdkIsError(attrs pcommon.Map) bool {
	switch attributes.GetOTelAttrVal(attrs, false, "status.code") {
	case "ERROR", "STATUS_CODE_ERROR", "2":
		return true
	}
	return false
}

// sdkTopLevelHits returns hits only when the datapoint is flagged top-level.
func sdkTopLevelHits(hits uint64, attrs pcommon.Map) uint64 {
	switch attributes.GetOTelAttrVal(attrs, false, "datadog.span.top_level") {
	case "true", "1":
		return hits
	}
	return 0
}

// sdkTraceTags carries the identifying dimensions for the remapped trace series.
// http.status_code is left unset for non-HTTP spans (the SMC path defaults it to 200).
func sdkTraceTags(attrs pcommon.Map) []kv {
	spanKind := spanKindFromAttr(attrs)
	tags := []kv{
		{"resource", sdkResourceName(attrs)},
		{"span.kind", sdkSpanKindName(spanKind)},
	}
	if status := attributes.GetStatusCode(attrs); status != 0 {
		tags = append(tags, kv{"http.status_code", uintToStr(status)})
	}
	for _, m := range []struct{ key, attr string }{
		{"span.type", "datadog.span.type"},
		{"origin", "datadog.origin"},
	} {
		if v := attributes.GetOTelAttrVal(attrs, false, m.attr); v != "" {
			tags = append(tags, kv{m.key, v})
		}
	}
	return tags
}

func sdkResourceName(attrs pcommon.Map) string {
	if v := attributes.GetOTelAttrVal(attrs, false, "span.name"); v != "" {
		return v
	}
	return "unspecified"
}

// sdkSpanKindName renders a span kind as the lowercase form Datadog APM expects
// (e.g. "server", "client"). ptrace.SpanKind.String() yields capitalized values,
// so we lowercase it here. Inlined rather than importing pkg/trace/transform to
// avoid adding a heavy module dependency to this submodule.
func sdkSpanKindName(k ptrace.SpanKind) string {
	return strings.ToLower(k.String())
}

// spanKindFromAttr maps the span.kind attribute to a ptrace.SpanKind.
func spanKindFromAttr(attrs pcommon.Map) ptrace.SpanKind {
	switch strings.ToUpper(attributes.GetOTelAttrVal(attrs, false, "span.kind")) {
	case "SERVER", "SPAN_KIND_SERVER":
		return ptrace.SpanKindServer
	case "CLIENT", "SPAN_KIND_CLIENT":
		return ptrace.SpanKindClient
	case "PRODUCER", "SPAN_KIND_PRODUCER":
		return ptrace.SpanKindProducer
	case "CONSUMER", "SPAN_KIND_CONSUMER":
		return ptrace.SpanKindConsumer
	case "INTERNAL", "SPAN_KIND_INTERNAL":
		return ptrace.SpanKindInternal
	default:
		return ptrace.SpanKindUnspecified
	}
}

func uintToStr(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}
