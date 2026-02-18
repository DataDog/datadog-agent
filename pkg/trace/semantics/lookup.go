// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"strconv"
)

// AttrGetter returns the attribute value as a string for the given key, or "" if missing.
// Callers use NewStringMapAccessor, NewPDataMapAccessor, or NewCombinedAccessor to create one.
type AttrGetter func(key string) string

// NewStringMapAccessor returns an AttrGetter for a map[string]string (e.g. DD span meta).
func NewStringMapAccessor(m map[string]string) AttrGetter {
	return func(key string) string {
		if m == nil {
			return ""
		}
		return m[key]
	}
}

// NewCombinedAccessor returns an AttrGetter that tries each in order; first non-empty value wins.
// Put higher-precedence accessors first (e.g. span attrs then resource attrs).
func NewCombinedAccessor(accessors ...AttrGetter) AttrGetter {
	return func(key string) string {
		for _, accessor := range accessors {
			if v := accessor(key); v != "" {
				return v
			}
		}
		return ""
	}
}

// LookupResult contains the result of a semantic attribute lookup.
type LookupResult struct {
	TagInfo     TagInfo
	StringValue string
}

// Lookup performs a semantic attribute lookup in precedence order and returns the first match.
func Lookup(r Registry, accessor AttrGetter, concept Concept) (LookupResult, bool) {
	tags := r.GetAttributePrecedence(concept)
	if tags == nil {
		return LookupResult{}, false
	}
	for _, tag := range tags {
		if v := accessor(tag.Name); v != "" {
			return LookupResult{TagInfo: tag, StringValue: v}, true
		}
	}
	return LookupResult{}, false
}

// LookupString returns the first matching string value for the concept, or "" if not found.
func LookupString(r Registry, accessor AttrGetter, concept Concept) string {
	result, ok := Lookup(r, accessor, concept)
	if !ok {
		return ""
	}
	return result.StringValue
}

// LookupFloat64 returns the first matching value as float64, or (0, false) if not found or unparseable.
func LookupFloat64(r Registry, accessor AttrGetter, concept Concept) (float64, bool) {
	result, ok := Lookup(r, accessor, concept)
	if !ok || result.StringValue == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(result.StringValue, 64)
	if err != nil {
		if i, errInt := strconv.ParseInt(result.StringValue, 10, 64); errInt == nil {
			return float64(i), true
		}
		return 0, false
	}
	return v, true
}

// LookupInt64 returns the first matching value as int64, or (0, false) if not found or unparseable.
// Accepts integer-valued floats (e.g. "200.0" or float 200).
func LookupInt64(r Registry, accessor AttrGetter, concept Concept) (int64, bool) {
	result, ok := Lookup(r, accessor, concept)
	if !ok || result.StringValue == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(result.StringValue, 10, 64)
	if err == nil {
		return v, true
	}
	f, err := strconv.ParseFloat(result.StringValue, 64)
	if err != nil {
		return 0, false
	}
	intVal := int64(f)
	if float64(intVal) != f {
		return 0, false
	}
	return intVal, true
}
