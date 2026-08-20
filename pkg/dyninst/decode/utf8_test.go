// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package decode

import (
	"encoding/binary"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

// TestDecoderInvalidUTF8String decodes a captured Go string whose bytes are not
// valid UTF-8. Go strings are arbitrary byte sequences, so the value must still
// be reported with the invalid bytes replaced instead of silently disappearing.
func TestDecoderInvalidUTF8String(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		valid string
	}{
		{name: "all_invalid", value: "\xff\xfe", valid: ""},
		{name: "mixed", value: "ok\xffbad", valid: "okbad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			irProg := generateIrForProbes(t, "simple", goVersionHmap, "stringArg")
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(t, err)
			item := stringArgEvent(
				t, irProg, uint64(len(tc.value)), []byte(tc.value),
			)
			buf, _, err := decoder.Decode(Event{
				EntryOrLine: output.SingleEvent(item),
				ServiceName: "foo",
			}, &noopSymbolicator{}, nil, []byte{})
			require.NoError(t, err)
			requireValidUTF8JSON(t, buf)

			var e eventCaptures
			require.NoError(t, json.Unmarshal(buf, &e))
			require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)

			value := argumentValue(t, e, "s")
			require.Equal(t, tc.valid, strings.ReplaceAll(value, "�", ""))
			require.Equal(
				t, "s: "+tc.valid,
				strings.ReplaceAll(e.Message, "�", ""),
			)
		})
	}
}

// TestDecoderStringTruncatedMidRune decodes a multi-byte string whose capture
// was cut short by a length limit in the middle of a rune. The emitted value
// must not contain the split rune's leading bytes.
func TestDecoderStringTruncatedMidRune(t *testing.T) {
	const full = "世界世界"
	// Five bytes covers the first rune and two thirds of the second one.
	const capturedLen = 5

	irProg := generateIrForProbes(t, "simple", goVersionHmap, "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	item := stringArgEvent(t, irProg, uint64(len(full)), []byte(full)[:capturedLen])
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)
	requireValidUTF8JSON(t, buf)

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)

	arg := argument(t, e, "s")
	require.Equal(t, "12", arg["size"])
	require.Equal(t, true, arg["truncated"])
	value, ok := arg["value"].(string)
	require.True(t, ok, "no value in %+v", arg)
	require.True(t, utf8.ValidString(value), "invalid UTF-8 value %q", value)
	require.Equal(t, "世", strings.TrimRight(value, "�"))
	require.True(
		t, utf8.ValidString(e.Message), "invalid UTF-8 message %q", e.Message,
	)
	require.Equal(t, "s: 世...", strings.ReplaceAll(e.Message, "�", ""))
}

// TestDecoderStringTruncatedInvalidByte decodes a string whose capture was cut
// short right after a byte that was never valid UTF-8. Trimming back to a rune
// boundary must not drop bytes that were really captured.
func TestDecoderStringTruncatedInvalidByte(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first string
	}{
		// 0xff can never appear in UTF-8 at all; 0xc0 has the shape of a
		// two-byte lead byte but only ever encodes an overlong rune. Neither
		// may be mistaken for a rune that the capture limit split.
		{name: "ff", first: "\xff"},
		{name: "c0", first: "\xc0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := tc.first + "abc"

			irProg := generateIrForProbes(t, "simple", goVersionHmap, "stringArg")
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(t, err)
			item := stringArgEvent(t, irProg, uint64(len(full)), []byte(full)[:1])
			buf, _, err := decoder.Decode(Event{
				EntryOrLine: output.SingleEvent(item),
				ServiceName: "foo",
			}, &noopSymbolicator{}, nil, []byte{})
			require.NoError(t, err)
			requireValidUTF8JSON(t, buf)

			var e eventCaptures
			require.NoError(t, json.Unmarshal(buf, &e))
			require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)

			arg := argument(t, e, "s")
			require.Equal(t, "4", arg["size"])
			require.Equal(t, true, arg["truncated"])
			require.Equal(t, "�", arg["value"])
		})
	}
}

