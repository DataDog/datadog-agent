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
// Implementations must supply all three methods. For string-only backends
// (e.g. map[string]string), the numeric methods parse from GetString.
type Accessor interface {
	GetString(key string) string
	GetInt64(key string) (int64, bool)
	GetFloat64(key string) (float64, bool)
}

// StringMapAccessor adapts a map[string]string into an Accessor.
// Numeric methods parse from the string values.
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

// GetInt64 parses the string value as int64.
func (a StringMapAccessor) GetInt64(key string) (int64, bool) {
	v := a.GetString(key)
	if v == "" {
		return 0, false
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

// GetFloat64 parses the string value as float64.
func (a StringMapAccessor) GetFloat64(key string) (float64, bool) {
	v := a.GetString(key)
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
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
