// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"bytes"

	"github.com/tinylib/msgp/msgp"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

const (
	// topLevelKey is a special metric, it's 1 if the span is top-level, 0 if not, this is kept for backwards
	// compatibility but will eventually be replaced with just using the preferred tracerTopLevelKey
	topLevelKey = "_top_level"
	// measuredKey is a special metric flag that marks a span for trace metrics calculation.
	measuredKey = "_dd.measured"
	// tracerTopLevelKey is a metric flag set by tracers on top_level spans
	tracerTopLevelKey = "_dd.top_level"
	// partialVersionKey is a metric carrying the snapshot seq number in the case the span is a partial snapshot
	partialVersionKey = "_dd.partial_version"
	// metaTraceIDHigh is the meta tag key for the high 64 bits of a 128-bit trace ID.
	// This is used by Datadog tracers to propagate the upper bits of the trace ID.
	metaTraceIDHigh = "_dd.p.tid"
)

// HasTopLevel returns true if span is top-level.
func HasTopLevel(s *pb.Span) bool {
	return HasTopLevelMetrics(s.Metrics)
}

// HasTopLevelMetrics returns true if the provided metrics map indicates the span is top-level.
func HasTopLevelMetrics(metrics map[string]float64) bool {
	return metrics[topLevelKey] == 1 || metrics[tracerTopLevelKey] == 1
}

// HasTopLevelMetricsV1 returns true if the provided metrics map indicates the span is top-level.
func HasTopLevelMetricsV1(s *idx.InternalSpan) bool {
	topLevel, ok := s.GetAttributeAsFloat64(topLevelKey)
	if ok && topLevel == 1 {
		return true
	}
	tracerTopLevel, ok := s.GetAttributeAsFloat64(tracerTopLevelKey)
	return ok && tracerTopLevel == 1
}

// UpdateTracerTopLevel sets _top_level tag on spans flagged by the tracer
func UpdateTracerTopLevel(s *pb.Span) {
	if s.Metrics[tracerTopLevelKey] == 1 {
		SetMetric(s, topLevelKey, 1)
	}
}

// UpdateTracerTopLevelV1 sets _top_level tag on spans flagged by the tracer
func UpdateTracerTopLevelV1(s *idx.InternalSpan) {
	topLevel, ok := s.GetAttributeAsFloat64(tracerTopLevelKey)
	if ok && topLevel == 1 {
		s.SetFloat64Attribute(topLevelKey, 1)
	}
}

// IsMeasured returns true if a span should be measured (i.e., it should get trace metrics calculated).
func IsMeasured(s *pb.Span) bool {
	return IsMeasuredMetrics(s.Metrics)
}

// IsMeasuredMetrics returns true if a span should be measured (i.e., it should get trace metrics calculated).
func IsMeasuredMetrics(metrics map[string]float64) bool {
	return metrics[measuredKey] == 1
}

// IsMeasuredMetricsV1 returns true if a span should be measured (i.e., it should get trace metrics calculated).
func IsMeasuredMetricsV1(s *idx.InternalSpan) bool {
	measured, ok := s.GetAttributeAsFloat64(measuredKey)
	return ok && measured == 1
}

// IsPartialSnapshot returns true if the span is a partial snapshot.
// This kind of spans are partial images of long-running spans.
// When incomplete, a partial snapshot has a metric _dd.partial_version which is a positive integer.
// The metric usually increases each time a new version of the same span is sent by the tracer
func IsPartialSnapshot(s *pb.Span) bool {
	return IsPartialSnapshotMetrics(s.Metrics)
}

// IsPartialSnapshotMetrics returns true if the span is a partial snapshot.
// These kinds of spans are partial images of long-running spans.
// When incomplete, a partial snapshot has a metric _dd.partial_version which is a positive integer.
// The metric usually increases each time a new version of the same span is sent by the tracer
func IsPartialSnapshotMetrics(metrics map[string]float64) bool {
	v, ok := metrics[partialVersionKey]
	return ok && v >= 0
}

// IsPartialSnapshotMetricsV1 returns true if the span is a partial snapshot.
// These kinds of spans are partial images of long-running spans.
// When incomplete, a partial snapshot has a metric _dd.partial_version which is a positive integer.
// The metric usually increases each time a new version of the same span is sent by the tracer
func IsPartialSnapshotMetricsV1(s *idx.InternalSpan) bool {
	partialVersion, ok := s.GetAttributeAsFloat64(partialVersionKey)
	return ok && partialVersion >= 0
}

// SetTopLevel sets the top-level attribute of the span.
func SetTopLevel(s *pb.Span, topLevel bool) {
	if !topLevel {
		if s.Metrics == nil {
			return
		}
		delete(s.Metrics, topLevelKey)
		return
	}
	// Setting the metrics value, so that code downstream in the pipeline
	// can identify this as top-level without recomputing everything.
	SetMetric(s, topLevelKey, 1)
}

// SetMeasured sets the measured attribute of the span.
func SetMeasured(s *pb.Span, measured bool) {
	if !measured {
		if s.Metrics == nil {
			return
		}
		delete(s.Metrics, measuredKey)
		return
	}
	// Setting the metrics value, so that code downstream in the pipeline
	// can identify this as top-level without recomputing everything.
	SetMetric(s, measuredKey, 1)
}

// SetMetric sets the metric at key to the val on the span s.
func SetMetric(s *pb.Span, key string, val float64) {
	if s.Metrics == nil {
		s.Metrics = make(map[string]float64)
	}
	s.Metrics[key] = val
}

