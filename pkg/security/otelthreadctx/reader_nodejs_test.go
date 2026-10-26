// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package otelthreadctx

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// fakeMemory is one contiguous range of a process's address space.
type fakeMemory struct {
	base uint64
	data []byte
}

var errFakeFault = errors.New("fault")

func (m *fakeMemory) Read(addr uint64, buf []byte) error {
	if addr < m.base || addr-m.base+uint64(len(buf)) > uint64(len(m.data)) {
		return errFakeFault
	}
	copy(buf, m.data[addr-m.base:])
	return nil
}

func (m *fakeMemory) putWord(addr, value uint64) {
	binary.NativeEndian.PutUint64(m.data[addr-m.base:], value)
}

func nodeJSTag(addr uint64) uint64 {
	return addr | otelV8HeapObjectTag
}

func nodeJSSmi(value int64) uint64 {
	return uint64(value) << 32
}

// Where the fake V8 graph lives, mirroring otel_nodejs_build in
// pkg/security/tests/syscall_tester/c/otel_nodejs_common.h.
const (
	fakeBase          = 0x10000
	fakeThreadPointer = fakeBase
	fakeTLSOffset     = 0x40
	fakeDiscovery     = fakeThreadPointer + fakeTLSOffset
	fakeCped          = fakeBase + 0x100
	fakeALSSlot       = fakeBase + 0x108
	fakeALSObject     = fakeBase + 0x110
	fakeUndefined     = fakeBase + 0x118
	fakeFrame         = fakeBase + 0x200
	fakeWrapper       = fakeBase + 0x300
	fakeTable         = fakeBase + 0x400
	// Odd on purpose: nothing says the writer aligns the record.
	fakeRecord = fakeBase + 0x1001

	fakeRecordSlotOffset = 0x18
	fakeBuckets          = 16
	fakeALSHash          = 0x5eed0003
	fakeSpanID           = 204

	// OTEL_NODEJS_MAX_CHAIN in pkg/security/ebpf/c/include/helpers/span_nodejs.h,
	// spelled out rather than taken from otelNodeJSMaxChain: the ptracer must
	// drop exactly the chains the eBPF probe drops.
	ebpfNodeJSMaxChain = 4
)

var fakeTraceID = utils.TraceID{Hi: 0x0102030405060708, Lo: 0x090a0b0c0d0e0f10}

// fakeNodeJSGraph lays out the object graph the reader walks, with the entry
// it is after behind decoys other keys of its bucket.
func fakeNodeJSGraph(decoys int) *fakeMemory {
	m := &fakeMemory{base: fakeBase, data: make([]byte, 0x2000)}

	m.putWord(fakeUndefined, nodeJSTag(fakeUndefined))
	m.putWord(fakeALSObject, nodeJSTag(fakeALSObject))
	m.putWord(fakeALSSlot, nodeJSTag(fakeALSObject))
	m.putWord(fakeWrapper+fakeRecordSlotOffset, fakeRecord)

	fields := uint64(fakeTable + 0x10)
	buckets := fields + otelV8OHMBucketsIndex*8
	entries := buckets + fakeBuckets*8
	entry := func(i int) uint64 { return entries + uint64(i)*otelV8OHMEntryWords*8 }

	m.putWord(fields+otelV8OHMNumDeletedIndex*8, nodeJSSmi(0))
	m.putWord(fields+otelV8OHMNumBucketsIndex*8, nodeJSSmi(fakeBuckets))
	for i := range fakeBuckets {
		m.putWord(buckets+uint64(i)*8, nodeJSSmi(-1))
	}

	m.putWord(entry(0), nodeJSTag(fakeALSObject))
	m.putWord(entry(0)+8, nodeJSTag(fakeWrapper))
	m.putWord(entry(0)+16, nodeJSSmi(-1))
	head := 0
	for i := 1; i <= decoys; i++ {
		m.putWord(entry(i), nodeJSTag(entry(i)))
		m.putWord(entry(i)+8, nodeJSTag(fakeUndefined))
		m.putWord(entry(i)+16, nodeJSSmi(int64(head)))
		head = i
	}
	m.putWord(buckets+uint64(fakeALSHash&(fakeBuckets-1))*8, nodeJSSmi(int64(head)))
	m.putWord(fields+otelV8OHMNumElementsIndex*8, nodeJSSmi(int64(decoys+1)))

	m.putWord(fakeFrame+0x18, nodeJSTag(fakeTable))
	m.putWord(fakeCped, nodeJSTag(fakeFrame))

	m.putWord(fakeDiscovery, fakeCped)
	m.putWord(fakeDiscovery+8, fakeALSSlot)
	binary.NativeEndian.PutUint32(m.data[fakeDiscovery+16-fakeBase:], fakeALSHash)
	m.data[fakeDiscovery+20-fakeBase] = fakeRecordSlotOffset
	m.putWord(fakeDiscovery+24, nodeJSTag(fakeUndefined))

	record := m.data[fakeRecord-fakeBase:]
	binary.BigEndian.PutUint64(record[0:8], fakeTraceID.Hi)
	binary.BigEndian.PutUint64(record[8:16], fakeTraceID.Lo)
	binary.BigEndian.PutUint64(record[16:24], fakeSpanID)
	record[otelRecordValidOffset] = 1
	attrs := []byte{0, 3, 'G', 'E', 'T'}
	binary.NativeEndian.PutUint16(record[otelRecordAttrsSizeOffset:], uint16(len(attrs)))
	copy(record[otelRecordHeaderSize:], attrs)

	return m
}

