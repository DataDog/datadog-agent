// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"slices"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// TestDecoderManually is a test that manually constructs an event and decodes
// it.
//
// This makes it easy to assert properties of the decoder's internal state.
func TestDecoderManually(t *testing.T) {
	type captures struct{ Entry struct{ Arguments any } }
	type debugger struct{ Snapshot struct{ Captures captures } }
	type eventCaptures struct{ Debugger debugger }
	for _, c := range cases {
		t.Run(c.probeName, func(t *testing.T) {
			irProg := generateIrForProbes(t, "simple", c.probeName)
			item := c.eventConstructor(t, irProg)
			decoder, err := NewDecoder(irProg)
			require.NoError(t, err)
			buf := bytes.NewBuffer(nil)
			probe, err := decoder.Decode(Event{
				Event:       output.Event(item),
				ServiceName: "foo",
			}, &noopSymbolicator{}, buf)
			require.NoError(t, err)
			require.Equal(t, c.probeName, probe.GetID())
			var e eventCaptures
			require.NoError(t, json.Unmarshal(buf.Bytes(), &e))
			require.Equal(t, c.expected, e.Debugger.Snapshot.Captures.Entry.Arguments)
			require.Empty(t, decoder.dataItems)
			require.Empty(t, decoder.currentlyEncoding)
			require.Zero(t, decoder.snapshotMessage)
		})
	}
}

type testCase struct {
	probeName        string
	eventConstructor func(testing.TB, *ir.Program) []byte
	expected         any
}

var cases = []testCase{
	{
		probeName:        "stringArg",
		eventConstructor: simpleStringArgEvent,
		expected:         simpleStringArgExpected,
	},
	{
		probeName:        "PointerChainArg",
		eventConstructor: simplePointerChainArgEvent,
		expected:         simplePointerChainArgExpected,
	},
}

func generateIrForProbes(
	t testing.TB, progName string, probeNames ...string,
) *ir.Program {
	cfgs := testprogs.MustGetCommonConfigs(t)
	bin := testprogs.MustGetBinary(t, progName, cfgs[0])
	probes := testprogs.MustGetProbeDefinitions(t, progName)
	probes = slices.DeleteFunc(probes, func(p ir.ProbeDefinition) bool {
		return !slices.Contains(probeNames, p.GetID())
	})
	require.Len(t, probes, len(probeNames))
	obj, err := object.OpenElfFile(bin)
	require.NoError(t, err)
	defer obj.Close()
	irProg, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	return irProg
}

var simpleStringArgExpected = map[string]any{
	"s": map[string]any{
		"type":  "string",
		"value": "abcdefghijklmnop",
	},
}

func simpleStringArgEvent(t testing.TB, irProg *ir.Program) []byte {
	probe := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "stringArg"
	})
	require.NotEqual(t, -1, probe)
	events := irProg.Probes[probe].Events
	require.Len(t, events, 1)
	eventType := events[0].Type
	var stringType *ir.GoStringHeaderType
	for _, t := range irProg.Types {
		if t.GetName() == "string" {
			stringType = t.(*ir.GoStringHeaderType)
		}
	}
	require.NotNil(t, stringType)
	require.NotNil(t, eventType)
	require.Equal(t, uint32(17), eventType.GetByteSize())

	var item []byte
	eventHeader := output.EventHeader{
		Data_byte_len: uint32(
			unsafe.Sizeof(output.EventHeader{}) +
				1 /* bitset */ + unsafe.Sizeof(output.DataItemHeader{}) +
				16 + 7 /* padding */ +
				unsafe.Sizeof(output.DataItemHeader{}) +
				16,
		),
		Prog_id:        1,
		Stack_byte_len: 0,
		Stack_hash:     1,
		Ktime_ns:       1,
	}
	dataItem0 := output.DataItemHeader{
		Type:    uint32(eventType.GetID()),
		Length:  17,
		Address: 0,
	}
	dataItem1 := output.DataItemHeader{
		Type:    uint32(stringType.Data.ID),
		Length:  16,
		Address: 0xdeadbeef,
	}
	item = append(item, unsafe.Slice(
		(*byte)(unsafe.Pointer(&eventHeader)), unsafe.Sizeof(eventHeader))...,
	)
	item = append(item, unsafe.Slice(
		(*byte)(unsafe.Pointer(&dataItem0)), unsafe.Sizeof(dataItem0))...,
	)
	item = append(item, 1) // bitset
	item = binary.NativeEndian.AppendUint64(item, 0xdeadbeef)
	item = binary.NativeEndian.AppendUint64(item, 16)
	item = append(item, 0, 0, 0, 0, 0, 0, 0) // padding
	item = append(item, unsafe.Slice(
		(*byte)(unsafe.Pointer(&dataItem1)), unsafe.Sizeof(dataItem1))...,
	)
	item = append(item, "abcdefghijklmnop"...)
	return item
}

