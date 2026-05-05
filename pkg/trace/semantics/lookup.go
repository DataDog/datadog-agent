// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"fmt"
	"math"
	"strconv"
)

// Accessor provides attribute access for semantic lookups.
// Implementations may return not-found from GetInt64/GetFloat64 when the
// underlying store has no numeric type information (e.g. map[string]string),
// or not-found from GetString when the store has no string values (e.g. map[string]float64).
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

// MetricsMapAccessor adapts a map[string]float64 into an Accessor (e.g. DD span Metrics).
// GetString always returns ""; string-typed registry entries are intentionally skipped
// because map[string]float64 holds no string values. Use DDSpanAccessor, which routes
// string lookups to StringMapAccessor (meta) instead.
type MetricsMapAccessor struct {
	m map[string]float64
}

// NewMetricsMapAccessor returns a MetricsMapAccessor for a map[string]float64.
func NewMetricsMapAccessor(m map[string]float64) MetricsMapAccessor {
	return MetricsMapAccessor{m: m}
}

// GetString always returns "". map[string]float64 holds no string values.
func (a MetricsMapAccessor) GetString(_ string) string { return "" }

// GetFloat64 returns the value directly, or (0, false) if missing or nil map.
func (a MetricsMapAccessor) GetFloat64(key string) (float64, bool) {
	if a.m == nil {
		return 0, false
	}
	v, ok := a.m[key]
	return v, ok
}

// GetInt64 converts the float64 value to int64 if it is an exact integer, or (0, false) otherwise.
func (a MetricsMapAccessor) GetInt64(key string) (int64, bool) {
	v, ok := a.GetFloat64(key)
	if !ok {
		return 0, false
	}
	i := int64(v)
	if float64(i) != v || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return i, true
}

// LookupResult contains the result of a semantic attribute lookup.
type LookupResult struct {
	TagInfo     TagInfo
	StringValue string
}

func readTag[A Accessor](accessor A, tag TagInfo) (LookupResult, bool) {
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
	return LookupResult{}, false
}

func readFloat64Tag[A Accessor](accessor A, tag TagInfo) (float64, bool) {
	switch tag.Type {
	case ValueTypeFloat64:
		return accessor.GetFloat64(tag.Name)
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
	return 0, false
}

func readInt64Tag[A Accessor](accessor A, tag TagInfo) (int64, bool) {
	switch tag.Type {
	case ValueTypeInt64:
		return accessor.GetInt64(tag.Name)
	case ValueTypeFloat64:
		if v, ok := accessor.GetFloat64(tag.Name); ok {
			return exactInt64(v)
		}
	default:
		if v := accessor.GetString(tag.Name); v != "" {
			if i, err := strconv.ParseInt(v, 10, 64); err == nil {
				return i, true
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return exactInt64(f)
			}
		}
	}
	return 0, false
}

func exactInt64(v float64) (int64, bool) {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	i := int64(v)
	return i, float64(i) == v
}

// conditionMatches evaluates a single Condition against the accessor's raw attribute store.
// Unlike concept lookup, conditions read attributes by exact key — fallback chains are not
// followed. To gate on a renamed attribute (e.g. rpc.system → rpc.system.name), list one
// fallback entry per condition attribute in mappings.json.
func conditionMatches[A Accessor](accessor A, c Condition) bool {
	v := accessor.GetString(c.Attribute)
	found := v != ""
	if c.Present != nil && found != *c.Present {
		return false
	}
	if c.Eq != nil {
		return found && v == fmt.Sprint(c.Eq)
	}
	if c.Present != nil {
		return true
	}
	return found
}

func conditionsMatch[A Accessor](accessor A, conditions []Condition) bool {
	for _, condition := range conditions {
		if !conditionMatches(accessor, condition) {
			return false
		}
	}
	return true
}

func lookupWithConditions[A Accessor, T any](
	r Registry,
	accessor A,
	concept Concept,
	read func(TagInfo) (T, bool),
) (T, bool) {
	var zero T
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return zero, false
	}
	for _, tag := range tags {
		if !conditionsMatch(accessor, tag.When) {
			continue
		}
		if result, ok := read(tag); ok {
			return result, true
		}
	}
	return zero, false
}

// Lookup performs a semantic attribute lookup in precedence order and returns the first match.
// For string-typed tags it uses GetString; for numeric-typed tags it uses the typed getter and
// formats the result as a string, so LookupString works correctly on any concept regardless of
// the underlying pdata storage type.
func Lookup[A Accessor](r Registry, accessor A, concept Concept) (LookupResult, bool) {
	return lookupWithConditions(r, accessor, concept, func(tag TagInfo) (LookupResult, bool) {
		return readTag(accessor, tag)
	})
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
	return lookupWithConditions(r, accessor, concept, func(tag TagInfo) (float64, bool) {
		return readFloat64Tag(accessor, tag)
	})
}

// LookupInt64 returns the first matching value as int64, or (0, false) if not found or unparseable.
// Uses typed access (GetInt64/GetFloat64) for tags with known numeric types, and falls back
// to string parsing for tags with unspecified type.
func LookupInt64[A Accessor](r Registry, accessor A, concept Concept) (int64, bool) {
	return lookupWithConditions(r, accessor, concept, func(tag TagInfo) (int64, bool) {
		return readInt64Tag(accessor, tag)
	})
}
