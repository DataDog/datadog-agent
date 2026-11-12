// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/slim/otlp/common/v1"
	processcontextpb "go.opentelemetry.io/proto/slim/otlp/processcontext/v1development"
	resourcepb "go.opentelemetry.io/proto/slim/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
)

const (
	schemaAttr = "threadlocal.schema_version"
	keyMapAttr = "threadlocal.attribute_key_map"
)

// accessorResult is what the accessors return for a payload.
type accessorResult struct {
	malformed bool
	schema    string
	schemaErr bool
	keyMap    []string
	keyMapErr bool
	tracer    tracermetadata.TracerMetadata
}

// reference decodes payload with proto.Unmarshal and the generated types, which is
// what decodeProcessContext replaces.
func reference(payload []byte) accessorResult {
	var pb processcontextpb.ProcessContext
	if err := proto.Unmarshal(payload, &pb); err != nil {
		return accessorResult{malformed: true}
	}
	findIn := func(attrs []*commonpb.KeyValue, key string) *commonpb.AnyValue {
		for _, kv := range attrs {
			if kv.GetKey() == key {
				return kv.GetValue()
			}
		}
		return nil
	}
	find := func(key string) *commonpb.AnyValue { return findIn(pb.GetAttributes(), key) }
	resource := func(key string) string { return findIn(pb.GetResource().GetAttributes(), key).GetStringValue() }

	var res accessorResult
	if v := find(schemaAttr); v != nil {
		res.schema = v.GetStringValue()
	} else {
		res.schemaErr = true
	}
	if v := find(keyMapAttr); v != nil {
		res.keyMap = []string{}
		for _, e := range v.GetArrayValue().GetValues() {
			res.keyMap = append(res.keyMap, e.GetStringValue())
		}
	} else {
		res.keyMapErr = true
	}
	res.tracer = tracermetadata.TracerMetadata{
		RuntimeID:      resource("service.instance.id"),
		TracerLanguage: resource("telemetry.sdk.language"),
		TracerVersion:  resource("telemetry.sdk.version"),
		Hostname:       resource("host.name"),
		ServiceName:    resource("service.name"),
		ServiceEnv:     resource("deployment.environment.name"),
		ServiceVersion: resource("service.version"),
		ContainerID:    resource("container.id"),
		ProcessTags:    find("datadog.process_tags").GetStringValue(),
	}
	return res
}

func decoded(payload []byte) accessorResult {
	ctx, err := decodeProcessContext(payload)
	if err != nil {
		if !errors.Is(err, ErrMalformed) {
			panic(err)
		}
		return accessorResult{malformed: true}
	}

	var res accessorResult
	res.schema, err = KeySchemaVersion(ctx)
	res.schemaErr = err != nil
	res.keyMap, err = KeyAttributeKeyMap(ctx)
	res.keyMapErr = err != nil
	res.tracer = TracerMetadata(ctx)
	return res
}

func str(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

func array(values ...*commonpb.AnyValue) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: values}}}
}

func marshal(t *testing.T, attrs ...*commonpb.KeyValue) []byte {
	t.Helper()
	b, err := proto.Marshal(&processcontextpb.ProcessContext{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "service.name", Value: str("svc")},
			{Key: "service.instance.id", Value: str("rid")},
			{Key: "telemetry.sdk.language", Value: str("nodejs")},
			{Key: "telemetry.sdk.version", Value: str("5.0.0")},
			{Key: "host.name", Value: str("host")},
			{Key: "deployment.environment.name", Value: str("prod")},
			{Key: "service.version", Value: str("1.2.3")},
			{Key: "container.id", Value: str("cid")},
			{Key: "service.version", Value: str("ignored, first wins")},
			{Key: "host.name"},
			{Key: "nested", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{
				KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{{Key: "k", Value: str("v")}}},
			}}},
		}},
		Attributes: attrs,
	})
	require.NoError(t, err)
	return b
}

// field encodes one length-delimited field.
func field(num protowire.Number, v []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, num, protowire.BytesType), v)
}