var simplePointerChainArgExpected = map[string]any{
	"ptr": map[string]any{
		"type":    "*****int",
		"address": "0xa0000005",
		"value":   "17",
	},
}

func simplePointerChainArgEvent(t testing.TB, irProg *ir.Program) []byte {
	probe := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "PointerChainArg"
	})
	require.NotEqual(t, -1, probe)
	events := irProg.Probes[probe].Events
	require.Len(t, events, 1)
	eventType := events[0].Type
	rootLen := int(eventType.GetByteSize())
	rootData := make([]byte, rootLen)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 1
	}
	// Build a fully captured pointer chain *****int → int(17)
	argType := eventType.Expressions[0].Expression.Type
	ptr1, ok := argType.(*ir.PointerType)
	require.True(t, ok)
	ptr2, ok := ptr1.Pointee.(*ir.PointerType)
	require.True(t, ok)
	ptr3, ok := ptr2.Pointee.(*ir.PointerType)
	require.True(t, ok)
	ptr4, ok := ptr3.Pointee.(*ir.PointerType)
	require.True(t, ok)
	ptr5, ok := ptr4.Pointee.(*ir.PointerType)
	require.True(t, ok)
	intType, ok := ptr5.Pointee.(*ir.BaseType)
	require.True(t, ok)

	const (
		addr1 = uint64(0xa0000001)
		addr2 = uint64(0xa0000002)
		addr3 = uint64(0xa0000003)
		addr4 = uint64(0xa0000004)
		addr5 = uint64(0xa0000005)
	)
	// Root data contains address of first pointer
	off := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[off:off+8], addr1)

	// Helper to build a pointer data item (8-byte address payload)
	makePtrItem := func(tid ir.TypeID, addr uint64, pointsTo uint64) (hdr output.DataItemHeader, data []byte) {
		data = make([]byte, 8)
		binary.NativeEndian.PutUint64(data, pointsTo)
		hdr = output.DataItemHeader{Type: uint32(tid), Length: uint32(len(data)), Address: addr}
		return
	}
	// Helper to build an int data item (8-byte int payload)
	makeIntItem := func(tid ir.TypeID, addr uint64, value uint64) (hdr output.DataItemHeader, data []byte) {
		data = make([]byte, 8)
		binary.NativeEndian.PutUint64(data, value)
		hdr = output.DataItemHeader{Type: uint32(tid), Length: uint32(len(data)), Address: addr}
		return
	}

	// Data items for each pointer level and the final int value
	ptr2Hdr, ptr2Data := makePtrItem(ptr2.GetID(), addr1, addr2)
	ptr3Hdr, ptr3Data := makePtrItem(ptr3.GetID(), addr2, addr3)
	ptr4Hdr, ptr4Data := makePtrItem(ptr4.GetID(), addr3, addr4)
	ptr5Hdr, ptr5Data := makePtrItem(ptr5.GetID(), addr4, addr5)
	intHdr, intData := makeIntItem(intType.GetID(), addr5, 17)

	// Compute total size
	const (
		eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
		dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
	)
	nextMultipleOf8 := func(v int) int { return (v + 7) & ^7 }
	sz := eventHeaderSize
	sz += dataItemHeaderSize + rootLen
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(ptr2Data)
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(ptr3Data)
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(ptr4Data)
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(ptr5Data)
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(intData)
	sz = nextMultipleOf8(sz)

	var item []byte
	eh := output.EventHeader{Data_byte_len: uint32(sz), Prog_id: 1, Stack_hash: 1, Ktime_ns: 1}
	dihRoot := output.DataItemHeader{Type: uint32(eventType.GetID()), Length: uint32(rootLen), Address: 0}
	pad := func() {
		for (len(item) % 8) != 0 {
			item = append(item, 0)
		}
	}
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&eh)), unsafe.Sizeof(eh))...)
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&dihRoot)), unsafe.Sizeof(dihRoot))...)
	item = append(item, rootData...)
	pad()
	// Append pointer chain items and final int item
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&ptr2Hdr)), unsafe.Sizeof(ptr2Hdr))...)
	item = append(item, ptr2Data...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&ptr3Hdr)), unsafe.Sizeof(ptr3Hdr))...)
	item = append(item, ptr3Data...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&ptr4Hdr)), unsafe.Sizeof(ptr4Hdr))...)
	item = append(item, ptr4Data...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&ptr5Hdr)), unsafe.Sizeof(ptr5Hdr))...)
	item = append(item, ptr5Data...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&intHdr)), unsafe.Sizeof(intHdr))...)
	item = append(item, intData...)
	pad()
	return item
}

type noopSymbolicator struct{}

func (s *noopSymbolicator) Symbolicate(
	[]uint64, uint64,
) ([]symbol.StackFrame, error) {
	return nil, nil
}
