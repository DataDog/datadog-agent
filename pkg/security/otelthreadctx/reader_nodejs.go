// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelthreadctx

import (
	"encoding/binary"

	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
)

// Mirrors pkg/security/ebpf/c/include/helpers/span_nodejs.h hop for hop: change
// both together, both probes must report the same records.
//
// The Node.js writer hangs each record off the asynchronous context rather than
// off the thread, and publishes in a thread-local of its own where to start
// walking for one:
//
//	cped_slot -> AsyncContextFrame (a JS Map)
//	          -> its OrderedHashMap
//	          -> the one bucket the identity hash designates
//	          -> the entry whose key is the ALS instance
//	          -> its value, the JS object wrapping the record
//	          -> the record

const (
	// Tag of a V8 pointer to a heap object
	otelV8HeapObjectTag = 1

	// Fields of an OrderedHashMap
	otelV8OHMNumElementsIndex = 0
	otelV8OHMNumDeletedIndex  = 1
	otelV8OHMNumBucketsIndex  = 2
	otelV8OHMBucketsIndex     = 3
	otelV8OHMEntryWords       = 3
	// V8 keeps an OrderedHashMap at half load, so its entry table holds twice
	// its bucket count.
	otelV8OHMLoadFactor = 2
	// An AsyncContextFrame holds one entry per AsyncLocalStorage in use, so
	// more buckets than this means the walk landed on something else.
	otelV8OHMMaxBuckets = 1024

	// How far the chain of one bucket is followed. The ptracer could follow it
	// further, but not without reporting contexts the eBPF probe drops.
	otelNodeJSMaxChain = 4
)

// otelNodeJSCtx mirrors struct otel_nodejs_ctx_t in
// pkg/security/ebpf/c/include/structs/span_context.h.
type otelNodeJSCtx struct {
	cpedSlot         uint64
	alsHandle        uint64
	alsIdentityHash  uint32
	recordSlotOffset uint8
	undefinedAddr    uint64
}

func parseOTelNodeJSCtx(buf []byte) otelNodeJSCtx {
	return otelNodeJSCtx{
		cpedSlot:         binary.NativeEndian.Uint64(buf[0:8]),
		alsHandle:        binary.NativeEndian.Uint64(buf[8:16]),
		alsIdentityHash:  binary.NativeEndian.Uint32(buf[16:20]),
		recordSlotOffset: buf[20],
		// buf[21:24] is _pad.
		undefinedAddr: binary.NativeEndian.Uint64(buf[24:32]),
	}
}

func otelV8Untag(value uint64) uint64 {
	return value &^ otelV8HeapObjectTag
}

func otelV8IsHeapObject(value uint64) bool {
	return value&otelV8HeapObjectTag == otelV8HeapObjectTag
}

// otelV8Smi decodes a Smi, or returns -1 for a word that is not one.
func otelV8Smi(value uint64) int64 {
	if value&0xffffffff != 0 {
		return -1
	}
	return int64(value) >> 32
}

// readNodeJSRecord mirrors fill_span_context_nodejs.
func (res Layout) readNodeJSRecord(mem otelprocessctx.Memory, threadPointer uint64) (Record, ReadStatus) {
	// Every step of the walk assumes a V8 built without pointer compression,
	// the only one the nodejs_v1 schema describes.
	if res.v8.taggedSize != otelSupportedTaggedSize {
		return Record{}, ReadNone
	}
	if threadPointer == 0 {
		return Record{}, ReadNoThreadPointer
	}

	varAddr, status := res.tlsVarAddr(mem, threadPointer)
	if status != ReadOK {
		return Record{}, status
	}

	var buf [otelNodeJSTLSExportSize]byte
	if err := mem.Read(varAddr, buf[:]); err != nil {
		return Record{}, ReadFault
	}
	nctx := parseOTelNodeJSCtx(buf[:])

	// Left zeroed until the writer runs on this thread
	if nctx.cpedSlot == 0 {
		return Record{}, ReadNone
	}

	// No entry is an asynchronous context carrying no thread context
	recordPtr, ok := res.v8.nodeJSRecordPtr(mem, nctx)
	if !ok {
		return Record{}, ReadNone
	}
	return readRecordAt(mem, recordPtr)
}