// TestDecoderLogLineLimitAfterSanitizing decodes a string holding enough invalid
// bytes that replacing each of them widens the log message past its byte limit.
// The limit has to hold for what is emitted, not just for what was read.
func TestDecoderLogLineLimitAfterSanitizing(t *testing.T) {
	value := strings.Repeat("\xffa", 3000)

	irProg := generateIrForProbes(t, "simple", goVersionHmap, "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	item := stringArgEvent(t, irProg, uint64(len(value)), []byte(value))
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)
	requireValidUTF8JSON(t, buf)

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)
	require.LessOrEqual(t, len(e.Message), maxLogLineBytes)
	require.Contains(t, e.Message, "�")
}

// invalidUTF8Symbolicator resolves every stack to a single frame whose function
// and file names are not valid UTF-8.
type invalidUTF8Symbolicator struct{}

func (s *invalidUTF8Symbolicator) Symbolicate(
	[]uint64, uint64,
) ([]symbol.StackFrame, error) {
	return []symbol.StackFrame{{Lines: []symbol.StackLine{{
		Function: "main.ba\xffd",
		File:     "ma\xffin.go",
		Line:     1,
	}}}}, nil
}

// TestDecoderInvalidUTF8StackFrame decodes an event whose symbolicated stack
// frame names are not valid UTF-8. The event must still be emitted.
func TestDecoderInvalidUTF8StackFrame(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", goVersionHmap, "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	item := stringArgEvent(t, irProg, 3, []byte("abc"))
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &invalidUTF8Symbolicator{}, nil, []byte{})
	require.NoError(t, err)
	requireValidUTF8JSON(t, buf)

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)
	require.Equal(t, "abc", argumentValue(t, e, "s"))
}

// invalidUTF8TypeNameResolver resolves every runtime type to a fixed name that
// is not valid UTF-8, mimicking a bogus runtime type word pointing at arbitrary
// bytes in the binary's types blob.
type invalidUTF8TypeNameResolver struct{}

const invalidUTF8TypeName = "pkg.Ty\xffpe"

func (r *invalidUTF8TypeNameResolver) ResolveTypeName(
	gotype.TypeID,
) (string, error) {
	return invalidUTF8TypeName, nil
}

type recordingMissingTypeCollector struct {
	names []string
}

func (c *recordingMissingTypeCollector) RecordMissingType(name string) {
	c.names = append(c.names, name)
}

// TestDecoderInvalidUTF8TypeName decodes an interface holding a runtime type
// that is not in the IR and whose runtime-resolved name is not valid UTF-8.
// The name must be treated as unresolved rather than emitted mangled.
func TestDecoderInvalidUTF8TypeName(t *testing.T) {
	const runtimeType = 0x1234

	irProg := generateIrForProbes(t, "sample", goVersionSwissMap, "testInterface")
	decoder, err := NewDecoder(irProg, &invalidUTF8TypeNameResolver{}, time.Now())
	require.NoError(t, err)
	var missingTypes recordingMissingTypeCollector
	buf, probe, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(interfaceArgEvent(t, irProg, runtimeType)),
		ServiceName: "foo",
	}, &noopSymbolicator{}, &missingTypes, []byte{})
	require.NoError(t, err)
	require.Equal(t, "testInterface", probe.GetID())
	requireValidUTF8JSON(t, buf)
	require.NotContains(t, string(buf), "pkg.Ty")

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)

	arg := argument(t, e, "b")
	require.Equal(t, "main.behavior", arg["type"])
	fields, ok := arg["fields"].(map[string]any)
	require.True(t, ok, "no fields in %+v", arg)
	data, ok := fields["data"].(map[string]any)
	require.True(t, ok, "no data field in %+v", fields)
	name, ok := data["type"].(string)
	require.True(t, ok, "no type in %+v", data)
	require.True(
		t, strings.HasPrefix(name, "UnknownType(0x1234)"),
		"expected unknown type fallback, got %q", name,
	)
	require.Equal(t, "missing type information", data["notCapturedReason"])
	require.Contains(t, e.Message, "UnknownType(0x1234)")
	for _, recorded := range missingTypes.names {
		require.True(
			t, utf8.ValidString(recorded),
			"recorded invalid UTF-8 type name %q", recorded,
		)
	}
}

