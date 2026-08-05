// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"encoding/json"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// TestDecoderContextTraceMap decodes a context.Context argument that carries an
// active trace span and asserts the decoder surfaces its trace/span/parent ids
// as a map of [key, value] entries.
func TestDecoderContextTraceMap(t *testing.T) {
	irProg := generateIrForProbes(t, "sample", goVersionSwissMap, "testTakeContext")
	item := simpleTraceContextEvent(t, irProg)

	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	buf, probe, err := decoder.Decode(Event{
		EntryOrLine: output.SingleEvent(item),
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)
	require.Equal(t, "testTakeContext", probe.GetID())

	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))

	expected := map[string]any{
		"ctx": map[string]any{
			"type": "context.Context",
			"entries": []any{
				[]any{
					map[string]any{"type": "string", "value": "trace_id"},
					map[string]any{"type": "big.Int", "value": "204258382862869200743700091125539174280"},
				},
				[]any{
					map[string]any{"type": "string", "value": "span_id"},
					map[string]any{"type": "uint64", "value": "123"},
				},
				[]any{
					map[string]any{"type": "string", "value": "parent_id"},
					map[string]any{"type": "uint64", "value": "456"},
				},
			},
		},
	}
	require.Equal(t, expected, e.Debugger.Snapshot.Captures.Entry.Arguments)
}

// simpleTraceContextEvent builds a synthetic event for the testTakeContext
// probe: the captured ctx points at an address for which the BPF chain walk
// published a synthetic trace-context data item carrying a valid span.
func simpleTraceContextEvent(t testing.TB, irProg *ir.Program) []byte {
	probeIdx := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "testTakeContext"
	})
	require.NotEqual(t, -1, probeIdx)
	eventType := irProg.Probes[probeIdx].Instances[0].Events[0].Type

	ctxIdx := slices.IndexFunc(eventType.Expressions, func(e *ir.RootExpression) bool {
		return e.Name == "ctx"
	})
	require.NotEqual(t, -1, ctxIdx)
	ctxOff := int(eventType.Expressions[ctxIdx].Offset)

	var traceCtxTypeID ir.TypeID
	for _, typ := range irProg.Types {
		if _, ok := typ.(*ir.TraceContextType); ok {
			traceCtxTypeID = typ.GetID()
		}
	}
	require.NotZero(t, traceCtxTypeID, "no TraceContextType in IR")

	const ctxAddr = uint64(0xc000010000)

	rootLen := int(eventType.GetByteSize())
	rootData := make([]byte, rootLen)
	copy(rootData, packExprStatuses(ir.ExprStatusPresent))
	// ctx interface header: nonzero runtime type word + concrete context pointer.
	binary.NativeEndian.PutUint64(rootData[ctxOff:ctxOff+8], 0x1234)
	binary.NativeEndian.PutUint64(rootData[ctxOff+8:ctxOff+16], ctxAddr)

	// Synthetic trace-context payload (trace_context_t): little-endian
	// trace_id lower/upper, span id, parent id, then a non-zero valid byte.
	tcData := make([]byte, ir.TraceContextByteSize)
	binary.LittleEndian.PutUint64(tcData[0:8], 0x1122334455667788)
	binary.LittleEndian.PutUint64(tcData[8:16], 0x99aabbccddeeff00)
	binary.LittleEndian.PutUint64(tcData[16:24], 123)
	binary.LittleEndian.PutUint64(tcData[24:32], 456)
	tcData[32] = 1

	dataItems := []struct {
		typeID  ir.TypeID
		address uint64
		data    []byte
	}{
		{eventType.GetID(), 0, rootData},
		{traceCtxTypeID, ctxAddr, tcData},
	}

	const (
		eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
		dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
	)
	nextMultipleOf8 := func(v int) int { return (v + 7) & ^7 }
	sz := eventHeaderSize
	for _, di := range dataItems {
		sz += dataItemHeaderSize + len(di.data)
		sz = nextMultipleOf8(sz)
	}

	eventHeader := output.EventHeader{
		Data_byte_len:  uint32(sz),
		Prog_id:        1,
		Stack_byte_len: 0,
		Stack_hash:     1,
		Ktime_ns:       1,
	}
	var item []byte
	item = append(item, unsafe.Slice(
		(*byte)(unsafe.Pointer(&eventHeader)), unsafe.Sizeof(eventHeader))...,
	)
	for _, di := range dataItems {
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
	return item
}
