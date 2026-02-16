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

// LookupResult contains the result of a semantic attribute lookup.
type LookupResult struct {
	Found        bool    // indicates whether a value was found.
	TagInfo      TagInfo // contains metadata about the matched attribute (use TagInfo.Name for the key).
	StringValue  string
	Float64Value float64
	Int64Value   int64
}

// LookupString looks up a semantic concept and returns the first matching string value.
// It checks attributes in precedence order as defined by the registry.
// For numeric types, converts the value to string.
func LookupString(r Registry, accessor SpanAccessor, concept Concept) string {
	result := Lookup(r, accessor, concept)
	if !result.Found {
		return ""
	}
	switch result.TagInfo.Type {
	case ValueTypeFloat64:
		return strconv.FormatFloat(result.Float64Value, 'f', -1, 64)
	case ValueTypeInt64:
		return strconv.FormatInt(result.Int64Value, 10)
	default:
		return result.StringValue
	}
}

// LookupFloat64 looks up a semantic concept and returns the first matching float64 value.
// It checks attributes in precedence order as defined by the registry.
// Returns 0 and false if no matching attribute is found.
func LookupFloat64(r Registry, accessor SpanAccessor, concept Concept) (float64, bool) {
	result := Lookup(r, accessor, concept)
	if !result.Found {
		return 0, false
	}
	switch result.TagInfo.Type {
	case ValueTypeFloat64:
		return result.Float64Value, true
	case ValueTypeInt64:
		return float64(result.Int64Value), true
	default:
		// Try to parse string value
		if result.StringValue != "" {
			if v, err := strconv.ParseFloat(result.StringValue, 64); err == nil {
				return v, true
			}
		}
		return 0, false
	}
}

// LookupInt64 looks up a semantic concept and returns the first matching int64 value.
// It checks attributes in precedence order as defined by the registry.
// Returns 0 and false if no matching attribute is found.
func LookupInt64(r Registry, accessor SpanAccessor, concept Concept) (int64, bool) {
	result := Lookup(r, accessor, concept)
	if !result.Found {
		return 0, false
	}
	switch result.TagInfo.Type {
	case ValueTypeInt64:
		return result.Int64Value, true
	case ValueTypeFloat64:
		return int64(result.Float64Value), true
	default:
		// Try to parse string value
		if result.StringValue != "" {
			if v, err := strconv.ParseInt(result.StringValue, 10, 64); err == nil {
				return v, true
			}
		}
		return 0, false
	}
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
// Only the value field corresponding to the tag type is populated.
// Type conversions are the caller's responsibility.
func lookupSingleTag(accessor SpanAccessor, tag TagInfo) LookupResult {
	switch tag.Type {
	case ValueTypeFloat64:
		if v, ok := accessor.GetFloat64Attribute(tag.Name); ok {
			return LookupResult{
				Found:        true,
				TagInfo:      tag,
				Float64Value: v,
			}
		}
	case ValueTypeInt64:
		if v, ok := accessor.GetInt64Attribute(tag.Name); ok {
			return LookupResult{
				Found:      true,
				TagInfo:    tag,
				Int64Value: v,
			}
		}
	case ValueTypeString, "":
		if v := accessor.GetStringAttribute(tag.Name); v != "" {
			return LookupResult{
				Found:       true,
				TagInfo:     tag,
				StringValue: v,
			}
		}
	}

	return LookupResult{}
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
