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
	"strings"
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

const (
	// goVersionHmap is the Go major version that uses hmap implementation.
	goVersionHmap = "go1.23"
	// goVersionSwissMap is the Go major version that uses swiss map implementation.
	goVersionSwissMap = "go1.24"
)

func FuzzDecoder(f *testing.F) {
	probeNameSet := make(map[string]bool)
	goVersions := make(map[string]bool)
	for _, c := range cases {
		probeNameSet[c.probeName] = true
		goVersions[c.goVersion] = true
	}
	probeNames := make([]string, 0, len(probeNameSet))
	for name := range probeNameSet {
		probeNames = append(probeNames, name)
	}
	for goVersion := range goVersions {
		irProg := generateIrForProbes(f, "simple", goVersion, probeNames...)
		for _, c := range cases {
			if c.goVersion == goVersion {
				f.Add(c.eventConstructor(f, irProg))
			}
		}
	}
	f.Fuzz(func(t *testing.T, item []byte) {
		// Use first go version for fuzzing
		irProg := generateIrForProbes(t, "simple", cases[0].goVersion, probeNames...)
		decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
		require.NoError(t, err)
		_, _, _ = decoder.Decode(Event{
			EntryOrLine: output.Event(item),
			ServiceName: "foo",
		}, &noopSymbolicator{}, nil, []byte{})
		require.Empty(t, decoder.entryOrLine.dataItems)
		require.Empty(t, decoder.entryOrLine.currentlyEncoding)
		require.Empty(t, decoder._return.dataItems)
	})
}

type (
	captures struct{ Entry struct{ Arguments any } }
	debugger struct {
		Snapshot         struct{ Captures captures }
		EvaluationErrors []struct {
			Expression string `json:"expr"`
			Message    string `json:"message"`
		} `json:"evaluationErrors,omitempty"`
	}
	eventCaptures struct {
		Debugger debugger
		Message  string `json:"message,omitempty"`
	}
)

// TestDecoderManually is a test that manually constructs an event and decodes
// it.
//
// This makes it easy to assert properties of the decoder's internal state.
func TestDecoderManually(t *testing.T) {
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s-%s", c.probeName, c.goVersion), func(t *testing.T) {
			irProg := generateIrForProbes(t, "simple", c.goVersion, c.probeName)
			item := c.eventConstructor(t, irProg)
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(t, err)
			buf, probe, err := decoder.Decode(Event{
				EntryOrLine: output.Event(item),
				ServiceName: "foo",
			}, &noopSymbolicator{}, nil, []byte{})
			require.NoError(t, err)
			require.Equal(t, c.probeName, probe.GetID())
			var e eventCaptures
			require.NoError(t, json.Unmarshal(buf, &e))
			require.Equal(t, c.expected, e.Debugger.Snapshot.Captures.Entry.Arguments)
			if c.expectedMessage != "" {
				require.Equal(t, c.expectedMessage, e.Message)
			}
			require.Empty(t, decoder.entryOrLine.dataItems)
			require.Empty(t, decoder.entryOrLine.currentlyEncoding)
			require.Nil(t, decoder.entryOrLine.rootType)
			require.Nil(t, decoder.entryOrLine.rootData)
			require.Zero(t, decoder.entryOrLine.evaluationErrors)
			require.Zero(t, decoder.message)
		})
	}
}

func BenchmarkDecoder(b *testing.B) {
	for _, c := range cases {
		b.Run(fmt.Sprintf("%s-%s", c.probeName, c.goVersion), func(b *testing.B) {
			irProg := generateIrForProbes(b, "simple", c.goVersion, c.probeName)
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(b, err)
			symbolicator := &noopSymbolicator{}
			event := Event{
				EntryOrLine: output.Event(c.eventConstructor(b, irProg)),
				ServiceName: "foo",
			}
			b.ResetTimer()
			for b.Loop() {
				_, _, err := decoder.Decode(event, symbolicator, nil, []byte{})
				require.NoError(b, err)
			}
		})
	}
}

type testCase struct {
	probeName        string
	goVersion        string
	eventConstructor func(testing.TB, *ir.Program) []byte
	expected         any
	expectedMessage  string
}

