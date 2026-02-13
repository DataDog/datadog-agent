// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

// OTelMapAccessor wraps OTel-style attribute maps (map[string]interface{} or similar)
// to implement SpanAccessor. This is useful for testing or when working with
// pre-extracted OTel attributes.
type OTelMapAccessor struct {
	Attributes map[string]interface{}
}

// GetStringAttribute returns the string value for the given key.
func (a *OTelMapAccessor) GetStringAttribute(key string) string {
	if a.Attributes == nil {
		return ""
	}
	v, ok := a.Attributes[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

// GetFloat64Attribute returns the float64 value for the given key.
func (a *OTelMapAccessor) GetFloat64Attribute(key string) (float64, bool) {
	if a.Attributes == nil {
		return 0, false
	}
	v, ok := a.Attributes[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

// GetInt64Attribute returns the int64 value for the given key.
func (a *OTelMapAccessor) GetInt64Attribute(key string) (int64, bool) {
	if a.Attributes == nil {
		return 0, false
	}
	v, ok := a.Attributes[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case int32:
		return int64(val), true
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	default:
		return 0, false
	}
}

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

// LookupWithFallback performs a semantic attribute lookup across multiple accessors.
// This is useful for OTel spans where you want to check span attributes first,
// then fall back to resource attributes.
func LookupWithFallback(r Registry, concept Concept, accessors ...SpanAccessor) LookupResult {
	combined := NewCombinedAccessor(accessors...)
	return Lookup(r, combined, concept)
}

// LookupStringWithFallback looks up a string value across multiple accessors.
func LookupStringWithFallback(r Registry, concept Concept, accessors ...SpanAccessor) string {
	combined := NewCombinedAccessor(accessors...)
	return LookupString(r, combined, concept)
}

// LookupFloat64WithFallback looks up a float64 value across multiple accessors.
func LookupFloat64WithFallback(r Registry, concept Concept, accessors ...SpanAccessor) (float64, bool) {
	combined := NewCombinedAccessor(accessors...)
	return LookupFloat64(r, combined, concept)
}

// LookupInt64WithFallback looks up an int64 value across multiple accessors.
func LookupInt64WithFallback(r Registry, concept Concept, accessors ...SpanAccessor) (int64, bool) {
	combined := NewCombinedAccessor(accessors...)
	return LookupInt64(r, combined, concept)
}
