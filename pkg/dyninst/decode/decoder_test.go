// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func FuzzDecoder(f *testing.F) {
	probeNames := make([]string, 0, len(cases))
	for _, c := range cases {
		probeNames = append(probeNames, c.probeName)
	}
	irProg := generateIrForProbes(f, "simple", probeNames...)
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(f, err)
	for _, c := range cases {
		f.Add(c.eventConstructor(f, irProg))
	}
	f.Fuzz(func(t *testing.T, item []byte) {
		_, _, _ = decoder.Decode(Event{
			Event:       output.Event(item),
			ServiceName: "foo",
		}, &noopSymbolicator{}, []byte{})
		require.Empty(t, decoder.dataItems)
		require.Empty(t, decoder.currentlyEncoding)
	})
}

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
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(t, err)
			buf, probe, err := decoder.Decode(Event{
				Event:       output.Event(item),
				ServiceName: "foo",
			}, &noopSymbolicator{}, []byte{})
			require.NoError(t, err)
			require.Equal(t, c.probeName, probe.GetID())
			var e eventCaptures
			require.NoError(t, json.Unmarshal(buf, &e))
			require.Equal(t, c.expected, e.Debugger.Snapshot.Captures.Entry.Arguments)
			require.Empty(t, decoder.dataItems)
			require.Empty(t, decoder.currentlyEncoding)
			require.Zero(t, decoder.snapshotMessage)
		})
	}
}