// SetMeta sets the metadata at key to the val on the span s.
func SetMeta(s *pb.Span, key, val string) {
	if s.Meta == nil {
		s.Meta = make(map[string]string)
	}
	s.Meta[key] = val
}

// GetMeta gets the metadata value in the span Meta map.
func GetMeta(s *pb.Span, key string) (string, bool) {
	if s.Meta == nil {
		return "", false
	}
	val, ok := s.Meta[key]
	return val, ok
}

// GetMetaDefault gets the metadata value in the span Meta map and fallbacks to fallback.
func GetMetaDefault(s *pb.Span, key, fallback string) string {
	if s.Meta == nil {
		return fallback
	}
	if val, ok := s.Meta[key]; ok {
		return val
	}
	return fallback
}

// SetMetaStruct sets the structured metadata at key to the val on the span s.
func SetMetaStruct(s *pb.Span, key string, val interface{}) error {
	var b bytes.Buffer

	if s.MetaStruct == nil {
		s.MetaStruct = make(map[string][]byte)
	}
	writer := msgp.NewWriter(&b)
	err := writer.WriteIntf(val)
	if err != nil {
		return err
	}
	writer.Flush()
	s.MetaStruct[key] = b.Bytes()
	return nil
}

// GetMetaStruct gets the structured metadata value in the span MetaStruct map.
func GetMetaStruct(s *pb.Span, key string) (interface{}, bool) {
	if s.MetaStruct == nil {
		return nil, false
	}
	if rawVal, ok := s.MetaStruct[key]; ok {
		val, _, err := msgp.ReadIntfBytes(rawVal)
		if err != nil {
			ok = false
		}
		return val, ok
	}
	return nil, false
}

// GetMetric gets the metadata value in the span Metrics map.
func GetMetric(s *pb.Span, key string) (float64, bool) {
	if s.Metrics == nil {
		return 0, false
	}
	val, ok := s.Metrics[key]
	return val, ok
}

// GetTraceIDHigh returns the high 64 bits of a 128-bit trace ID, if present.
// The high bits are stored in the span's Meta map under the "_dd.p.tid" key
// as a 16-character lowercase hex string.
func GetTraceIDHigh(s *pb.Span) (string, bool) {
	return GetMeta(s, metaTraceIDHigh)
}

// SetTraceIDHigh sets the high 64 bits of a 128-bit trace ID.
// The value should be a 16-character lowercase hex string.
func SetTraceIDHigh(s *pb.Span, value string) {
	SetMeta(s, metaTraceIDHigh, value)
}

// HasTraceIDHigh returns true if the span has the high 64 bits of a 128-bit trace ID.
func HasTraceIDHigh(s *pb.Span) bool {
	_, ok := GetTraceIDHigh(s)
	return ok
}

// UpgradeTraceID upgrades dst's trace ID to 128-bit if src has high bits that dst lacks.
// This is useful when a 64-bit trace ID span arrives first, then a 128-bit span
// from the same trace arrives later. Returns true if an upgrade occurred.
//
// The upgrade only happens if the low 64 bits match (same trace) and dst lacks
// the high bits that src has.
func UpgradeTraceID(dst, src *pb.Span) bool {
	if dst.TraceID != src.TraceID {
		return false // Different traces
	}
	if HasTraceIDHigh(dst) {
		return false // Already has high bits
	}
	if tidHigh, ok := GetTraceIDHigh(src); ok {
		SetTraceIDHigh(dst, tidHigh)
		return true
	}
	return false
}

// CopyTraceID copies the full trace ID (both low and high 64 bits) from src to dst.
// This handles both 64-bit trace IDs (just the TraceID field) and 128-bit trace IDs
// by copying the TraceID field and the _dd.p.tid meta tag (if present).
//
// For 128-bit trace IDs:
//   - Low 64 bits: span.TraceID field
//   - High 64 bits: span.Meta["_dd.p.tid"] (hex-encoded string)
//
// This is essential for maintaining trace identity during span manipulation,
// particularly for log-trace correlation and trace propagation.
func CopyTraceID(dst, src *pb.Span) {
	dst.TraceID = src.TraceID
	if tidHigh, ok := GetMeta(src, metaTraceIDHigh); ok {
		SetMeta(dst, metaTraceIDHigh, tidHigh)
	}
}

// SameTraceID returns true if both spans have the same full trace ID.
// This compares both the low 64 bits (TraceID field) and the high 64 bits
// (_dd.p.tid meta tag) to properly handle 128-bit trace IDs.
//
// If either span lacks the high 64 bits, only the low 64 bits are compared.
// This is necessary because tracers only set _dd.p.tid on the first span
// in each trace chunk to avoid redundant data in the payload. Other spans
// in the same chunk implicitly share the 128-bit trace ID from the first span.
// See: https://docs.datadoghq.com/tracing/guide/span_and_trace_id_format/
func SameTraceID(a, b *pb.Span) bool {
	if a.TraceID != b.TraceID {
		return false
	}
	aHigh, aHasHigh := GetMeta(a, metaTraceIDHigh)
	bHigh, bHasHigh := GetMeta(b, metaTraceIDHigh)
	// If either span lacks high bits, only compare low 64 bits (already done above).
	// Tracers only set _dd.p.tid on the first span in each chunk, so other spans
	// in the same chunk won't have it but still share the same 128-bit trace ID.
	if !aHasHigh || !bHasHigh {
		return true
	}
	// Both have high bits, compare them
	return aHigh == bHigh
}