// nodeJSRecordPtr mirrors otel_nodejs_record_ptr: it walks from the discovery
// struct to the record the active asynchronous context holds.
func (v8 otelV8Layout) nodeJSRecordPtr(mem otelprocessctx.Memory, nctx otelNodeJSCtx) (uint64, bool) {
	tagged := uint64(v8.taggedSize)

	frame, err := readWord(mem, nctx.cpedSlot)
	if err != nil || frame == nctx.undefinedAddr || !otelV8IsHeapObject(frame) {
		return 0, false
	}

	table, err := readWord(mem, otelV8Untag(frame)+uint64(v8.jsMapTableOffset))
	if err != nil || !otelV8IsHeapObject(table) {
		return 0, false
	}
	fields := otelV8Untag(table) + uint64(v8.orderedHashMapHeaderSize)

	bucketsWord, err := readWord(mem, fields+otelV8OHMNumBucketsIndex*tagged)
	if err != nil {
		return 0, false
	}
	// V8 keeps the bucket count a power of two, which the mask below needs.
	numBuckets := otelV8Smi(bucketsWord)
	if numBuckets <= 0 || numBuckets > otelV8OHMMaxBuckets || numBuckets&(numBuckets-1) != 0 {
		return 0, false
	}

	bucket := uint64(nctx.alsIdentityHash) & uint64(numBuckets-1)
	headWord, err := readWord(mem, fields+(otelV8OHMBucketsIndex+bucket)*tagged)
	if err != nil {
		return 0, false
	}

	// The handle is a slot the garbage collector keeps pointing at the ALS
	// instance, so the instance address has to be read now.
	als, err := readWord(mem, nctx.alsHandle)
	if err != nil {
		return 0, false
	}

	entries := fields + (otelV8OHMBucketsIndex+uint64(numBuckets))*tagged
	capacity := uint64(numBuckets) * otelV8OHMLoadFactor

	// By construction every entry the table ever wrote lives in the first
	// number_of_elements + number_of_deleted_elements slots.
	numElementsWord, err := readWord(mem, fields+otelV8OHMNumElementsIndex*tagged)
	if err != nil {
		return 0, false
	}
	numDeletedWord, err := readWord(mem, fields+otelV8OHMNumDeletedIndex*tagged)
	if err != nil {
		return 0, false
	}
	numElements := otelV8Smi(numElementsWord)
	numDeleted := otelV8Smi(numDeletedWord)
	if numElements < 0 || numDeleted < 0 {
		return 0, false
	}
	entryCount := min(uint64(numElements+numDeleted), capacity)

	value, ok := v8.lookupALS(mem, entries, entryCount, otelV8Smi(headWord), als)
	if !ok || value == nctx.undefinedAddr || !otelV8IsHeapObject(value) {
		return 0, false
	}

	// The value is the JS object wrapping the record: its internal field 0
	// holds the record pointer directly.
	record, err := readWord(mem, otelV8Untag(value)+uint64(nctx.recordSlotOffset))
	// Cleared when the isolate is torn down.
	if err != nil || record == 0 {
		return 0, false
	}
	return record, true
}

// lookupALS mirrors otel_nodejs_lookup_als: it walks the chain of one bucket
// looking for the entry keyed by the ALS instance, and yields its value.
func (v8 otelV8Layout) lookupALS(mem otelprocessctx.Memory, entries, entryCount uint64, entryIndex int64, als uint64) (uint64, bool) {
	var entry [otelV8OHMEntryWords * otelSupportedTaggedSize]byte

	for range otelNodeJSMaxChain {
		if entryIndex < 0 || uint64(entryIndex) >= entryCount {
			return 0, false
		}

		// key, value and next index are adjacent: one read for the whole entry.
		addr := entries + uint64(entryIndex)*otelV8OHMEntryWords*uint64(v8.taggedSize)
		if err := mem.Read(addr, entry[:]); err != nil {
			return 0, false
		}

		key := binary.NativeEndian.Uint64(entry[0:8])
		if key == als {
			return binary.NativeEndian.Uint64(entry[8:16]), true
		}
		entryIndex = otelV8Smi(binary.NativeEndian.Uint64(entry[16:24]))
	}

	return 0, false
}