func BenchmarkDecoder(b *testing.B) {
	for _, c := range cases {
		b.Run(c.probeName, func(b *testing.B) {
			irProg := generateIrForProbes(b, "simple", c.probeName)
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(b, err)
			symbolicator := &noopSymbolicator{}
			event := Event{
				Event:       output.Event(c.eventConstructor(b, irProg)),
				ServiceName: "foo",
			}
			b.ResetTimer()
			for b.Loop() {
				_, _, err := decoder.Decode(event, symbolicator, []byte{})
				require.NoError(b, err)
			}
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
		probeName:        "mapArg",
		eventConstructor: simpleMapArgEvent,
		expected:         simpleMapArgExpected,
	},
	{
		probeName:        "bigMapArg",
		eventConstructor: simpleBigMapArgEvent,
		expected:         simpleBigMapArgExpected,
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
	obj, err := object.OpenElfFileWithDwarf(bin)
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

var simpleMapArgExpected = map[string]any{
	"m": map[string]any{
		"type": "map[string]int",
		"size": "1",
		"entries": []any{
			[]any{
				map[string]any{"type": "string", "value": "a"},
				map[string]any{"type": "int", "value": "1"},
			},
		},
	},
}

func simpleMapArgEvent(t testing.TB, irProg *ir.Program) []byte {
	probe := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "mapArg"
	})
	require.NotEqual(t, -1, probe)
	events := irProg.Probes[probe].Events
	require.Len(t, events, 1)
	eventType := events[0].Type

	var (
		mapParamType  *ir.GoMapType
		headerType    *ir.GoHMapHeaderType
		bucketType    *ir.GoHMapBucketType
		stringHdrType *ir.GoStringHeaderType
	)

	require.NotNil(t, eventType)
	// Expect exactly one expression for parameter 'm'
	require.NotEmpty(t, eventType.Expressions)
	paramType := eventType.Expressions[0].Expression.Type
	var ok bool
	mapParamType, ok = paramType.(*ir.GoMapType)
	require.True(t, ok, "expected map parameter type, got %T", paramType)
	// 0th test config uses hmaps
	headerType, ok = mapParamType.HeaderType.(*ir.GoHMapHeaderType)
	require.True(t, ok, "expected hmap header type")
	bucketType = headerType.BucketType
	require.NotNil(t, bucketType)

	// Key should be string
	stringHdrType, ok = bucketType.KeyType.(*ir.GoStringHeaderType)
	require.True(t, ok, "expected string key type, got %T", bucketType.KeyType)

	// Offsets in header
	countOff := fieldOffsetByName(t, headerType.RawFields, "count")
	bucketsOff := fieldOffsetByName(t, headerType.RawFields, "buckets")
	oldbucketsOff := fieldOffsetByName(t, headerType.RawFields, "oldbuckets")

	// Offsets in bucket
	topHashOff := fieldOffsetByName(t, bucketType.RawFields, "tophash")
	keysOff := fieldOffsetByName(t, bucketType.RawFields, "keys")
	valuesOff := fieldOffsetByName(t, bucketType.RawFields, "values")
	overflowOff := fieldOffsetByName(t, bucketType.RawFields, "overflow")

	// Offsets in string header
	strPtrOff := fieldOffsetByName(t, stringHdrType.RawFields, "str")
	strLenOff := fieldOffsetByName(t, stringHdrType.RawFields, "len")

	// Sizes
	rootLen := int(eventType.GetByteSize())
	headerLen := int(headerType.GetByteSize())
	bucketLen := int(bucketType.GetByteSize())
	keyElemSize := int(bucketType.KeyType.GetByteSize())
	valElemSize := int(bucketType.ValueType.GetByteSize())

	// Addresses
	const (
		headerAddr  = uint64(0x100000001)
		bucketsAddr = uint64(0x200000002)
		strAddr     = uint64(0x300000003)
	)

	// Build root data item (presence bitset + pointer to header)
	rootData := make([]byte, rootLen)
	// Set presence bit for first expression
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 1
	}
	ptrOff := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[ptrOff:ptrOff+8], headerAddr)

	// Build header bytes
	headerData := make([]byte, headerLen)
	// count = 1
	binary.NativeEndian.PutUint64(headerData[countOff:countOff+8], 1)
	// buckets pointer
	binary.NativeEndian.PutUint64(headerData[bucketsOff:bucketsOff+8], bucketsAddr)
	// oldbuckets = 0
	// Zero by default; explicitly set for clarity
	binary.NativeEndian.PutUint64(headerData[oldbucketsOff:oldbucketsOff+8], 0)

	// Build one bucket with one entry: ["a"] => 1
	bucketData := make([]byte, bucketLen)
	// tophash: mark first entry as non-empty (not 0,1,2..4)
	bucketData[topHashOff] = 7
	// key[0] string header
	key0Off := keysOff + 0*uint32(keyElemSize)
	binary.NativeEndian.PutUint64(bucketData[key0Off+strPtrOff:key0Off+strPtrOff+8], strAddr)
	binary.NativeEndian.PutUint64(bucketData[key0Off+strLenOff:key0Off+strLenOff+8], 1)
	// value[0] int = 1
	val0Off := valuesOff + 0*uint32(valElemSize)
	binary.NativeEndian.PutUint64(bucketData[val0Off:val0Off+8], 1)
	// overflow = 0 (already zero)
	_ = overflowOff

	// String data bytes for "a"
	strData := []byte("a")

	// Compute total event length with padding
	const (
		eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
		dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
	)
	nextMultipleOf8 := func(v int) int { return (v + 7) & ^7 }

	sz := eventHeaderSize // header
	// no stack data
	// root item
	sz += dataItemHeaderSize + rootLen
	sz = nextMultipleOf8(sz)
	// header item
	sz += dataItemHeaderSize + headerLen
	sz = nextMultipleOf8(sz)
	// buckets item (one bucket)
	sz += dataItemHeaderSize + bucketLen
	sz = nextMultipleOf8(sz)
	// string data item
	sz += dataItemHeaderSize + len(strData)
	sz = nextMultipleOf8(sz)

	var item []byte
	eventHeader := output.EventHeader{
		Data_byte_len:  uint32(sz),
		Prog_id:        1,
		Stack_byte_len: 0,
		Stack_hash:     1,
		Ktime_ns:       1,
	}

	// DataItem 0: root
	rootHeader := output.DataItemHeader{
		Type:    uint32(eventType.GetID()),
		Length:  uint32(rootLen),
		Address: 0,
	}
	// DataItem 1: header
	mapHeader := output.DataItemHeader{
		Type:    uint32(headerType.GetID()),
		Length:  uint32(headerLen),
		Address: headerAddr,
	}
	// DataItem 2: buckets backing array
	bucketsHeader := output.DataItemHeader{
		Type:    uint32(headerType.BucketsType.GetID()),
		Length:  uint32(bucketLen),
		Address: bucketsAddr,
	}
	// DataItem 3: string data for key
	strHeader := output.DataItemHeader{
		Type:    uint32(stringHdrType.Data.GetID()),
		Length:  uint32(len(strData)),
		Address: strAddr,
	}

	pad := func() {
		for (len(item) % 8) != 0 {
			item = append(item, 0)
		}
	}

	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&eventHeader)), unsafe.Sizeof(eventHeader))...)
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&rootHeader)), unsafe.Sizeof(rootHeader))...)
	item = append(item, rootData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&mapHeader)), unsafe.Sizeof(mapHeader))...)
	item = append(item, headerData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&bucketsHeader)), unsafe.Sizeof(bucketsHeader))...)
	item = append(item, bucketData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&strHeader)), unsafe.Sizeof(strHeader))...)
	item = append(item, strData...)
	pad()

	return item
}

