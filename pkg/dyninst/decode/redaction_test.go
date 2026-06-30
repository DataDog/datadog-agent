// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
)

// TestDecoderRedaction decodes a map[string]int argument (key "a" => 1) under
// various redaction policies and checks the captured value is scrubbed.
func TestDecoderRedaction(t *testing.T) {
	decodeArgs := func(t *testing.T, red *redaction.Config) any {
		irProg := generateIrForProbes(t, "simple", goVersionSwissMap, "mapArg")
		irProg.Redaction = red
		item := simpleSwissMapArgEvent(t, irProg)
		decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
		require.NoError(t, err)
		buf, _, err := decoder.Decode(Event{
			EntryOrLine: output.SingleEvent(item),
			ServiceName: "foo",
		}, &noopSymbolicator{}, nil, []byte{})
		require.NoError(t, err)
		var e eventCaptures
		require.NoError(t, json.Unmarshal(buf, &e))
		return e.Debugger.Snapshot.Captures.Entry.Arguments
	}

	expectedMap := func(value map[string]any) map[string]any {
		return map[string]any{
			"m": map[string]any{
				"type": "map[string]int",
				"size": "1",
				"entries": []any{[]any{
					map[string]any{"type": "string", "value": "a"},
					value,
				}},
			},
		}
	}

	t.Run("value redacted by matching key", func(t *testing.T) {
		got := decodeArgs(t, redaction.NewConfig([]string{"a"}, nil, nil))
		require.Equal(t, expectedMap(map[string]any{
			"type": "int", "notCapturedReason": "redactedIdent",
		}), got)
	})

	t.Run("value redacted by type", func(t *testing.T) {
		got := decodeArgs(t, redaction.NewConfig(nil, []string{"int"}, nil))
		require.Equal(t, expectedMap(map[string]any{
			"type": "int", "notCapturedReason": "redactedType",
		}), got)
	})

	t.Run("non-matching key leaves value intact", func(t *testing.T) {
		got := decodeArgs(t, redaction.NewConfig([]string{"password"}, nil, nil))
		require.Equal(t, expectedMap(map[string]any{
			"type": "int", "value": "1",
		}), got)
	})
}

// decodeMapArg decodes the mapArg probe (map[string]int, "a" => 1) under the
// given policy and returns the decoded snapshot.
func decodeMapArg(t *testing.T, probe string, goVersion string, event func(testing.TB, *ir.Program) []byte, red *redaction.Config) eventCaptures {
	irProg := generateIrForProbes(t, "simple", goVersion, probe)
	irProg.Redaction = red
	item := event(t, irProg)
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)
	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	return e
}

// TestDecoderRedactionMessage checks that the log-message render path scrubs
// the same values as the JSON snapshot path.
func TestDecoderRedactionMessage(t *testing.T) {
	t.Run("map value by key", func(t *testing.T) {
		e := decodeMapArg(t, "mapArg", goVersionSwissMap, simpleSwissMapArgEvent,
			redaction.NewConfig([]string{"a"}, nil, nil))
		require.Equal(t, "m: map[a: {redacted}]", e.Message)
	})

	t.Run("map value by type", func(t *testing.T) {
		e := decodeMapArg(t, "mapArg", goVersionSwissMap, simpleSwissMapArgEvent,
			redaction.NewConfig(nil, []string{"int"}, nil))
		require.Equal(t, "m: map[a: {redacted}]", e.Message)
	})

	t.Run("struct field", func(t *testing.T) {
		e := decodeMapArg(t, "bigMapArg", goVersionHmap, simpleBigMapArgEvent,
			redaction.NewConfig([]string{"Field1"}, nil, nil))
		require.Contains(t, e.Message, "Field1: {redacted}")
		require.Contains(t, e.Message, "Field2: 0")
	})
}

// TestDecoderRedactionExpressionMark checks that a root expression marked
// redacted by irgen is scrubbed in both the JSON snapshot and the log message.
// (The decision of which expressions to redact is made and tested in irgen,
// from the parsed AST; here we verify the decoder honors the mark.)
func TestDecoderRedactionExpressionMark(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", goVersionSwissMap, "mapArg")
	for _, probe := range irProg.Probes {
		for i := range probe.Instances {
			for _, event := range probe.Instances[i].Events {
				for _, expr := range event.Type.Expressions {
					if expr.Name == "m" {
						expr.Redacted = true
					}
				}
			}
		}
	}
	item := simpleSwissMapArgEvent(t, irProg)
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Equal(t, map[string]any{
		"m": map[string]any{"type": "map[string]int", "notCapturedReason": "redactedIdent"},
	}, e.Debugger.Snapshot.Captures.Entry.Arguments)
	require.Equal(t, "m: {redacted}", e.Message)
}

// TestEncodeInterfaceRedactsConcreteType checks that type redaction applies to
// the concrete type resolved behind an interface, which the static check in
// encodeValue cannot see. e.g. var x any = main.Secret{} with
// DD_DYNAMIC_INSTRUMENTATION_REDACTED_TYPES=main.Secret.
func TestEncodeInterfaceRedactsConcreteType(t *testing.T) {
	secret := &ir.BaseType{TypeCommon: ir.TypeCommon{ID: 7, Name: "main.Secret", ByteSize: 8}}
	c := &encodingContext{
		typesByID:            map[ir.TypeID]decoderType{7: (*baseType)(secret)},
		typesByGoRuntimeType: map[uint32]ir.TypeID{42: 7},
		dataItems:            map[typeAndAddr]output.DataItem{},
		redaction:            redaction.NewConfig(nil, []string{"main.Secret"}, nil),
	}
	data := make([]byte, 16)
	binary.NativeEndian.PutUint64(data[goRuntimeTypeOffset:goRuntimeTypeOffset+8], 42)
	binary.NativeEndian.PutUint64(data[goInterfaceDataOffset:goInterfaceDataOffset+8], 0xdead)

	var buf []byte
	enc := jsontext.NewEncoder(&jsonBuf{buf: &buf})
	require.NoError(t, enc.WriteToken(jsontext.BeginObject))
	require.NoError(t, encodeInterface(c, enc, data))
	require.NoError(t, enc.WriteToken(jsontext.EndObject))

	var out map[string]any
	require.NoError(t, json.Unmarshal(buf, &out))
	inner := out["fields"].(map[string]any)["data"].(map[string]any)
	require.Equal(t, "main.Secret", inner["type"])
	require.Equal(t, "redactedType", inner["notCapturedReason"])
}

// TestDecoderRedactionStructField decodes a struct argument and checks that a
// field whose name matches a redacted identifier is scrubbed while its
// siblings are left intact. This is the primary case: a sensitive struct field.
func TestDecoderRedactionStructField(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", goVersionHmap, "bigMapArg")
	irProg.Redaction = redaction.NewConfig([]string{"Field1"}, nil, nil)
	item := simpleBigMapArgEvent(t, irProg)
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	args := e.Debugger.Snapshot.Captures.Entry.Arguments.(map[string]any)
	entries := args["m"].(map[string]any)["entries"].([]any)
	fields := entries[0].([]any)[1].(map[string]any)["fields"].(map[string]any)

	require.Equal(t, map[string]any{
		"type": "int", "notCapturedReason": "redactedIdent",
	}, fields["Field1"], "Field1 must be redacted")
	require.Equal(t, map[string]any{
		"type": "int", "value": "0",
	}, fields["Field2"], "sibling field must be intact")
}