func fakeNodeJSLayout(t *testing.T) Layout {
	schema, ok := otelNodeJSSchemaFor("nodejs_v1_dev")
	require.True(t, ok)
	return Layout{runtimeLang: otelRuntimeNodeJS, tlsOffset: fakeTLSOffset, v8: schema.layout}
}

func TestReadNodeJSRecord(t *testing.T) {
	layout := fakeNodeJSLayout(t)

	assertRecord := func(t *testing.T, mem *fakeMemory) {
		t.Helper()
		record, status := layout.ReadRecord(mem, fakeThreadPointer)
		require.Equal(t, ReadOK, status)
		require.Equal(t, fakeTraceID, record.TraceID)
		require.Equal(t, uint64(fakeSpanID), record.SpanID)
		require.Equal(t, []Attribute{{KeyIndex: 0, Value: "GET"}}, ParseAttributes(record.Attrs))
	}

	assertStatus := func(t *testing.T, mem *fakeMemory, threadPointer uint64, expected ReadStatus) {
		t.Helper()
		_, status := layout.ReadRecord(mem, threadPointer)
		require.Equal(t, expected, status)
	}

	t.Run("head", func(t *testing.T) {
		assertRecord(t, fakeNodeJSGraph(0))
	})

	t.Run("bucket_collision", func(t *testing.T) {
		assertRecord(t, fakeNodeJSGraph(1))
	})

	t.Run("deepest_followed", func(t *testing.T) {
		assertRecord(t, fakeNodeJSGraph(ebpfNodeJSMaxChain-1))
	})

	t.Run("chain_overflow", func(t *testing.T) {
		assertStatus(t, fakeNodeJSGraph(ebpfNodeJSMaxChain), fakeThreadPointer, ReadNone)
	})

	t.Run("no_thread_pointer", func(t *testing.T) {
		assertStatus(t, fakeNodeJSGraph(0), 0, ReadNoThreadPointer)
	})

	t.Run("discovery_fault", func(t *testing.T) {
		assertStatus(t, fakeNodeJSGraph(0), fakeBase+0x10000, ReadFault)
	})

	t.Run("no_writer", func(t *testing.T) {
		mem := fakeNodeJSGraph(0)
		clear(mem.data[fakeDiscovery-fakeBase : fakeDiscovery-fakeBase+otelNodeJSTLSExportSize])
		assertStatus(t, mem, fakeThreadPointer, ReadNone)
	})

	t.Run("no_context", func(t *testing.T) {
		mem := fakeNodeJSGraph(0)
		mem.putWord(fakeCped, nodeJSTag(fakeUndefined))
		assertStatus(t, mem, fakeThreadPointer, ReadNone)
	})

	t.Run("invalidated_record", func(t *testing.T) {
		mem := fakeNodeJSGraph(0)
		mem.data[fakeRecord+otelRecordValidOffset-fakeBase] = 0
		assertStatus(t, mem, fakeThreadPointer, ReadNone)
	})

	t.Run("cleared_record_pointer", func(t *testing.T) {
		mem := fakeNodeJSGraph(0)
		mem.putWord(fakeWrapper+fakeRecordSlotOffset, 0)
		assertStatus(t, mem, fakeThreadPointer, ReadNone)
	})

	t.Run("bucket_count_not_power_of_two", func(t *testing.T) {
		mem := fakeNodeJSGraph(0)
		mem.putWord(fakeTable+0x10+otelV8OHMNumBucketsIndex*8, nodeJSSmi(fakeBuckets-1))
		assertStatus(t, mem, fakeThreadPointer, ReadNone)
	})

	t.Run("entry_past_declared_count", func(t *testing.T) {
		// The head index points past the entries the table declares.
		mem := fakeNodeJSGraph(1)
		mem.putWord(fakeTable+0x10+otelV8OHMNumElementsIndex*8, nodeJSSmi(1))
		assertStatus(t, mem, fakeThreadPointer, ReadNone)
	})

	t.Run("pointer_compression", func(t *testing.T) {
		compressed := layout
		compressed.v8.taggedSize = 4
		_, status := compressed.ReadRecord(fakeNodeJSGraph(0), fakeThreadPointer)
		require.Equal(t, ReadNone, status)
	})
}