var simpleBigMapArgExpected = map[string]any{
	"m": map[string]any{
		"type": "map[string]main.bigStruct",
		"size": "1",
		"entries": []any{
			[]any{
				map[string]any{"type": "string", "value": "b"},
				map[string]any{
					"type":    "*main.bigStruct", // This shouldn't be a pointer
					"address": "0x700000007",     // or carry this address.
					"fields": map[string]any{
						"Field1": map[string]any{"type": "int", "value": "1"},
						"Field2": map[string]any{"type": "int", "value": "0"},
						"Field3": map[string]any{"type": "int", "value": "0"},
						"Field4": map[string]any{"type": "int", "value": "0"},
						"Field5": map[string]any{"type": "int", "value": "0"},
						"Field6": map[string]any{"type": "int", "value": "0"},
						"Field7": map[string]any{"type": "int", "value": "0"},
						"data": map[string]any{
							"type": "[128]uint8",
							"size": "128",
							"elements": slices.Repeat([]any{
								map[string]any{"type": "uint8", "value": "0"},
							}, 128),
						},
					},
				},
			},
		},
	},
}

func simpleBigMapArgEvent(t testing.TB, irProg *ir.Program) []byte {
	probe := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "bigMapArg"
	})
	require.NotEqual(t, -1, probe)
	events := irProg.Probes[probe].Events
	require.Len(t, events, 1)
	eventType := events[0].Type

	var (
		mapParamType   *ir.GoMapType
		headerType     *ir.GoHMapHeaderType
		bucketType     *ir.GoHMapBucketType
		stringHdrType  *ir.GoStringHeaderType
		valStructType  *ir.StructureType
		valPointerType *ir.PointerType
	)
	paramType := eventType.Expressions[0].Expression.Type
	var ok bool
	mapParamType, ok = paramType.(*ir.GoMapType)
	require.True(t, ok)
	headerType, ok = mapParamType.HeaderType.(*ir.GoHMapHeaderType)
	require.True(t, ok)
	bucketType = headerType.BucketType
	require.NotNil(t, bucketType)
	stringHdrType, ok = bucketType.KeyType.(*ir.GoStringHeaderType)
	require.True(t, ok)
	valPointerType, ok = bucketType.ValueType.(*ir.PointerType)
	require.True(t, ok)
	valStructType, ok = valPointerType.Pointee.(*ir.StructureType)
	require.True(t, ok)
	require.Equal(t, "main.bigStruct", valStructType.GetName())

	countOff := fieldOffsetByName(t, headerType.RawFields, "count")
	bucketsOff := fieldOffsetByName(t, headerType.RawFields, "buckets")
	oldbucketsOff := fieldOffsetByName(t, headerType.RawFields, "oldbuckets")
	topHashOff := fieldOffsetByName(t, bucketType.RawFields, "tophash")
	keysOff := fieldOffsetByName(t, bucketType.RawFields, "keys")
	valuesOff := fieldOffsetByName(t, bucketType.RawFields, "values")
	_ = fieldOffsetByName(t, bucketType.RawFields, "overflow")
	strPtrOff := fieldOffsetByName(t, stringHdrType.RawFields, "str")
	strLenOff := fieldOffsetByName(t, stringHdrType.RawFields, "len")

	rootLen := int(eventType.GetByteSize())
	headerLen := int(headerType.GetByteSize())
	bucketLen := int(bucketType.GetByteSize())
	keyElemSize := int(bucketType.KeyType.GetByteSize())
	valElemSize := int(bucketType.ValueType.GetByteSize())

	const (
		headerAddr  = uint64(0x400000004)
		bucketsAddr = uint64(0x500000005)
		strAddr     = uint64(0x600000006)
		structAddr  = uint64(0x700000007)
	)

	rootData := make([]byte, rootLen)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 1
	}
	ptrOff := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[ptrOff:ptrOff+8], headerAddr)

	headerData := make([]byte, headerLen)
	binary.NativeEndian.PutUint64(headerData[countOff:countOff+8], 1)
	binary.NativeEndian.PutUint64(headerData[bucketsOff:bucketsOff+8], bucketsAddr)
	binary.NativeEndian.PutUint64(headerData[oldbucketsOff:oldbucketsOff+8], 0)

	bucketData := make([]byte, bucketLen)
	bucketData[topHashOff] = 7
	key0Off := keysOff + 0*uint32(keyElemSize)
	binary.NativeEndian.PutUint64(bucketData[key0Off+strPtrOff:key0Off+strPtrOff+8], strAddr)
	binary.NativeEndian.PutUint64(bucketData[key0Off+strLenOff:key0Off+strLenOff+8], 1)
	val0Off := valuesOff + 0*uint32(valElemSize)
	// Build struct backing data and set Field1=1
	field1Off := fieldOffsetByName(t, valStructType.RawFields, "Field1")
	structData := make([]byte, int(valStructType.GetByteSize()))
	binary.NativeEndian.PutUint64(structData[field1Off:field1Off+8], 1)
	binary.NativeEndian.PutUint64(bucketData[val0Off:val0Off+8], structAddr)

	strData := []byte("b")

	const (
		eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
		dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
	)
	nextMultipleOf8 := func(v int) int { return (v + 7) & ^7 }
	sz := eventHeaderSize
	sz += dataItemHeaderSize + rootLen
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + headerLen
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + bucketLen
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(strData)
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(structData)
	sz = nextMultipleOf8(sz)

	var item []byte
	eventHeader := output.EventHeader{
		Data_byte_len: uint32(sz),
		Prog_id:       1,
		Stack_hash:    1,
		Ktime_ns:      1,
	}
	rootHeader := output.DataItemHeader{
		Type:    uint32(eventType.GetID()),
		Length:  uint32(rootLen),
		Address: 0,
	}
	mapHeader := output.DataItemHeader{
		Type:    uint32(headerType.GetID()),
		Length:  uint32(headerLen),
		Address: headerAddr,
	}
	bucketsHeader := output.DataItemHeader{
		Type:    uint32(headerType.BucketsType.GetID()),
		Length:  uint32(bucketLen),
		Address: bucketsAddr,
	}
	strHeader := output.DataItemHeader{
		Type:    uint32(stringHdrType.Data.GetID()),
		Length:  uint32(len(strData)),
		Address: strAddr,
	}
	structHeader := output.DataItemHeader{
		Type:    uint32(valStructType.GetID()),
		Length:  uint32(len(structData)),
		Address: structAddr,
	}

	pad := func() {
		for (len(item) % 8) != 0 {
			item = append(item, 0)
		}
	}
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&eventHeader)), unsafe.Sizeof(eventHeader))...)
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&rootHeader)), unsafe.Sizeof(rootHeader))...)
	item = append(item, rootData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&mapHeader)), unsafe.Sizeof(mapHeader))...)
	item = append(item, headerData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&bucketsHeader)), unsafe.Sizeof(bucketsHeader))...)
	item = append(item, bucketData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&strHeader)), unsafe.Sizeof(strHeader))...)
	item = append(item, strData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&structHeader)), unsafe.Sizeof(structHeader))...)
	item = append(item, structData...)
	pad()
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