// TestDecoderInvalidUTF8DebugInfoNames decodes a struct-valued map argument
// whose type, field and variable names are not valid UTF-8. Names are read out
// of the target's debug info with no more validation than captured values get,
// so a bad name must be mangled rather than take the whole event down.
func TestDecoderInvalidUTF8DebugInfoNames(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", goVersionHmap, "bigMapArg")
	// Build the event before corrupting the names; the helper checks them.
	item := addStackToEvent(simpleBigMapArgEvent(t, irProg), syntheticStackPCs)

	mapType := irType[*ir.GoMapType](t, irProg, "map[string]main.bigStruct")
	ptrType := irType[*ir.PointerType](t, irProg, "*main.bigStruct")
	structType := irType[*ir.StructureType](t, irProg, "main.bigStruct")
	mapType.Name = "map[string]main.big\xffStruct"
	ptrType.Name = "*main.big\xffStruct"
	structType.RawFields[0].Name = "Fie\xffld1"
	for _, expr := range probeEventType(t, irProg, "bigMapArg").Expressions {
		if expr.Kind == ir.RootExpressionKindArgument {
			expr.Name = "m\xff"
		}
	}

	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	buf, _, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)
	requireValidUTF8JSON(t, buf)

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Empty(t, e.Debugger.Snapshot.EvaluationErrors)

	arg := argument(t, e, "m�")
	require.Equal(t, "map[string]main.big�Struct", arg["type"])
	entries, ok := arg["entries"].([]any)
	require.True(t, ok, "no entries in %+v", arg)
	require.Len(t, entries, 1)
	value, ok := entries[0].([]any)[1].(map[string]any)
	require.True(t, ok, "no entry value in %+v", entries[0])
	require.Equal(t, "*main.big�Struct", value["type"])
	fields, ok := value["fields"].(map[string]any)
	require.True(t, ok, "no fields in %+v", value)
	require.Equal(
		t, map[string]any{"type": "int", "value": "1"}, fields["Fie�ld1"],
	)
	require.Contains(t, e.Message, "Fie�ld1: 1")
}

// irType returns the IR type named name, which must be a T.
func irType[T ir.Type](t testing.TB, irProg *ir.Program, name string) T {
	for _, typ := range irProg.Types {
		if typ.GetName() != name {
			continue
		}
		typed, ok := typ.(T)
		require.True(t, ok, "type %q is a %T", name, typ)
		return typed
	}
	require.FailNowf(t, "type not found", "no IR type named %q", name)
	var zero T
	return zero
}

func requireValidUTF8JSON(t testing.TB, buf []byte) {
	t.Helper()
	require.True(t, utf8.Valid(buf), "invalid UTF-8 output: %q", buf)
	require.True(t, json.Valid(buf), "invalid JSON output: %q", buf)
}

func argument(t testing.TB, e eventCaptures, name string) map[string]any {
	t.Helper()
	args, ok := e.Debugger.Snapshot.Captures.Entry.Arguments.(map[string]any)
	require.True(t, ok, "no arguments captured")
	arg, ok := args[name].(map[string]any)
	require.True(t, ok, "argument %q missing from %+v", name, args)
	return arg
}

func argumentValue(t testing.TB, e eventCaptures, name string) string {
	t.Helper()
	arg := argument(t, e, name)
	value, ok := arg["value"].(string)
	require.True(t, ok, "no value in %+v", arg)
	return value
}