var cases = []testCase{
	{
		probeName:        "stringArg",
		goVersion:        goVersionHmap,
		eventConstructor: simpleStringArgEvent,
		expected:         simpleStringArgExpected,
		expectedMessage:  "s: abcdefghijklmnop",
	},
	{
		probeName:        "mapArg",
		goVersion:        goVersionHmap,
		eventConstructor: simpleMapArgEvent,
		expected:         simpleMapArgExpected,
		expectedMessage:  "m: map[a: 1]",
	},
	{
		probeName:        "bigMapArg",
		goVersion:        goVersionHmap,
		eventConstructor: simpleBigMapArgEvent,
		expected:         simpleBigMapArgExpected,
		expectedMessage:  "m: map[b: {Field1: 1, Field2: 0, Field3: 0, Field4: 0, Field5: 0, ...}}]",
	},
	{
		probeName:        "PointerChainArg",
		goVersion:        goVersionHmap,
		eventConstructor: simplePointerChainArgEvent,
		expected:         simplePointerChainArgExpected,
		expectedMessage:  "ptr: 17",
	},
	{
		probeName:        "mapArg",
		goVersion:        goVersionSwissMap,
		eventConstructor: simpleSwissMapArgEvent,
		expected:         simpleMapArgExpected,
		expectedMessage:  "m: map[a: 1]",
	},
	{
		probeName:        "mapArg",
		goVersion:        goVersionSwissMap,
		eventConstructor: simpleSwissMapTablesArgEvent,
		expected:         simpleMapArgExpected,
		expectedMessage:  "m: map[a: 1]",
	},
}

