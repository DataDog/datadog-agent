// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

// KeyAttributeKeyMap is the attribute key under which an OTEP 4947 writer publishes,
// alongside its records, the key names a reader needs to decode them.
const KeyAttributeKeyMap = "threadlocal.attribute_key_map"

// ProcessContext is the decoded content of the process context a process publishes.
type ProcessContext struct {
	// Resource describes the process itself, in OpenTelemetry semantic conventions:
	// service.name, deployment.environment.name, telemetry.sdk.language, ...
	Resource Attributes
	// Attributes are the ones the publisher shares outside the standard resource,
	// among them the thread-local block this reader is after.
	Attributes Attributes
}

// ValueKind tells which field of a Value is populated.
type ValueKind uint8

// The value kinds this reader keeps. Anything else in the payload is dropped while
// decoding, since the process context is only read for the thread-local block.
const (
	// ValueUnset is the zero value, returned for attributes that are not present.
	ValueUnset ValueKind = iota
	// ValueString is a string_value.
	ValueString
	// ValueInt is an int_value.
	ValueInt
	// ValueStringSlice is an array_value whose entries are all string_values.
	ValueStringSlice
)

// Value is one decoded attribute value.
type Value struct {
	Kind        ValueKind
	Str         string
	Int         int64
	StringSlice []string
}

// Attributes are the extra attributes of a process context, keyed by attribute name.
type Attributes map[string]Value

// String returns the value of a string attribute.
func (a Attributes) String(key string) (string, bool) {
	v, ok := a[key]
	if !ok || v.Kind != ValueString {
		return "", false
	}
	return v.Str, true
}

// Int returns the value of an integer attribute.
func (a Attributes) Int(key string) (int64, bool) {
	v, ok := a[key]
	if !ok || v.Kind != ValueInt {
		return 0, false
	}
	return v.Int, true
}

// StringSlice returns the value of a string array attribute.
func (a Attributes) StringSlice(key string) ([]string, bool) {
	v, ok := a[key]
	if !ok || v.Kind != ValueStringSlice {
		return nil, false
	}
	return v.StringSlice, true
}
