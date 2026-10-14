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

// stringAttribute returns the value of the string attribute named key, or an
// error if ctx published none by that name.
func stringAttribute(ctx ProcessContext, key string) (string, error) {
	value := findAttribute(ctx.GetAttributes(), key)
	if value == nil {
		return "", fmt.Errorf("unknown attribute %s", key)
	}
	return value.GetStringValue(), nil
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
