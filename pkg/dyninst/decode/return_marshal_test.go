// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// makeCaptureEvent builds a minimal captureEvent for return value tests.
// Each expression is a simple int with the given name and value.
// kind specifies the RootExpressionKind for all expressions.
func makeCaptureEvent(
	t *testing.T,
	names []string,
	values []int64,
	kind ir.RootExpressionKind,
) *captureEvent {
	t.Helper()
	require.Equal(t, len(names), len(values))

	// Create a BaseType for int (8 bytes, reflect.Int).
	intType := &ir.BaseType{
		TypeCommon:       ir.TypeCommon{ID: 1, Name: "int", ByteSize: 8},
		GoTypeAttributes: ir.GoTypeAttributes{GoKind: reflect.Int},
	}

	// Build root expressions.
	nExprs := len(names)
	presenceBitsetSize := uint32((2*nExprs + 7) / 8)
	expressions := make([]*ir.RootExpression, nExprs)
	offset := presenceBitsetSize
	for i, name := range names {
		expressions[i] = &ir.RootExpression{
			Name:   name,
			Offset: offset,
			Kind:   kind,
			Expression: ir.Expression{
				Type: intType,
			},
		}
		offset += 8
	}

	rootType := &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       100,
			Name:     "TestRoot",
			ByteSize: offset,
		},
		PresenceBitsetSize: presenceBitsetSize,
		Expressions:        expressions,
	}

	// Build root data: presence bitset (all present) + int values.
	rootData := make([]byte, offset)
	// Set all presence bits (bit 2*i for each expression).
	for i := range nExprs {
		byteIdx := (2 * i) / 8
		bitIdx := uint((2 * i) % 8)
		rootData[byteIdx] |= 1 << bitIdx
	}
	for i, val := range values {
		binary.NativeEndian.PutUint64(
			rootData[expressions[i].Offset:],
			uint64(val),
		)
	}

	ce := &captureEvent{
		rootData: rootData,
		rootType: rootType,
	}
	ce.encodingContext = encodingContext{
		typesByID:            map[ir.TypeID]decoderType{1: (*baseType)(intType)},
		typesByGoRuntimeType: map[uint32]ir.TypeID{},
		currentlyEncoding:    map[typeAndAddr]struct{}{},
		dataItems:            map[typeAndAddr]output.DataItem{},
	}
	return ce
}

// marshalCaptureEvent marshals a captureEvent to JSON and returns the raw bytes.
func marshalCaptureEvent(t *testing.T, ce *captureEvent) []byte {
	t.Helper()
	var buf []byte
	enc := jsontext.NewEncoder(&jsonBuf{buf: &buf})
	require.NoError(t, ce.MarshalJSONTo(enc))
	return buf
}

// jsonBuf adapts a *[]byte for use as an io.Writer with jsontext.NewEncoder.
type jsonBuf struct{ buf *[]byte }

func (b *jsonBuf) Write(p []byte) (int, error) {
	*b.buf = append(*b.buf, p...)
	return len(p), nil
}

func TestReturnMarshalSingleReturn(t *testing.T) {
	// IR generation renames single returns to "@return", so the expression
	// name arriving at the marshaling layer is already "@return".
	ce := makeCaptureEvent(t,
		[]string{"@return"},
		[]int64{142},
		ir.RootExpressionKindReturn,
	)
	raw := marshalCaptureEvent(t, ce)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	locals, ok := got["locals"].(map[string]any)
	require.True(t, ok, "expected 'locals' object, got: %s", raw)

	ret, ok := locals["@return"].(map[string]any)
	require.True(t, ok, "expected '@return' in locals, got: %s", raw)

	require.Equal(t, "int", ret["type"])
	require.Equal(t, "142", ret["value"])
}

func TestReturnMarshalSingleNamedReturn(t *testing.T) {
	// Even named returns get renamed to "@return" by IR generation for
	// single-return functions.
	ce := makeCaptureEvent(t,
		[]string{"@return"},
		[]int64{80},
		ir.RootExpressionKindReturn,
	)
	raw := marshalCaptureEvent(t, ce)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	locals := got["locals"].(map[string]any)
	ret, ok := locals["@return"].(map[string]any)
	require.True(t, ok, "single named return should use '@return', got: %s", raw)
	require.Equal(t, "int", ret["type"])
	require.Equal(t, "80", ret["value"])
}

func TestReturnMarshalMultipleReturns(t *testing.T) {
	ce := makeCaptureEvent(t,
		[]string{"r0", "r1", "r2"},
		[]int64{1, 2, 3},
		ir.RootExpressionKindReturn,
	)
	raw := marshalCaptureEvent(t, ce)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	locals, ok := got["locals"].(map[string]any)
	require.True(t, ok, "expected 'locals' object, got: %s", raw)

	// Multiple returns: should be wrapped in "@return" with "fields".
	ret, ok := locals["@return"].(map[string]any)
	require.True(t, ok, "expected '@return' in locals for multi-return, got: %s", raw)

	// Should NOT have a "type" field (design decision).
	_, hasType := ret["type"]
	require.False(t, hasType, "@return wrapper should not have 'type' field")

	fields, ok := ret["fields"].(map[string]any)
	require.True(t, ok, "expected 'fields' in @return, got: %v", ret)

	for _, name := range []string{"r0", "r1", "r2"} {
		field, ok := fields[name].(map[string]any)
		require.True(t, ok, "expected field %q in @return.fields", name)
		require.Equal(t, "int", field["type"])
	}
	require.Equal(t, "1", fields["r0"].(map[string]any)["value"])
	require.Equal(t, "2", fields["r1"].(map[string]any)["value"])
	require.Equal(t, "3", fields["r2"].(map[string]any)["value"])

	// Individual return names should NOT appear at the top level of locals.
	for _, name := range []string{"r0", "r1", "r2"} {
		_, hasRaw := locals[name]
		require.False(t, hasRaw, "should not have individual return %q in locals", name)
	}
}

func TestReturnMarshalMultipleNamedReturns(t *testing.T) {
	ce := makeCaptureEvent(t,
		[]string{"result", "result2"},
		[]int64{82, 123},
		ir.RootExpressionKindReturn,
	)
	raw := marshalCaptureEvent(t, ce)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	locals := got["locals"].(map[string]any)
	ret := locals["@return"].(map[string]any)
	fields := ret["fields"].(map[string]any)

	require.Equal(t, "82", fields["result"].(map[string]any)["value"])
	require.Equal(t, "123", fields["result2"].(map[string]any)["value"])
}

func TestReturnMarshalEntryEventUnchanged(t *testing.T) {
	// Local variables (KindLocal) should NOT get @return wrapping.
	ce := makeCaptureEvent(t,
		[]string{"x"},
		[]int64{42},
		ir.RootExpressionKindLocal,
	)
	raw := marshalCaptureEvent(t, ce)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	locals := got["locals"].(map[string]any)

	// Entry event locals should appear with their original names.
	val, ok := locals["x"].(map[string]any)
	require.True(t, ok, "entry event should keep original name 'x', got: %s", raw)
	require.Equal(t, "42", val["value"])

	_, hasReturn := locals["@return"]
	require.False(t, hasReturn, "entry event should NOT have @return")
}
