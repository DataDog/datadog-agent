// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"fmt"

	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
)

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

	value := findAttribute(ctx, attr)
	if value == nil {
		return nil, fmt.Errorf("unknown attribute %s", attr)
	}

	keys := make([]string, 0, len(value.array))
	return append(keys, value.array...), nil
}

// TracerMetadata mirrors libdatadog's TracerMetadata::to_otel_process_ctx.
// SchemaVersion and LogsCollected have no process context counterpart.
func TracerMetadata(ctx ProcessContext) tracermetadata.TracerMetadata {
	var resource, attributes []keyValue
	if ctx != nil {
		resource, attributes = ctx.resource, ctx.attributes
	}
	return tracermetadata.TracerMetadata{
		RuntimeID:      findAttributeIn(resource, "service.instance.id").stringValue(),
		TracerLanguage: findAttributeIn(resource, "telemetry.sdk.language").stringValue(),
		TracerVersion:  findAttributeIn(resource, "telemetry.sdk.version").stringValue(),
		Hostname:       findAttributeIn(resource, "host.name").stringValue(),
		ServiceName:    findAttributeIn(resource, "service.name").stringValue(),
		ServiceEnv:     findAttributeIn(resource, "deployment.environment.name").stringValue(),
		ServiceVersion: findAttributeIn(resource, "service.version").stringValue(),
		ContainerID:    findAttributeIn(resource, "container.id").stringValue(),
		ProcessTags:    findAttributeIn(attributes, "datadog.process_tags").stringValue(),
	}
}

// stringAttribute returns the value of the string attribute named key, or an
// error if ctx published none by that name.
func stringAttribute(ctx ProcessContext, key string) (string, error) {
	value := findAttribute(ctx, key)
	if value == nil {
		return "", fmt.Errorf("unknown attribute %s", key)
	}
	return value.stringValue(), nil
}

// stringValue returns the string member of v, or "" if v is nil or holds another.
func (v *anyValue) stringValue() string {
	if v == nil || v.kind != anyValueString {
		return ""
	}
	return v.str
}

// findAttribute returns the value of the first attribute named key in ctx, or nil if
// ctx has none by that name or it carries no value.
func findAttribute(ctx ProcessContext, key string) *anyValue {
	if ctx == nil {
		return nil
	}
	return findAttributeIn(ctx.attributes, key)
}

// findAttributeIn returns the value of the first attribute named key among attrs,
// or nil if attrs has none by that name or it carries no value.
func findAttributeIn(attrs []keyValue, key string) *anyValue {
	for _, kv := range attrs {
		if kv.key == key {
			return kv.value
		}
	}
	return nil
}