func fieldOffsetByName(t testing.TB, fields []ir.Field, name string) uint32 {
	for i := range fields {
		if fields[i].Name == name {
			return fields[i].Offset
		}
	}
	require.Failf(t, "field not found", "field %q not found", name)
	return 0
}

type noopSymbolicator struct{}

func (s *noopSymbolicator) Symbolicate(
	[]uint64, uint64,
) ([]symbol.StackFrame, error) {
	return nil, nil
}

type noopTypeNameResolver struct{}

func (r *noopTypeNameResolver) ResolveTypeName(
	typeID gotype.TypeID,
) (string, error) {
	return fmt.Sprintf("type%#x", typeID), nil
}

type panicDecoderType struct {
	decoderType
}

var _ decoderType = (*panicDecoderType)(nil)

func (t *panicDecoderType) encodeValueFields(*Decoder, *jsontext.Encoder, []byte) error {
	panic("boom")
}

func TestDecoderPanics(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
		return c.probeName == "stringArg"
	})
	testCase := &cases[caseIdx]
	input := testCase.eventConstructor(t, irProg)
	var stringType ir.Type
	for _, t := range irProg.Types {
		if t.GetName() == "string" {
			stringType = t
			break
		}
	}
	require.NotNil(t, stringType)
	stringID := stringType.GetID()
	decoder.decoderTypes[stringID] = &panicDecoderType{decoder.decoderTypes[stringID]}
	_, _, err = decoder.Decode(Event{
		Event:       output.Event(input),
		ServiceName: "foo"},
		&noopSymbolicator{},
		[]byte{},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestDecoderFailsOnEvaluationError(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
		return c.probeName == "stringArg"
	})
	testCase := &cases[caseIdx]
	input := testCase.eventConstructor(t, irProg)
	var stringType ir.Type
	for _, t := range irProg.Types {
		if t.GetName() == "string" {
			stringType = t
			break
		}
	}
	require.NotNil(t, stringType)
	stringID := stringType.GetID()
	delete(decoder.decoderTypes, stringID)
	out, _, err := decoder.Decode(Event{
		Event:       output.Event(input),
		ServiceName: "foo"},
		&noopSymbolicator{},
		[]byte{},
	)
	require.NoError(t, err)
	require.Contains(t, string(out), "no decoder type found")
}

