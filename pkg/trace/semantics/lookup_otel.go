// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

// CombinedAccessor combines multiple SpanAccessors, checking each in order.
// This is useful for combining span attributes with resource attributes,
// where span attributes typically take precedence.
type CombinedAccessor struct {
	Accessors []SpanAccessor
}

// GetStringAttribute returns the first non-empty string value from any accessor.
func (a *CombinedAccessor) GetStringAttribute(key string) string {
	for _, accessor := range a.Accessors {
		if v := accessor.GetStringAttribute(key); v != "" {
			return v
		}
	}
	return ""
}

// GetFloat64Attribute returns the first found float64 value from any accessor.
func (a *CombinedAccessor) GetFloat64Attribute(key string) (float64, bool) {
	for _, accessor := range a.Accessors {
		if v, ok := accessor.GetFloat64Attribute(key); ok {
			return v, true
		}
	}
	return 0, false
}

// GetInt64Attribute returns the first found int64 value from any accessor.
func (a *CombinedAccessor) GetInt64Attribute(key string) (int64, bool) {
	for _, accessor := range a.Accessors {
		if v, ok := accessor.GetInt64Attribute(key); ok {
			return v, true
		}
	}
	return 0, false
}

// NewCombinedAccessor creates a CombinedAccessor from the given accessors.
// Accessors are checked in order, so put higher-precedence accessors first.
func NewCombinedAccessor(accessors ...SpanAccessor) *CombinedAccessor {
	return &CombinedAccessor{Accessors: accessors}
}