// stringArgEvent builds an event for the simple program's stringArg probe whose
// string header reports strLen bytes and whose captured backing data is data.
// Passing fewer bytes than strLen models a capture cut short by a length limit.
func stringArgEvent(
	t testing.TB, irProg *ir.Program, strLen uint64, data []byte,
) []byte {
	eventType := probeEventType(t, irProg, "stringArg")
	var stringType *ir.GoStringHeaderType
	for _, typ := range irProg.Types {
		if typ.GetName() == "string" {
			stringType = typ.(*ir.GoStringHeaderType)
		}
	}
	require.NotNil(t, stringType)

	const strAddr = uint64(0xdeadbeef)
	rootData := rootDataWithAllExprsPresent(eventType)
	// Both expressions (the template segment and the argument) refer to the
	// same string header.
	for _, expr := range eventType.Expressions {
		off := expr.Offset
		binary.NativeEndian.PutUint64(rootData[off:off+8], strAddr)
		binary.NativeEndian.PutUint64(rootData[off+8:off+16], strLen)
	}
	return buildEvent([]eventDataItem{
		{typeID: eventType.GetID(), data: rootData},
		{typeID: stringType.Data.GetID(), address: strAddr, data: data},
	})
}

// interfaceArgEvent builds an event for the sample program's testInterface
// probe holding an interface value with the given runtime type and a nil data
// pointer.
func interfaceArgEvent(
	t testing.TB, irProg *ir.Program, runtimeType uint64,
) []byte {
	eventType := probeEventType(t, irProg, "testInterface")
	rootData := rootDataWithAllExprsPresent(eventType)
	for _, expr := range eventType.Expressions {
		off := expr.Offset
		binary.NativeEndian.PutUint64(rootData[off:off+8], runtimeType)
		binary.NativeEndian.PutUint64(rootData[off+8:off+16], 0)
	}
	return buildEvent([]eventDataItem{
		{typeID: eventType.GetID(), data: rootData},
	})
}

func probeEventType(
	t testing.TB, irProg *ir.Program, probeID string,
) *ir.EventRootType {
	t.Helper()
	probeIdx := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == probeID
	})
	require.NotEqual(t, -1, probeIdx, "probe %q not found", probeID)
	events := irProg.Probes[probeIdx].Instances[0].Events
	require.GreaterOrEqual(t, len(events), 1)
	return events[0].Type
}

func rootDataWithAllExprsPresent(eventType *ir.EventRootType) []byte {
	rootData := make([]byte, eventType.GetByteSize())
	copy(rootData, packExprStatuses(slices.Repeat(
		[]ir.ExprStatus{ir.ExprStatusPresent}, len(eventType.Expressions),
	)...))
	return rootData
}

type eventDataItem struct {
	typeID  ir.TypeID
	address uint64
	data    []byte
}

// syntheticStackPCs give the decoder a stack to report so that events do not
// carry an unrelated missing-stack evaluation error.
var syntheticStackPCs = []uint64{0x1000, 0x2000, 0x3000}

func buildEvent(items []eventDataItem) []byte {
	const (
		eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
		dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
	)
	nextMultipleOf8 := func(v int) int { return (v + 7) & ^7 }
	sz := eventHeaderSize
	for _, di := range items {
		sz = nextMultipleOf8(sz + dataItemHeaderSize + len(di.data))
	}
	eventHeader := output.EventHeader{
		Data_byte_len: uint32(sz),
		Prog_id:       1,
		Stack_hash:    1,
		Ktime_ns:      1,
	}
	item := append([]byte(nil), unsafe.Slice(
		(*byte)(unsafe.Pointer(&eventHeader)), unsafe.Sizeof(eventHeader))...,
	)
	for _, di := range items {
		header := output.DataItemHeader{
			Type:    uint32(di.typeID),
			Length:  uint32(len(di.data)),
			Address: di.address,
		}
		item = append(item, unsafe.Slice(
			(*byte)(unsafe.Pointer(&header)), unsafe.Sizeof(header))...,
		)
		item = append(item, di.data...)
		for len(item)%8 != 0 {
			item = append(item, 0)
		}
	}
	return addStackToEvent(item, syntheticStackPCs)
}