func generateIrForProbes(
	t testing.TB, progName string, goVersion string, probeNames ...string,
) *ir.Program {
	cfgs := testprogs.MustGetCommonConfigs(t)
	cfgIdx := slices.IndexFunc(cfgs, func(c testprogs.Config) bool {
		return strings.HasPrefix(c.GOTOOLCHAIN, goVersion)
	})
	require.NotEqual(t, -1, cfgIdx, "no config found for go version prefix %q", goVersion)
	cfg := cfgs[cfgIdx]
	bin := testprogs.MustGetBinary(t, progName, cfg)
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
	require.GreaterOrEqual(t, len(events), 1)
	eventType := events[0].Type
	var stringType *ir.GoStringHeaderType
	for _, t := range irProg.Types {
		if t.GetName() == "string" {
			stringType = t.(*ir.GoStringHeaderType)
		}
	}
	require.NotNil(t, stringType)
	require.NotNil(t, eventType)
	require.Equal(t, uint32(33), eventType.GetByteSize())

	var item []byte
	eventHeader := output.EventHeader{
		Data_byte_len: uint32(
			unsafe.Sizeof(output.EventHeader{}) +
				1 /* bitset */ + unsafe.Sizeof(output.DataItemHeader{}) +
				32 + 7 /* padding */ +
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
		Length:  33,
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
	item = append(item, 3) // bitset: bit 0 (argument) and bit 1 (template_segment) set
	// First expression (argument) at offset 1
	item = binary.NativeEndian.AppendUint64(item, 0xdeadbeef)
	item = binary.NativeEndian.AppendUint64(item, 16)
	// Second expression (template_segment) at offset 17
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
	require.GreaterOrEqual(t, len(events), 1)
	eventType := events[0].Type

	var (
		mapParamType  *ir.GoMapType
		headerType    *ir.GoHMapHeaderType
		bucketType    *ir.GoHMapBucketType
		stringHdrType *ir.GoStringHeaderType
	)

	require.NotNil(t, eventType)
	// Expect two expressions: argument and template_segment
	require.GreaterOrEqual(t, len(eventType.Expressions), 2)
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

	// Build root data item (presence bitset + pointers to header)
	rootData := make([]byte, rootLen)
	// Set presence bits for both expressions (bit 0 for argument, bit 1 for template_segment)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 3 // bits 0 and 1 set
	}
	// First expression (argument) at offset 1
	ptrOff := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[ptrOff:ptrOff+8], headerAddr)
	// Second expression (template_segment) at offset 9
	templatePtrOff := int(eventType.Expressions[1].Offset)
	binary.NativeEndian.PutUint64(rootData[templatePtrOff:templatePtrOff+8], headerAddr)

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

func simpleSwissMapArgEvent(t testing.TB, irProg *ir.Program) []byte {
	probe := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "mapArg"
	})
	require.NotEqual(t, -1, probe)
	events := irProg.Probes[probe].Events
	require.GreaterOrEqual(t, len(events), 1)
	eventType := events[0].Type

	var (
		mapParamType    *ir.GoMapType
		headerType      *ir.GoSwissMapHeaderType
		groupType       *ir.StructureType
		stringHdrType   *ir.GoStringHeaderType
		slotsArrayType  *ir.ArrayType
		entryStructType *ir.StructureType
	)

	require.NotNil(t, eventType)
	require.GreaterOrEqual(t, len(eventType.Expressions), 2)
	paramType := eventType.Expressions[0].Expression.Type
	var ok bool
	mapParamType, ok = paramType.(*ir.GoMapType)
	require.True(t, ok, "expected map parameter type, got %T", paramType)
	headerType, ok = mapParamType.HeaderType.(*ir.GoSwissMapHeaderType)
	require.True(t, ok, "expected swiss map header type, got %T", mapParamType.HeaderType)
	groupType = headerType.GroupType
	require.NotNil(t, groupType)

	// Find slots field in group
	slotsFieldIdx := slices.IndexFunc(groupType.RawFields, func(f ir.Field) bool {
		return f.Name == "slots"
	})
	require.NotEqual(t, -1, slotsFieldIdx, "slots field not found")
	slotsField := groupType.RawFields[slotsFieldIdx]
	slotsFieldOffset := slotsField.Offset
	ctrlField := fieldOffsetByName(t, groupType.RawFields, "ctrl")
	slotsArrayType, ok = slotsField.Type.(*ir.ArrayType)
	require.True(t, ok, "expected slots to be array type, got %T", slotsField.Type)
	entryStructType, ok = slotsArrayType.Element.(*ir.StructureType)
	require.True(t, ok, "expected slots element to be struct type, got %T", slotsArrayType.Element)

	// Find key and elem fields in entry struct
	keyFieldIdx := slices.IndexFunc(entryStructType.RawFields, func(f ir.Field) bool {
		return f.Name == "key"
	})
	require.NotEqual(t, -1, keyFieldIdx, "key field not found")
	keyFieldOffset := entryStructType.RawFields[keyFieldIdx].Offset
	elemFieldOffset := fieldOffsetByName(t, entryStructType.RawFields, "elem")
	keyFieldType := entryStructType.RawFields[keyFieldIdx].Type
	stringHdrType, ok = keyFieldType.(*ir.GoStringHeaderType)
	require.True(t, ok, "expected string key type, got %T", keyFieldType)

	// Offsets in header
	dirPtrOff := fieldOffsetByName(t, headerType.RawFields, "dirPtr")
	dirLenOff := fieldOffsetByName(t, headerType.RawFields, "dirLen")
	usedOff := fieldOffsetByName(t, headerType.RawFields, "used")

	// Offsets in string header
	strPtrOff := fieldOffsetByName(t, stringHdrType.RawFields, "str")
	strLenOff := fieldOffsetByName(t, stringHdrType.RawFields, "len")

	// Sizes
	rootLen := int(eventType.GetByteSize())
	headerLen := int(headerType.GetByteSize())
	groupLen := int(groupType.GetByteSize())
	entrySize := int(entryStructType.GetByteSize())

	// Addresses
	const (
		headerAddr = uint64(0x100000001)
		groupAddr  = uint64(0x200000002)
		strAddr    = uint64(0x300000003)
	)

	// Build root data item
	rootData := make([]byte, rootLen)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 3 // bits 0 and 1 set
	}
	ptrOff := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[ptrOff:ptrOff+8], headerAddr)
	templatePtrOff := int(eventType.Expressions[1].Offset)
	binary.NativeEndian.PutUint64(rootData[templatePtrOff:templatePtrOff+8], headerAddr)

	// Build header bytes
	headerData := make([]byte, headerLen)
	// dirLen = 0 (means dirPtr points directly to group)
	binary.NativeEndian.PutUint64(headerData[dirLenOff:dirLenOff+8], 0)
	// dirPtr points to group
	binary.NativeEndian.PutUint64(headerData[dirPtrOff:dirPtrOff+8], groupAddr)
	// used = 1
	binary.NativeEndian.PutUint64(headerData[usedOff:usedOff+8], 1)

	// Build group data with one entry: ["a"] => 1
	groupData := make([]byte, groupLen)
	// ctrl: mark slot 0 as occupied (bit 7+0*8 = bit 7 should be 0)
	// All other slots empty (bits 7+1*8 through 7+7*8 should be 1)
	ctrlWord := uint64(0)
	for i := 1; i < 8; i++ {
		ctrlWord |= 1 << (7 + uint(i*8))
	}
	binary.LittleEndian.PutUint64(groupData[ctrlField:ctrlField+8], ctrlWord)
	// Entry 0: key (string header)
	entry0Off := slotsFieldOffset + 0*uint32(entrySize)
	keyOff := entry0Off + keyFieldOffset
	binary.NativeEndian.PutUint64(groupData[keyOff+strPtrOff:keyOff+strPtrOff+8], strAddr)
	binary.NativeEndian.PutUint64(groupData[keyOff+strLenOff:keyOff+strLenOff+8], 1)
	// Entry 0: value (int = 1)
	elemOff := entry0Off + elemFieldOffset
	binary.NativeEndian.PutUint64(groupData[elemOff:elemOff+8], 1)

	// String data bytes for "a"
	strData := []byte("a")

	// Compute total event length with padding
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
	sz += dataItemHeaderSize + groupLen
	sz = nextMultipleOf8(sz)
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
	groupHeader := output.DataItemHeader{
		Type:    uint32(groupType.GetID()),
		Length:  uint32(groupLen),
		Address: groupAddr,
	}
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
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&groupHeader)), unsafe.Sizeof(groupHeader))...)
	item = append(item, groupData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&strHeader)), unsafe.Sizeof(strHeader))...)
	item = append(item, strData...)
	pad()

	return item
}

