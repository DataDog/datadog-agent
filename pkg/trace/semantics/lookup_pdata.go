// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// PDataMapAccessor wraps a pcommon.Map to implement SpanAccessor.
// This is useful for accessing OTel span or resource attributes directly.
type PDataMapAccessor struct {
	attrs pcommon.Map
}

// NewPDataMapAccessor creates a new PDataMapAccessor from a pcommon.Map.
func NewPDataMapAccessor(attrs pcommon.Map) *PDataMapAccessor {
	return &PDataMapAccessor{attrs: attrs}
}

// GetStringAttribute returns the string value for the given key.
func (a *PDataMapAccessor) GetStringAttribute(key string) string {
	v, ok := a.attrs.Get(key)
	if !ok {
		return ""
	}
	return v.AsString()
}

// GetFloat64Attribute returns the float64 value for the given key.
func (a *PDataMapAccessor) GetFloat64Attribute(key string) (float64, bool) {
	v, ok := a.attrs.Get(key)
	if !ok {
		return 0, false
	}
	switch v.Type() {
	case pcommon.ValueTypeDouble:
		return v.Double(), true
	case pcommon.ValueTypeInt:
		return float64(v.Int()), true
	default:
		return 0, false
	}
}

// GetInt64Attribute returns the int64 value for the given key.
func (a *PDataMapAccessor) GetInt64Attribute(key string) (int64, bool) {
	v, ok := a.attrs.Get(key)
	if !ok {
		return 0, false
	}
	switch v.Type() {
	case pcommon.ValueTypeInt:
		return v.Int(), true
	case pcommon.ValueTypeDouble:
		return int64(v.Double()), true
	default:
		return 0, false
	}
}

// NewOTelSpanAccessor creates a CombinedAccessor for OTel span and resource attributes.
// Span attributes take precedence over resource attributes.
func NewOTelSpanAccessor(spanAttrs, resAttrs pcommon.Map) *CombinedAccessor {
	return NewCombinedAccessor(
		NewPDataMapAccessor(spanAttrs),
		NewPDataMapAccessor(resAttrs),
	)
}
