// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"strconv"
)

// SpanAccessor provides access to span attributes for semantic lookups.
// This interface abstracts the underlying span representation (DD spans, OTel spans, etc.)
// to allow the semantic lookup functions to work with any span type.
type SpanAccessor interface {
	// GetStringAttribute returns the string value for the given key, or empty string if not found.
	GetStringAttribute(key string) string

	// GetFloat64Attribute returns the float64 value for the given key and whether it was found.
	GetFloat64Attribute(key string) (float64, bool)

	// GetInt64Attribute returns the int64 value for the given key and whether it was found.
	GetInt64Attribute(key string) (int64, bool)
}

// DDSpanAccessor wraps DD span Meta and Metrics maps to implement SpanAccessor.
type DDSpanAccessor struct {
	Meta    map[string]string
	Metrics map[string]float64
}

// GetStringAttribute returns the string value from Meta for the given key.
func (a *DDSpanAccessor) GetStringAttribute(key string) string {
	if a.Meta == nil {
		return ""
	}
	return a.Meta[key]
}

// GetFloat64Attribute returns the float64 value from Metrics for the given key.
func (a *DDSpanAccessor) GetFloat64Attribute(key string) (float64, bool) {
	if a.Metrics == nil {
		return 0, false
	}
	v, ok := a.Metrics[key]
	return v, ok
}

// GetInt64Attribute returns the int64 value from Metrics for the given key (converted from float64).
func (a *DDSpanAccessor) GetInt64Attribute(key string) (int64, bool) {
	if a.Metrics == nil {
		return 0, false
	}
	v, ok := a.Metrics[key]
	if !ok {
		return 0, false
	}
	return int64(v), true
}

// LookupResult contains the result of a semantic attribute lookup.
type LookupResult struct {
	Found        bool    // indicates whether a value was found.
	Key          string  // actual attribute key that matched.
	TagInfo      TagInfo // contains metadata about the matched attribute.
	StringValue  string
	Float64Value float64
	Int64Value   int64
}

// LookupString looks up a semantic concept and returns the first matching string value.
// It checks attributes in precedence order as defined by the registry.
func LookupString(r Registry, accessor SpanAccessor, concept Concept) string {
	result := Lookup(r, accessor, concept)
	if !result.Found {
		return ""
	}
	return result.StringValue
}