func simpleSwissMapTablesArgEvent(t testing.TB, irProg *ir.Program) []byte {
	probe := slices.IndexFunc(irProg.Probes, func(p *ir.Probe) bool {
		return p.GetID() == "mapArg"
	})
	require.NotEqual(t, -1, probe)
	events := irProg.Probes[probe].Events
	require.GreaterOrEqual(t, len(events), 1)
	eventType := events[0].Type

	var (
		mapParamType      *ir.GoMapType
		headerType        *ir.GoSwissMapHeaderType
		groupType         *ir.StructureType
		stringHdrType     *ir.GoStringHeaderType
		tablePtrSliceType *ir.GoSliceDataType
		tablePtrType      *ir.PointerType
		tableType         *ir.StructureType
		groupsType        *ir.GoSwissMapGroupsType
		groupSliceType    *ir.GoSliceDataType
		slotsArrayType    *ir.ArrayType
		entryStructType   *ir.StructureType
	)

	require.NotNil(t, eventType)
	require.GreaterOrEqual(t, len(eventType.Expressions), 2)
	paramType := eventType.Expressions[0].Expression.Type
	var ok bool
	mapParamType, ok = paramType.(*ir.GoMapType)
	require.True(t, ok, "expected map parameter type, got %T", paramType)
	headerType, ok = mapParamType.HeaderType.(*ir.GoSwissMapHeaderType)
	require.True(t, ok, "expected swiss map header type, got %T", mapParamType.HeaderType)
	groupType = headerType.GroupType
	require.NotNil(t, groupType)
	tablePtrSliceType = headerType.TablePtrSliceType
	require.NotNil(t, tablePtrSliceType)
	tablePtrType, ok = tablePtrSliceType.Element.(*ir.PointerType)
	require.True(t, ok, "expected table pointer type")
	tableType, ok = tablePtrType.Pointee.(*ir.StructureType)
	require.True(t, ok, "expected table structure type")

	// Find groups field in table
	groupsFieldIdx := slices.IndexFunc(tableType.RawFields, func(f ir.Field) bool {
		return f.Name == "groups"
	})
	require.NotEqual(t, -1, groupsFieldIdx, "groups field not found")
	groupsField := tableType.RawFields[groupsFieldIdx]
	groupsType, ok = groupsField.Type.(*ir.GoSwissMapGroupsType)
	require.True(t, ok, "expected swiss map groups type, got %T", groupsField.Type)
	groupSliceType = groupsType.GroupSliceType
	require.NotNil(t, groupSliceType)

	// Find slots field in group
	slotsFieldIdx := slices.IndexFunc(groupType.RawFields, func(f ir.Field) bool {
		return f.Name == "slots"
	})
	require.NotEqual(t, -1, slotsFieldIdx, "slots field not found")
	slotsField := groupType.RawFields[slotsFieldIdx]
	slotsArrayType, ok = slotsField.Type.(*ir.ArrayType)
	require.True(t, ok, "expected slots to be array type, got %T", slotsField.Type)
	entryStructType, ok = slotsArrayType.Element.(*ir.StructureType)
	require.True(t, ok, "expected slots element to be struct type, got %T", slotsArrayType.Element)

	// Find key and elem fields in entry struct
	keyFieldIdx := slices.IndexFunc(entryStructType.RawFields, func(f ir.Field) bool {
		return f.Name == "key"
	})
	require.NotEqual(t, -1, keyFieldIdx, "key field not found")
	keyFieldOffset := entryStructType.RawFields[keyFieldIdx].Offset
	elemFieldOffset := fieldOffsetByName(t, entryStructType.RawFields, "elem")
	keyFieldType := entryStructType.RawFields[keyFieldIdx].Type
	stringHdrType, ok = keyFieldType.(*ir.GoStringHeaderType)
	require.True(t, ok, "expected string key type, got %T", keyFieldType)

	// Offsets in header
	dirPtrOff := fieldOffsetByName(t, headerType.RawFields, "dirPtr")
	dirLenOff := fieldOffsetByName(t, headerType.RawFields, "dirLen")
	usedOff := fieldOffsetByName(t, headerType.RawFields, "used")

	// Offsets in table
	groupsFieldOffset := groupsField.Offset

	// Offsets in groups structure
	dataFieldOffset := fieldOffsetByName(t, groupsType.RawFields, "data")

	// Offsets in group
	slotsFieldOffset := slotsField.Offset
	ctrlField := fieldOffsetByName(t, groupType.RawFields, "ctrl")

	// Offsets in string header
	strPtrOff := fieldOffsetByName(t, stringHdrType.RawFields, "str")
	strLenOff := fieldOffsetByName(t, stringHdrType.RawFields, "len")

	// Sizes
	rootLen := int(eventType.GetByteSize())
	headerLen := int(headerType.GetByteSize())
	tableLen := int(tableType.GetByteSize())
	groupsStructLen := int(groupsType.GetByteSize())
	entrySize := int(entryStructType.GetByteSize())
	groupSliceElementSize := int(groupSliceType.Element.GetByteSize())

	// Addresses
	const (
		headerAddr        = uint64(0x100000001)
		tablePtrSliceAddr = uint64(0x200000002)
		tableAddr         = uint64(0x300000003)
		groupsSliceAddr   = uint64(0x400000004)
		strAddr           = uint64(0x500000005)
	)

	// Build root data item
	rootData := make([]byte, rootLen)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 3 // bits 0 and 1 set
	}
	ptrOff := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[ptrOff:ptrOff+8], headerAddr)
	templatePtrOff := int(eventType.Expressions[1].Offset)
	binary.NativeEndian.PutUint64(rootData[templatePtrOff:templatePtrOff+8], headerAddr)

	// Build header bytes
	headerData := make([]byte, headerLen)
	// dirLen = 1 (means dirPtr points to table pointer slice)
	binary.NativeEndian.PutUint64(headerData[dirLenOff:dirLenOff+8], 1)
	// dirPtr points to table pointer slice
	binary.NativeEndian.PutUint64(headerData[dirPtrOff:dirPtrOff+8], tablePtrSliceAddr)
	// used = 1
	binary.NativeEndian.PutUint64(headerData[usedOff:usedOff+8], 1)

	// Build table pointer slice data (one pointer to table)
	tablePtrSliceData := make([]byte, 8)
	binary.NativeEndian.PutUint64(tablePtrSliceData, tableAddr)

	// Build table data
	tableData := make([]byte, tableLen)
	// groups field is stored inline in the table (not a pointer)
	// The groups structure's data field points to the slice
	groupsStructInline := tableData[groupsFieldOffset : groupsFieldOffset+uint32(groupsStructLen)]
	binary.NativeEndian.PutUint64(groupsStructInline[dataFieldOffset:dataFieldOffset+8], groupsSliceAddr)

	// Build groups slice data (one group)
	groupsSliceData := make([]byte, groupSliceElementSize)
	// Copy group data will be done separately, but we need the slice to point to it
	// Actually, the groups slice contains the group data directly, not pointers
	// So groupsSliceData IS the group data
	groupData := groupsSliceData

	// Build group data with one entry: ["a"] => 1
	// ctrl: mark slot 0 as occupied (bit 7+0*8 = bit 7 should be 0)
	ctrlWord := uint64(0)
	for i := 1; i < 8; i++ {
		ctrlWord |= 1 << (7 + uint(i*8))
	}
	binary.LittleEndian.PutUint64(groupData[ctrlField:ctrlField+8], ctrlWord)
	// Entry 0: key (string header)
	entry0Off := slotsFieldOffset + 0*uint32(entrySize)
	keyOff := entry0Off + keyFieldOffset
	binary.NativeEndian.PutUint64(groupData[keyOff+strPtrOff:keyOff+strPtrOff+8], strAddr)
	binary.NativeEndian.PutUint64(groupData[keyOff+strLenOff:keyOff+strLenOff+8], 1)
	// Entry 0: value (int = 1)
	elemOff := entry0Off + elemFieldOffset
	binary.NativeEndian.PutUint64(groupData[elemOff:elemOff+8], 1)

	// String data bytes for "a"
	strData := []byte("a")

	// Compute total event length with padding
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
	sz += dataItemHeaderSize + len(tablePtrSliceData)
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + tableLen
	sz = nextMultipleOf8(sz)
	sz += dataItemHeaderSize + len(groupsSliceData)
	sz = nextMultipleOf8(sz)
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
	tablePtrSliceHeader := output.DataItemHeader{
		Type:    uint32(tablePtrSliceType.GetID()),
		Length:  uint32(len(tablePtrSliceData)),
		Address: tablePtrSliceAddr,
	}
	tableHeader := output.DataItemHeader{
		Type:    uint32(tableType.GetID()),
		Length:  uint32(tableLen),
		Address: tableAddr,
	}
	groupsSliceHeader := output.DataItemHeader{
		Type:    uint32(groupSliceType.GetID()),
		Length:  uint32(len(groupsSliceData)),
		Address: groupsSliceAddr,
	}
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
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&tablePtrSliceHeader)), unsafe.Sizeof(tablePtrSliceHeader))...)
	item = append(item, tablePtrSliceData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&tableHeader)), unsafe.Sizeof(tableHeader))...)
	item = append(item, tableData...)
	pad()
	item = append(item, unsafe.Slice((*byte)(unsafe.Pointer(&groupsSliceHeader)), unsafe.Sizeof(groupsSliceHeader))...)
	item = append(item, groupsSliceData...)
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
	require.GreaterOrEqual(t, len(events), 1)
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
	// Set presence bits for both expressions (bit 0 for argument, bit 1 for template_segment)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 3 // bits 0 and 1 set
	}
	// First expression (argument) at offset 1
	ptrOff := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[ptrOff:ptrOff+8], headerAddr)
	// Second expression (template_segment) at offset 9
	templatePtrOff := int(eventType.Expressions[1].Offset)
	binary.NativeEndian.PutUint64(rootData[templatePtrOff:templatePtrOff+8], headerAddr)

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
	// Set presence bits for both expressions (bit 0 for argument, bit 1 for template_segment)
	if eventType.PresenceBitsetSize > 0 {
		rootData[0] = 3 // bits 0 and 1 set
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
	// First expression (argument) at offset 1: address of first pointer
	off := int(eventType.Expressions[0].Offset)
	binary.NativeEndian.PutUint64(rootData[off:off+8], addr1)
	// Second expression (template_segment) at offset 9: same address
	templateOff := int(eventType.Expressions[1].Offset)
	binary.NativeEndian.PutUint64(rootData[templateOff:templateOff+8], addr1)

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

// addStackToEvent adds a stack trace to an event and returns a new event with
// the stack embedded.
func addStackToEvent(event []byte, stack []uint64) []byte {
	stackByteLen := len(stack) * 8
	headerSize := int(unsafe.Sizeof(output.EventHeader{}))
	newEvent := make([]byte, len(event)+stackByteLen)
	copy(newEvent, event)
	copy(newEvent[headerSize+stackByteLen:], event[headerSize:])
	header, err := output.Event(newEvent[:len(event)]).Header()
	if err != nil {
		panic(fmt.Sprintf("failed to get header: %v", err))
	}
	header.Data_byte_len += uint32(stackByteLen)
	header.Stack_byte_len = uint16(stackByteLen)
	*(*output.EventHeader)(unsafe.Pointer(&newEvent[0])) = *header
	for i, addr := range stack {
		binary.NativeEndian.PutUint64(
			newEvent[headerSize+i*8:], addr,
		)
	}
	return newEvent
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

func (t *panicDecoderType) encodeValueFields(
	*encodingContext, *jsontext.Encoder, []byte,
) error {
	panic("boom")
}

func TestDecoderPanics(t *testing.T) {
	caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
		return c.probeName == "stringArg"
	})
	testCase := &cases[caseIdx]
	irProg := generateIrForProbes(t, "simple", testCase.goVersion, "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
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
		EntryOrLine: output.Event(input),
		ServiceName: "foo"},
		&noopSymbolicator{},
		nil,
		[]byte{},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestDecoderFailsOnEvaluationError(t *testing.T) {
	caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
		return c.probeName == "stringArg"
	})
	testCase := &cases[caseIdx]
	irProg := generateIrForProbes(t, "simple", testCase.goVersion, "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
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
		EntryOrLine: output.Event(input),
		ServiceName: "foo"},
		&noopSymbolicator{},
		nil,
		[]byte{},
	)
	require.NoError(t, err)
	require.Contains(t, string(out), "no decoder type found")
}

