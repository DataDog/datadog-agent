// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// PDataMapAccessor provides typed access to a pcommon.Map for semantic lookups.
type PDataMapAccessor struct {
	attrs pcommon.Map
}

// NewPDataMapAccessor returns a PDataMapAccessor for a pcommon.Map.
func NewPDataMapAccessor(attrs pcommon.Map) *PDataMapAccessor {
	return &PDataMapAccessor{attrs: attrs}
}

// GetString returns the attribute value if the underlying pdata type is Str, or "" otherwise.
// Numeric attributes should be accessed via GetInt64 or GetFloat64 for type safety.
func (a *PDataMapAccessor) GetString(key string) string {
	v, ok := a.attrs.Get(key)
	if !ok || v.Type() != pcommon.ValueTypeStr {
		return ""
	}
	return v.Str()
}

// GetInt64 returns the attribute value as int64 if the underlying pdata type is Int
// or an exact-integer Double.
func (a *PDataMapAccessor) GetInt64(key string) (int64, bool) {
	v, ok := a.attrs.Get(key)
	if !ok {
		return 0, false
	}
	switch v.Type() {
	case pcommon.ValueTypeInt:
		return v.Int(), true
	case pcommon.ValueTypeDouble:
		f := v.Double()
		i := int64(f)
		if float64(i) == f {
			return i, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// GetFloat64 returns the attribute value as float64 if the underlying pdata type is Double or Int.
func (a *PDataMapAccessor) GetFloat64(key string) (float64, bool) {
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

// OTelSpanAccessor provides typed access to combined span and resource attributes.
// The primary accessor (typically span attributes) takes precedence over the secondary.
type OTelSpanAccessor struct {
	primary   PDataMapAccessor
	secondary PDataMapAccessor
}

// NewOTelSpanAccessor returns an OTelSpanAccessor for OTel span and resource attributes.
// Span attributes take precedence over resource attributes.
func NewOTelSpanAccessor(spanAttrs, resAttrs pcommon.Map) *OTelSpanAccessor {
	return &OTelSpanAccessor{
		primary:   PDataMapAccessor{attrs: spanAttrs},
		secondary: PDataMapAccessor{attrs: resAttrs},
	}
}

// GetString returns the first non-empty string value, checking primary then secondary.
func (a *OTelSpanAccessor) GetString(key string) string {
	if v := a.primary.GetString(key); v != "" {
		return v
	}
	return a.secondary.GetString(key)
}

// GetInt64 returns the first found int64 value, checking primary then secondary.
func (a *OTelSpanAccessor) GetInt64(key string) (int64, bool) {
	if v, ok := a.primary.GetInt64(key); ok {
		return v, true
	}
	return a.secondary.GetInt64(key)
}

// GetFloat64 returns the first found float64 value, checking primary then secondary.
func (a *OTelSpanAccessor) GetFloat64(key string) (float64, bool) {
	if v, ok := a.primary.GetFloat64(key); ok {
		return v, true
	}
	return a.secondary.GetFloat64(key)
}