// LookupFloat64 looks up a semantic concept and returns the first matching float64 value.
// It checks attributes in precedence order as defined by the registry.
// Returns 0 and false if no matching attribute is found.
func LookupFloat64(r Registry, accessor SpanAccessor, concept Concept) (float64, bool) {
	result := Lookup(r, accessor, concept)
	if !result.Found {
		return 0, false
	}
	// If the TagInfo says it's a float64 type and we found it in metrics, use that
	if result.TagInfo.Type == ValueTypeFloat64 || result.TagInfo.Type == ValueTypeInt64 {
		return result.Float64Value, true
	}
	// Otherwise try to parse the string value
	if result.StringValue != "" {
		if v, err := strconv.ParseFloat(result.StringValue, 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

// LookupInt64 looks up a semantic concept and returns the first matching int64 value.
// It checks attributes in precedence order as defined by the registry.
// Returns 0 and false if no matching attribute is found.
func LookupInt64(r Registry, accessor SpanAccessor, concept Concept) (int64, bool) {
	result := Lookup(r, accessor, concept)
	if !result.Found {
		return 0, false
	}
	// If the TagInfo says it's an int64 type and we found it in metrics, use that
	if result.TagInfo.Type == ValueTypeInt64 {
		return result.Int64Value, true
	}
	if result.TagInfo.Type == ValueTypeFloat64 {
		return int64(result.Float64Value), true
	}
	// Otherwise try to parse the string value
	if result.StringValue != "" {
		if v, err := strconv.ParseInt(result.StringValue, 10, 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

// Lookup performs a semantic attribute lookup and returns detailed information about the match.
// It checks attributes in precedence order as defined by the registry.
func Lookup(r Registry, accessor SpanAccessor, concept Concept) LookupResult {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return LookupResult{}
	}

	for _, tag := range tags {
		result := lookupSingleTag(accessor, tag)
		if result.Found {
			return result
		}
	}

	return LookupResult{}
}

// lookupSingleTag looks up a single tag from the accessor based on its type.
func lookupSingleTag(accessor SpanAccessor, tag TagInfo) LookupResult {
	switch tag.Type {
	case ValueTypeFloat64:
		if v, ok := accessor.GetFloat64Attribute(tag.Name); ok {
			return LookupResult{
				Found:        true,
				Key:          tag.Name,
				TagInfo:      tag,
				StringValue:  strconv.FormatFloat(v, 'f', -1, 64),
				Float64Value: v,
				Int64Value:   int64(v),
			}
		}
	case ValueTypeInt64:
		if v, ok := accessor.GetInt64Attribute(tag.Name); ok {
			return LookupResult{
				Found:        true,
				Key:          tag.Name,
				TagInfo:      tag,
				StringValue:  strconv.FormatInt(v, 10),
				Float64Value: float64(v),
				Int64Value:   v,
			}
		}
		// Also check float64 for int64 types (DD spans store ints as float64 in Metrics)
		if v, ok := accessor.GetFloat64Attribute(tag.Name); ok {
			return LookupResult{
				Found:        true,
				Key:          tag.Name,
				TagInfo:      tag,
				StringValue:  strconv.FormatInt(int64(v), 10),
				Float64Value: v,
				Int64Value:   int64(v),
			}
		}
	case ValueTypeString, "":
		if v := accessor.GetStringAttribute(tag.Name); v != "" {
			return LookupResult{
				Found:       true,
				Key:         tag.Name,
				TagInfo:     tag,
				StringValue: v,
			}
		}
	}

	return LookupResult{}
}

// LookupFromMaps is a convenience function that looks up a semantic concept from DD span maps.
// This is the most common use case for DD spans.
func LookupFromMaps(r Registry, meta map[string]string, metrics map[string]float64, concept Concept) LookupResult {
	accessor := &DDSpanAccessor{Meta: meta, Metrics: metrics}
	return Lookup(r, accessor, concept)
}

// LookupStringFromMaps is a convenience function that looks up a string value from DD span maps.
func LookupStringFromMaps(r Registry, meta map[string]string, metrics map[string]float64, concept Concept) string {
	accessor := &DDSpanAccessor{Meta: meta, Metrics: metrics}
	return LookupString(r, accessor, concept)
}

// LookupFloat64FromMaps is a convenience function that looks up a float64 value from DD span maps.
func LookupFloat64FromMaps(r Registry, meta map[string]string, metrics map[string]float64, concept Concept) (float64, bool) {
	accessor := &DDSpanAccessor{Meta: meta, Metrics: metrics}
	return LookupFloat64(r, accessor, concept)
}

// LookupInt64FromMaps is a convenience function that looks up an int64 value from DD span maps.
func LookupInt64FromMaps(r Registry, meta map[string]string, metrics map[string]float64, concept Concept) (int64, bool) {
	accessor := &DDSpanAccessor{Meta: meta, Metrics: metrics}
	return LookupInt64(r, accessor, concept)
}

// GetAttributeKeys returns all attribute keys for a concept in precedence order.
// This is useful for iterating over possible keys without performing a lookup.
func GetAttributeKeys(r Registry, concept Concept) []string {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return nil
	}
	keys := make([]string, len(tags))
	for i, tag := range tags {
		keys[i] = tag.Name
	}
	return keys
}

// GetAttributeKeysForType returns attribute keys for a concept filtered by value type.
// This is useful when you only want to check certain types of attributes.
func GetAttributeKeysForType(r Registry, concept Concept, valueType ValueType) []string {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return nil
	}
	var keys []string
	for _, tag := range tags {
		if tag.Type == valueType || (valueType == ValueTypeString && tag.Type == "") {
			keys = append(keys, tag.Name)
		}
	}
	return keys
}