func concat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

func TestDecodeProcessContextMatchesProtoUnmarshal(t *testing.T) {
	typical := marshal(t,
		&commonpb.KeyValue{Key: "other", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}},
		&commonpb.KeyValue{Key: schemaAttr, Value: str("nodejs_v1_dev")},
		&commonpb.KeyValue{Key: keyMapAttr, Value: array(str("http.route"), str("user.id"))},
	)

	stringValue := func(s string) []byte { return field(anyValueString, []byte(s)) }
	arrayOf := func(elems ...[]byte) []byte {
		var values []byte
		for _, e := range elems {
			values = append(values, field(arrayValueValues, e)...)
		}
		return field(anyValueArray, values)
	}
	attribute := func(parts ...[]byte) []byte { return field(processContextAttributes, concat(parts...)) }
	key := func(k string) []byte { return field(keyValueKey, []byte(k)) }
	value := func(v []byte) []byte { return field(keyValueValue, v) }
	resource := func(attrs ...[]byte) []byte {
		var b []byte
		for _, a := range attrs {
			b = append(b, field(resourceAttributes, a)...)
		}
		return field(processContextResource, b)
	}

	cases := map[string][]byte{
		"typical":       typical,
		"empty":         {},
		"no attributes": marshal(t),
		"missing value": marshal(t,
			&commonpb.KeyValue{Key: schemaAttr},
			&commonpb.KeyValue{Key: keyMapAttr},
		),
		"wrong member types": marshal(t,
			&commonpb.KeyValue{Key: schemaAttr, Value: array(str("x"))},
			&commonpb.KeyValue{Key: keyMapAttr, Value: str("x")},
		),
		"non-string elements": marshal(t,
			&commonpb.KeyValue{Key: keyMapAttr, Value: array(
				str("a"),
				&commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}},
				array(str("nested")),
				&commonpb.AnyValue{},
				str("b"),
			)},
		),
		"empty array": marshal(t, &commonpb.KeyValue{Key: keyMapAttr, Value: array()}),
		"duplicate key, first wins": marshal(t,
			&commonpb.KeyValue{Key: schemaAttr, Value: str("first")},
			&commonpb.KeyValue{Key: schemaAttr, Value: str("second")},
		),
		"repeated key field, last wins": attribute(key("ignored"), key(schemaAttr), value(stringValue("v"))),
		"repeated value merges arrays": attribute(key(keyMapAttr),
			value(arrayOf(stringValue("a"))), value(arrayOf(stringValue("b")))),
		"repeated string, last wins": attribute(key(schemaAttr), value(concat(stringValue("a"), stringValue("b")))),
		"string then array":          attribute(key(schemaAttr), value(concat(stringValue("a"), arrayOf(stringValue("b"))))),
		"array, string, array resets": attribute(key(keyMapAttr),
			value(concat(arrayOf(stringValue("a")), stringValue("s"), arrayOf(stringValue("b"))))),
		"string then bool": attribute(key(schemaAttr),
			value(concat(stringValue("a"), protowire.AppendVarint(protowire.AppendTag(nil, anyValueBool, protowire.VarintType), 1)))),
		"member with wrong wire type is unknown": attribute(key(schemaAttr),
			value(concat(stringValue("a"), protowire.AppendVarint(protowire.AppendTag(nil, anyValueString, protowire.VarintType), 1)))),
		"attributes with wrong wire type is unknown": concat(
			protowire.AppendVarint(protowire.AppendTag(nil, processContextAttributes, protowire.VarintType), 7), typical),
		"unknown fields everywhere": concat(
			protowire.AppendFixed32(protowire.AppendTag(nil, 99, protowire.Fixed32Type), 1),
			attribute(key(schemaAttr), field(99, []byte("x")), value(concat(field(99, nil), stringValue("v")))),
		),
		"unknown group": concat(
			protowire.AppendTag(nil, 50, protowire.StartGroupType),
			protowire.AppendVarint(protowire.AppendTag(nil, 1, protowire.VarintType), 3),
			protowire.AppendTag(nil, 50, protowire.EndGroupType),
			typical),
		"stray end group":       concat(protowire.AppendTag(nil, 50, protowire.EndGroupType), typical),
		"field number zero":     concat([]byte{0x02, 0x00}, typical),
		"overlong varint":       concat(protowire.AppendTag(nil, 99, protowire.VarintType), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}),
		"invalid UTF-8 key":     attribute(key("\xff"), value(stringValue("v"))),
		"invalid UTF-8 value":   attribute(key(schemaAttr), value(stringValue("\xff"))),
		"invalid UTF-8 element": attribute(key(keyMapAttr), value(arrayOf(stringValue("\xff")))),
		"invalid UTF-8 in overwritten string": attribute(key(schemaAttr),
			value(concat(stringValue("\xff"), stringValue("ok")))),
		"process tags": marshal(t,
			&commonpb.KeyValue{Key: "datadog.process_tags", Value: str("entrypoint.name:app")},
		),
		"non-string resource attributes": concat(resource(
			concat(key("service.name"), value(protowire.AppendVarint(protowire.AppendTag(nil, anyValueInt, protowire.VarintType), 7))),
			concat(key("host.name"), value(arrayOf(stringValue("h")))),
		), typical),
		"repeated resource merges": concat(
			resource(concat(key("service.name"), value(stringValue("a")))),
			typical,
			resource(concat(key("host.name"), value(stringValue("b")))),
		),
		"resource with wrong wire type is unknown": concat(
			protowire.AppendVarint(protowire.AppendTag(nil, processContextResource, protowire.VarintType), 1), typical),
		"resource attribute with wrong wire type is unknown": concat(field(processContextResource, concat(
			protowire.AppendVarint(protowire.AppendTag(nil, resourceAttributes, protowire.VarintType), 1),
			field(resourceAttributes, concat(key("service.name"), value(stringValue("s")))),
		)), typical),
		"unknown resource fields": concat(resource(), field(processContextResource, concat(
			protowire.AppendVarint(protowire.AppendTag(nil, 2, protowire.VarintType), 3),
			field(resourceAttributes, concat(key("service.name"), value(stringValue("s")))),
		)), typical),
		"invalid UTF-8 resource key":   concat(resource(concat(key("\xff"), value(stringValue("v")))), typical),
		"invalid UTF-8 resource value": concat(resource(concat(key("service.name"), value(stringValue("\xff")))), typical),
		"malformed resource attribute": concat(field(processContextResource, field(resourceAttributes, []byte{0x0a, 0x05})), typical),
		"malformed array element":      attribute(key(keyMapAttr), value(field(anyValueArray, field(arrayValueValues, []byte{0x0a, 0x05})))),
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, reference(payload), decoded(payload))
		})
	}

	// Every truncation of a typical payload either ends on a field boundary or cuts
	// through the outermost field, and both decoders must agree on which.
	for n := range typical {
		require.Equal(t, reference(typical[:n]), decoded(typical[:n]), "truncated to %d bytes", n)
	}
}

func TestDecodeProcessContextTypical(t *testing.T) {
	payload := marshal(t,
		&commonpb.KeyValue{Key: schemaAttr, Value: str("nodejs_v1_dev")},
		&commonpb.KeyValue{Key: keyMapAttr, Value: array(str("http.route"), str("user.id"))},
	)
	require.Equal(t, accessorResult{
		schema: "nodejs_v1_dev",
		keyMap: []string{"http.route", "user.id"},
		tracer: tracermetadata.TracerMetadata{
			RuntimeID:      "rid",
			TracerLanguage: "nodejs",
			TracerVersion:  "5.0.0",
			Hostname:       "host",
			ServiceName:    "svc",
			ServiceEnv:     "prod",
			ServiceVersion: "1.2.3",
			ContainerID:    "cid",
		},
	}, decoded(payload))
}