func TestDecoderIsRobustToDataItemDecodingErrors(t *testing.T) {
	c := cases[0]
	irProg := generateIrForProbes(t, "simple", c.goVersion, c.probeName)
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
	eventData := simpleStringArgEvent(t, irProg)
	event := output.Event(eventData)
	eventHeader, err := output.Event(eventData).Header()
	require.NoError(t, err)
	newItemOffset := eventHeader.Data_byte_len
	eventHeader.Data_byte_len += uint32(unsafe.Sizeof(output.DataItemHeader{}) + 8)
	for range unsafe.Sizeof(output.DataItemHeader{}) + 8 {
		event = append(event, 0)
	}
	dataItemHeader := (*output.DataItemHeader)(unsafe.Pointer(&event[newItemOffset]))
	dataItemHeader.Type = uint32(ir.TypeID(1))
	dataItemHeader.Length = 424242 // too long
	dataItemHeader.Address = 0xdeadbeef
	var items []output.DataItem
	var itemErr error
	for item, err := range event.DataItems() {
		if err != nil {
			itemErr = err
			break
		}
		items = append(items, item)
	}
	require.Regexp(t, "not enough bytes to read data item", itemErr)

	buf, probe, err := decoder.Decode(Event{
		EntryOrLine: event,
		ServiceName: "foo",
	}, &noopSymbolicator{}, nil, []byte{})
	require.NoError(t, err)
	require.Equal(t, c.probeName, probe.GetID())
	var e eventCaptures
	require.NoError(t, json.Unmarshal(buf, &e))
	require.Equal(t, c.expected, e.Debugger.Snapshot.Captures.Entry.Arguments)
}