// TestDecoderFailsOnEvaluationErrorAndRetainsPassedBuffer tests that the decoder
// fails on evaluation error while preserving the contents of the passed buffer.
// It is expected that consumers of the decoder API will call Decode with a reused
// buffer to avoid unnecessary allocations and they will expect the buffer to be
// appended to only.
func TestDecoderFailsOnEvaluationErrorAndRetainsPassedBuffer(t *testing.T) {
	irProg := generateIrForProbes(t, "simple", "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
		return c.probeName == "stringArg"
	})
	testCase := &cases[caseIdx]
	input := testCase.eventConstructor(t, irProg)
	var stringType ir.Type
	for _, t := range irProg.Types {
		if t.GetName() == "string" {
			stringType = t
			break
		}
	}

	buf := []byte{1, 2, 3, 4, 5}

	require.NotNil(t, stringType)
	stringID := stringType.GetID()
	delete(decoder.decoderTypes, stringID)
	// We loop here to test that the buffer is retained and not overwritten
	// by each iteration of the loop. It's expected/possible that consumers
	// of the decoder API will call Decode every time with the same buffer.
	out, _, err := decoder.Decode(Event{
		Event:       output.Event(input),
		ServiceName: "foo"},
		&noopSymbolicator{},
		buf,
	)
	require.NoError(t, err)
	require.Contains(t, string(out), "no decoder type found")
	require.Equal(t, buf, []byte{1, 2, 3, 4, 5})

}
