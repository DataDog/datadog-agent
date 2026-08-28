// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"fmt"

	commonpb "go.opentelemetry.io/proto/slim/otlp/common/v1"
	processcontextpb "go.opentelemetry.io/proto/slim/otlp/processcontext/v1development"
)

type ProcessContext = *processcontextpb.ProcessContext

// KeySchemaVersion returns the thread-local record schema version the process
// published, e.g. "nodejs_v1_dev" for the Node.js writer. Absent for the plain
// thread-local writer, which predates the field.
func KeySchemaVersion(ctx ProcessContext) (string, error) {
	return stringAttribute(ctx, "threadlocal.schema_version")
}

// KeyAttributeKeyMap returns the ordered list of attribute key names the process
// published in its OTel process context (OTEP 4947).
func KeyAttributeKeyMap(ctx ProcessContext) ([]string, error) {
	attr := "threadlocal.attribute_key_map"

	value := findAttribute(ctx.GetAttributes(), attr)
	if value == nil {
		return nil, fmt.Errorf("unknown attribute %s", attr)
	}

	entries := value.GetArrayValue().GetValues()
	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = entry.GetStringValue()
	}
	return keys, nil
}

// The V8 layout attributes below are only published by the Node.js writer,
// alongside the schema version, so the reader needs to know nothing of how V8
// was built.

// KeyTaggedSize returns the width in bytes of a V8 tagged word.
func KeyTaggedSize(ctx ProcessContext) (int64, error) {
	return intAttribute(ctx, "threadlocal.tagged_size")
}

// KeyJSMapTableOffset returns the offset of the backing table pointer within a
// JSMap.
func KeyJSMapTableOffset(ctx ProcessContext) (int64, error) {
	return intAttribute(ctx, "threadlocal.js_map_table_offset")
}

// KeyOrderedHashMapHeaderSize returns the size of the header preceding the
// fields of an OrderedHashMap.
func KeyOrderedHashMapHeaderSize(ctx ProcessContext) (int64, error) {
	return intAttribute(ctx, "threadlocal.ordered_hash_map_header_size")
}

// KeyJSObjectRecordOffset returns the offset of internal field 0 within the
// JSObject wrapping a record, which holds the record pointer.
func KeyJSObjectRecordOffset(ctx ProcessContext) (int64, error) {
	return intAttribute(ctx, "threadlocal.js_object_record_offset")
}

// stringAttribute returns the value of the string attribute named key, or an
// error if ctx published none by that name.
func stringAttribute(ctx ProcessContext, key string) (string, error) {
	value := findAttribute(ctx.GetAttributes(), key)
	if value == nil {
		return "", fmt.Errorf("unknown attribute %s", key)
	}
	return value.GetStringValue(), nil
}

// intAttribute returns the value of the integer attribute named key, or an
// error if ctx published none by that name.
func intAttribute(ctx ProcessContext, key string) (int64, error) {
	value := findAttribute(ctx.GetAttributes(), key)
	if value == nil {
		return 0, fmt.Errorf("unknown attribute %s", key)
	}
	return value.GetIntValue(), nil
}

// findAttribute returns the value of the attribute named key among attrs, or nil if
// attrs has none by that name.
func findAttribute(attrs []*commonpb.KeyValue, key string) *commonpb.AnyValue {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			return kv.GetValue()
		}
	}
	return nil
}