// TestDecoderFailsOnEvaluationErrorAndRetainsPassedBuffer tests that the decoder
// fails on evaluation error while preserving the contents of the passed buffer.
// It is expected that consumers of the decoder API will call Decode with a reused
// buffer to avoid unnecessary allocations and they will expect the buffer to be
// appended to only.
func TestDecoderFailsOnEvaluationErrorAndRetainsPassedBuffer(t *testing.T) {
	caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
		return c.probeName == "stringArg"
	})
	testCase := &cases[caseIdx]
	irProg := generateIrForProbes(t, "simple", testCase.goVersion, "stringArg")
	decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
	require.NoError(t, err)
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
		EntryOrLine: output.Event(input),
		ServiceName: "foo"},
		&noopSymbolicator{},
		nil,
		buf,
	)
	require.NoError(t, err)
	require.Contains(t, string(out), "no decoder type found")
	require.Equal(t, buf, []byte{1, 2, 3, 4, 5})

}

func TestDecoderMissingReturnEventEvaluationError(t *testing.T) {
	tests := []struct {
		name                    string
		pairingExpectation      output.EventPairingExpectation
		expectedErrorExpression string
		expectedErrorMessage    string
		shouldHaveError         bool
	}{
		{
			name:                    "return pairing expected",
			pairingExpectation:      output.EventPairingExpectationReturnPairingExpected,
			expectedErrorExpression: "@duration",
			expectedErrorMessage:    "not available: return event not received",
			shouldHaveError:         true,
		},
		{
			name:                    "buffer full",
			pairingExpectation:      output.EventPairingExpectationBufferFull,
			expectedErrorExpression: "@duration",
			expectedErrorMessage:    "not available: userspace buffer capacity exceeded",
			shouldHaveError:         true,
		},
		{
			name:                    "call map full",
			pairingExpectation:      output.EventPairingExpectationCallMapFull,
			expectedErrorExpression: "@duration",
			expectedErrorMessage:    "not available: call map capacity exceeded",
			shouldHaveError:         true,
		},
		{
			name:                    "call count exceeded",
			pairingExpectation:      output.EventPairingExpectationCallCountExceeded,
			expectedErrorExpression: "@duration",
			expectedErrorMessage:    "not available: maximum call count exceeded",
			shouldHaveError:         true,
		},
		{
			name:                    "inlined",
			pairingExpectation:      output.EventPairingExpectationNoneInlined,
			expectedErrorExpression: "@duration",
			expectedErrorMessage:    "not available: function was inlined",
			shouldHaveError:         true,
		},
		{
			name:                    "no body",
			pairingExpectation:      output.EventPairingExpectationNoneNoBody,
			expectedErrorExpression: "@duration",
			expectedErrorMessage:    "not available: function has no body",
			shouldHaveError:         true,
		},
		{
			name:               "no pairing expected",
			pairingExpectation: output.EventPairingExpectationNone,
			shouldHaveError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseIdx := slices.IndexFunc(cases, func(c testCase) bool {
				return c.probeName == "stringArg"
			})
			testCase := &cases[caseIdx]
			irProg := generateIrForProbes(t, "simple", testCase.goVersion, "stringArg")
			decoder, err := NewDecoder(irProg, &noopTypeNameResolver{}, time.Now())
			require.NoError(t, err)

			// Construct an entry event with the specified pairing expectation
			eventData := simpleStringArgEvent(t, irProg)
			stack := []uint64{0x1000, 0x2000, 0x3000}
			newEvent := addStackToEvent(eventData, stack)
			header, err := output.Event(newEvent).Header()
			require.NoError(t, err)
			header.Event_pairing_expectation = uint8(tt.pairingExpectation)

			buf, probe, err := decoder.Decode(Event{
				EntryOrLine: newEvent,
				Return:      nil, // Explicitly no return event
				ServiceName: "foo",
			}, &noopSymbolicator{}, nil, []byte{})
			require.NoError(t, err)
			require.Equal(t, "stringArg", probe.GetID())

			var e eventCaptures
			require.NoError(t, json.Unmarshal(buf, &e))

			if tt.shouldHaveError {
				require.NotEmpty(t, e.Debugger.EvaluationErrors,
					"expected evaluation error but none found")
				found := false
				for _, evalErr := range e.Debugger.EvaluationErrors {
					if evalErr.Expression == tt.expectedErrorExpression &&
						evalErr.Message == tt.expectedErrorMessage {
						found = true
						break
					}
				}
				require.True(t, found,
					"expected evaluation error with expression %q and message %q, got errors: %+v",
					tt.expectedErrorExpression, tt.expectedErrorMessage,
					e.Debugger.EvaluationErrors)
			} else {
				// Check that there's no return-related error
				require.Empty(t, e.Debugger.EvaluationErrors)
			}
		})
	}
}
