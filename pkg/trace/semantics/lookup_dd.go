// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package semantics

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// DDSpanAccessor provides typed access to combined DD span meta and metrics maps.
// meta (map[string]string) is the primary store for string attributes.
// metrics (map[string]float64) is the primary store for numeric attributes.
type DDSpanAccessor struct {
	meta    StringMapAccessor
	metrics MetricsMapAccessor
}

// NewDDSpanAccessor returns a DDSpanAccessor for DD span meta and metrics maps.
func NewDDSpanAccessor(meta map[string]string, metrics map[string]float64) DDSpanAccessor {
	return DDSpanAccessor{
		meta:    NewStringMapAccessor(meta),
		metrics: NewMetricsMapAccessor(metrics),
	}
}

// GetString returns the value from meta, or "" if missing.
func (a DDSpanAccessor) GetString(key string) string {
	return a.meta.GetString(key)
}

// GetFloat64 returns the value from metrics, or (0, false) if missing.
func (a DDSpanAccessor) GetFloat64(key string) (float64, bool) {
	return a.metrics.GetFloat64(key)
}

// GetInt64 returns the value from metrics as int64 if it is an exact integer, or (0, false) otherwise.
func (a DDSpanAccessor) GetInt64(key string) (int64, bool) {
	return a.metrics.GetInt64(key)
}

// DDSpanAccessorV1 adapts an *idx.InternalSpan into an Accessor.
//
// Attribute routing is strictly typed:
//   - GetString returns a value only if the attribute is stored as a StringValueRef.
//   - GetFloat64 returns a value only if the attribute is stored as a DoubleValue.
//   - GetInt64 returns a value for IntValue or exact-integer DoubleValue.
type DDSpanAccessorV1 struct {
	span *idx.InternalSpan
}

// NewDDSpanAccessorV1 returns a DDSpanAccessorV1 for the given InternalSpan.
func NewDDSpanAccessorV1(s *idx.InternalSpan) DDSpanAccessorV1 {
	return DDSpanAccessorV1{span: s}
}

// GetString returns the string value for the given key, or "" if the attribute
// is missing or not stored as a string. Promoted span fields (component, span.kind,
// env, version) are not handled here; callers that need promoted fields should use
// InternalSpan.GetAttributeAsString directly.
func (a DDSpanAccessorV1) GetString(key string) string {
	attr, ok := a.span.GetAttribute(key)
	if !ok || attr == nil {
		return ""
	}
	v, ok := attr.Value.(*idx.AnyValue_StringValueRef)
	if !ok {
		return ""
	}
	return a.span.Strings.Get(v.StringValueRef)
}

// GetFloat64 returns the attribute value if it is a DoubleValue, or (0, false) otherwise.
func (a DDSpanAccessorV1) GetFloat64(key string) (float64, bool) {
	attr, ok := a.span.GetAttribute(key)
	if !ok || attr == nil {
		return 0, false
	}
	v, ok := attr.Value.(*idx.AnyValue_DoubleValue)
	if !ok {
		return 0, false
	}
	return v.DoubleValue, true
}

// GetInt64 returns the attribute value for IntValue directly, or converts an exact-integer
// DoubleValue. Returns (0, false) for NaN, Inf, fractional doubles, or missing/wrong-typed attributes.
func (a DDSpanAccessorV1) GetInt64(key string) (int64, bool) {
	attr, ok := a.span.GetAttribute(key)
	if !ok || attr == nil {
		return 0, false
	}
	switch v := attr.Value.(type) {
	case *idx.AnyValue_IntValue:
		return v.IntValue, true
	case *idx.AnyValue_DoubleValue:
		f := v.DoubleValue
		i := int64(f)
		if float64(i) != f || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		return i, true
	}
	return 0, false
}
