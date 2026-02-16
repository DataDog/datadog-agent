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
// Only returns a value if the attribute is actually a string type.
func (a *PDataMapAccessor) GetStringAttribute(key string) string {
	v, ok := a.attrs.Get(key)
	if !ok {
		return ""
	}
	return v.Str()
}

// GetFloat64Attribute returns the float64 value for the given key.
// Only returns a value if the attribute is actually a double type.
func (a *PDataMapAccessor) GetFloat64Attribute(key string) (float64, bool) {
	v, ok := a.attrs.Get(key)
	if !ok {
		return 0, false
	}
	if v.Type() == pcommon.ValueTypeDouble {
		return v.Double(), true
	}
	return 0, false
}

// GetInt64Attribute returns the int64 value for the given key.
// Only returns a value if the attribute is actually an int type.
func (a *PDataMapAccessor) GetInt64Attribute(key string) (int64, bool) {
	v, ok := a.attrs.Get(key)
	if !ok {
		return 0, false
	}
	if v.Type() == pcommon.ValueTypeInt {
		return v.Int(), true
	}
	return 0, false
}

// NewOTelSpanAccessor creates a CombinedAccessor for OTel span and resource attributes.
// Span attributes take precedence over resource attributes.
func NewOTelSpanAccessor(spanAttrs, resAttrs pcommon.Map) *CombinedAccessor {
	return NewCombinedAccessor(
		NewPDataMapAccessor(spanAttrs),
		NewPDataMapAccessor(resAttrs),
	)
}
