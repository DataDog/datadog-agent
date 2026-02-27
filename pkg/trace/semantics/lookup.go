// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"fmt"
	"strconv"
)

// Accessor provides attribute access for semantic lookups.
// Implementations may return not-found from GetInt64/GetFloat64 when the
// underlying store has no numeric type information (e.g. map[string]string).
type Accessor interface {
	GetString(key string) string
	GetInt64(key string) (int64, bool)
	GetFloat64(key string) (float64, bool)
}

// StringMapAccessor adapts a map[string]string into an Accessor.
// GetInt64 and GetFloat64 always return not-found; numeric attributes must be
// registered with type "string" in mappings.json to be reachable via this accessor.
type StringMapAccessor struct {
	m map[string]string
}

// NewStringMapAccessor returns a StringMapAccessor for a map[string]string (e.g. DD span meta).
func NewStringMapAccessor(m map[string]string) StringMapAccessor {
	return StringMapAccessor{m: m}
}

// GetString returns the value for key, or "" if missing or nil map.
func (a StringMapAccessor) GetString(key string) string {
	if a.m == nil {
		return ""
	}
	return a.m[key]
}

// GetInt64 always returns (0, false). map[string]string has no numeric type information,
// so numeric registry entries are intentionally skipped. Use the string-typed entry for
// the same attribute (e.g. http.status_code as "string" in mappings.json) instead.
func (a StringMapAccessor) GetInt64(_ string) (int64, bool) {
	return 0, false
}

// GetFloat64 always returns (0, false) for the same reason as GetInt64.
func (a StringMapAccessor) GetFloat64(_ string) (float64, bool) {
	return 0, false
}

// LookupResult contains the result of a semantic attribute lookup.
type LookupResult struct {
	TagInfo     TagInfo
	StringValue string
}

// Lookup performs a semantic attribute lookup in precedence order and returns the first match.
// For string-typed tags it uses GetString; for numeric-typed tags it uses the typed getter and
// formats the result as a string, so LookupString works correctly on any concept regardless of
// the underlying pdata storage type.
func Lookup[A Accessor](r Registry, accessor A, concept Concept) (LookupResult, bool) {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return LookupResult{}, false
	}
	for _, tag := range tags {
		switch tag.Type {
		case ValueTypeInt64:
			if v, ok := accessor.GetInt64(tag.Name); ok {
				return LookupResult{TagInfo: tag, StringValue: strconv.FormatInt(v, 10)}, true
			}
		case ValueTypeFloat64:
			if v, ok := accessor.GetFloat64(tag.Name); ok {
				return LookupResult{TagInfo: tag, StringValue: fmt.Sprintf("%g", v)}, true
			}
		default:
			if v := accessor.GetString(tag.Name); v != "" {
				return LookupResult{TagInfo: tag, StringValue: v}, true
			}
		}
	}
	return LookupResult{}, false
}

// LookupString returns the first matching string value for the concept, or "" if not found.
func LookupString[A Accessor](r Registry, accessor A, concept Concept) string {
	result, ok := Lookup(r, accessor, concept)
	if !ok {
		return ""
	}
	return result.StringValue
}

// LookupFloat64 returns the first matching value as float64, or (0, false) if not found or unparseable.
// Uses typed access (GetFloat64/GetInt64) for tags with known numeric types, and falls back
// to string parsing for tags with unspecified type.
func LookupFloat64[A Accessor](r Registry, accessor A, concept Concept) (float64, bool) {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return 0, false
	}
	for _, tag := range tags {
		switch tag.Type {
		case ValueTypeFloat64:
			if v, ok := accessor.GetFloat64(tag.Name); ok {
				return v, true
			}
		case ValueTypeInt64:
			if v, ok := accessor.GetInt64(tag.Name); ok {
				return float64(v), true
			}
		default:
			if v := accessor.GetString(tag.Name); v != "" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}

// LookupInt64 returns the first matching value as int64, or (0, false) if not found or unparseable.
// Uses typed access (GetInt64/GetFloat64) for tags with known numeric types, and falls back
// to string parsing for tags with unspecified type.
func LookupInt64[A Accessor](r Registry, accessor A, concept Concept) (int64, bool) {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return 0, false
	}
	for _, tag := range tags {
		switch tag.Type {
		case ValueTypeInt64:
			if v, ok := accessor.GetInt64(tag.Name); ok {
				return v, true
			}
		case ValueTypeFloat64:
			if v, ok := accessor.GetFloat64(tag.Name); ok {
				i := int64(v)
				if float64(i) == v {
					return i, true
				}
			}
		default:
			if v := accessor.GetString(tag.Name); v != "" {
				if i, err := strconv.ParseInt(v, 10, 64); err == nil {
					return i, true
				}
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					intVal := int64(f)
					if float64(intVal) == f {
						return intVal, true
					}
				}
			}
		}
	}
	return 0, false
}
